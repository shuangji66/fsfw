// 后端进程的生命周期与运行配置交换。
//
// Go 后端不是 Tauri 的 sidecar 插件管理的，而是普通子进程：启动参数由这里给，
// 端口与令牌由后端写进 <data>\run\server.json，这边轮询到文件后再建窗口。
// 这样前端拿到的 apiBase 一定是后端已经 listen 成功的那个端口（--port 0）。
use serde::{Deserialize, Serialize};
use std::{
    fs::{self, File, OpenOptions},
    io::{Read, Seek, SeekFrom, Write},
    path::{Path, PathBuf},
    process::{Child, Command, Stdio},
    sync::Mutex,
    thread,
    time::{Duration, Instant},
};
use tauri::{AppHandle, Manager};

/// Go 后端写出的运行时信息（字段名与后端 json tag 一致）。
#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RuntimeInfo {
    pub port: u16,
    #[allow(dead_code)]
    pub token: String,
    pub pid: u32,
    #[allow(dead_code)]
    pub version: String,
    #[allow(dead_code)]
    pub data_dir: String,
    #[allow(dead_code)]
    pub started_at: String,
}

/// 注入到页面里的 window.__FSFW__。
#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct InitConfig {
    pub api_base: String,
    pub token: String,
    pub version: String,
    pub data_dir: String,
}

pub struct Started {
    pub child: Child,
    pub init: InitConfig,
    pub runtime_file: PathBuf,
}

/// 由 Tauri 托管的后端句柄。退出时从这里取走并杀掉。
pub struct BackendState {
    child: Mutex<Option<Child>>,
    runtime_file: PathBuf,
}

impl BackendState {
    pub fn new(child: Child, runtime_file: PathBuf) -> Self {
        Self {
            child: Mutex::new(Some(child)),
            runtime_file,
        }
    }
}

fn runtime_file_of(data_dir: &Path) -> PathBuf {
    data_dir.join("run").join("server.json")
}

/// 解析并准备 <Documents>\fpkstudio。
pub fn ensure_data_dir(app: &AppHandle, version: &str) -> Result<PathBuf, String> {
    let documents = app
        .path()
        .document_dir()
        .map_err(|err| format!("无法定位当前用户的 Documents 目录：{err}"))?;
    let data_dir = documents.join("fpkstudio");
    for dir in [&data_dir, &data_dir.join("logs"), &data_dir.join("run")] {
        fs::create_dir_all(dir)
            .map_err(|err| format!("创建数据目录 {} 失败：{err}", dir.display()))?;
    }
    append_log(
        &data_dir,
        &format!(
            "---- fsfw {version} 启动，数据目录 {} ----",
            data_dir.display()
        ),
    );
    Ok(data_dir)
}

/// 拉起后端并等到它报告端口。
pub fn start(data_dir: &Path, version: &str) -> Result<Started, String> {
    let backend = locate_backend().ok_or_else(|| {
        "找不到后端程序 fsfw-backend.exe：安装包可能不完整，请重新安装。".to_string()
    })?;

    let log_path = data_dir.join("logs").join("shell.log");
    let stdout = OpenOptions::new()
        .create(true)
        .append(true)
        .open(&log_path)
        .map_err(|err| format!("打开外壳日志 {} 失败：{err}", log_path.display()))?;
    let stderr = stdout
        .try_clone()
        .map_err(|err| format!("复制日志句柄失败：{err}"))?;

    let runtime_file = runtime_file_of(data_dir);
    // 上一次异常退出可能留下一个过期的 server.json；先删掉，避免把旧端口当成新后端。
    let _ = fs::remove_file(&runtime_file);

    let token = uuid::Uuid::new_v4().simple().to_string();
    let mut command = Command::new(&backend);
    command
        .current_dir(data_dir)
        .arg("--host")
        .arg("127.0.0.1")
        .arg("--port")
        .arg("0")
        .arg("--token")
        .arg(&token)
        .arg("--data-dir")
        .arg(data_dir)
        .arg("--runtime-file")
        .arg(&runtime_file)
        .arg("--parent-pid")
        .arg(std::process::id().to_string())
        .stdin(Stdio::null())
        .stdout(Stdio::from(stdout))
        .stderr(Stdio::from(stderr));
    hide_console(&mut command);

    let mut child = command
        .spawn()
        .map_err(|err| format!("启动后端 {} 失败：{err}", backend.display()))?;

    let deadline = Instant::now() + Duration::from_secs(20);
    loop {
        if let Some(status) = child
            .try_wait()
            .map_err(|err| format!("等待后端退出状态失败：{err}"))?
        {
            let tail = log_tail(&log_path, 40);
            return Err(format!(
                "后端进程已退出（{status}）。\n\n日志：{}\n\n{}",
                log_path.display(),
                tail
            ));
        }

        if let Ok(text) = fs::read_to_string(&runtime_file) {
            if let Ok(info) = serde_json::from_str::<RuntimeInfo>(&text) {
                if info.port > 0 && info.pid == child.id() && info.token == token {
                    append_log(
                        data_dir,
                        &format!(
                            "后端已就绪：pid={} port={}（{}）",
                            info.pid, info.port, info.started_at
                        ),
                    );
                    let init = InitConfig {
                        api_base: format!("http://127.0.0.1:{}", info.port),
                        token,
                        version: version.to_string(),
                        data_dir: data_dir.to_string_lossy().to_string(),
                    };
                    return Ok(Started {
                        child,
                        init,
                        runtime_file,
                    });
                }
            }
        }

        if Instant::now() >= deadline {
            let _ = child.kill();
            let _ = child.wait();
            let tail = log_tail(&log_path, 40);
            return Err(format!(
                "后端 20 秒内没有就绪。\n\n日志：{}\n\n{}",
                log_path.display(),
                tail
            ));
        }
        thread::sleep(Duration::from_millis(100));
    }
}

/// 停掉后端（幂等）。
pub fn stop(app: &AppHandle) {
    let Some(state) = app.try_state::<BackendState>() else {
        return;
    };
    let mut guard = match state.child.lock() {
        Ok(guard) => guard,
        Err(poisoned) => poisoned.into_inner(),
    };
    if let Some(mut child) = guard.take() {
        let _ = child.kill();
        let _ = child.wait();
    }
    let _ = fs::remove_file(&state.runtime_file);
}

/// 在资源管理器里打开一个目录 / 选中一个文件。
///
/// Windows 用 explorer.exe；其它平台只为开发期能跑（xdg-open）。
pub fn reveal(path: &Path) -> Result<(), String> {
    if !path.exists() {
        return Err(format!("路径不存在：{}", path.display()));
    }

    #[cfg(windows)]
    {
        let mut command = Command::new("explorer.exe");
        if path.is_file() {
            // /select 必须是同一个参数：explorer.exe "/select,C:\a\b.txt"
            command.arg(format!("/select,{}", path.display()));
        } else {
            command.arg(path);
        }
        // explorer.exe 不认 CREATE_NO_WINDOW，但它是 GUI 程序，本来就不会带控制台。
        command
            .spawn()
            .map_err(|err| format!("调用资源管理器失败：{err}"))?;
    }

    #[cfg(not(windows))]
    {
        Command::new("xdg-open")
            .arg(path)
            .spawn()
            .map_err(|err| format!("调用系统文件管理器失败：{err}"))?;
    }

    Ok(())
}

/// 找后端可执行文件：安装目录里通常叫 fsfw-backend.exe；
/// 开发构建（target/debug）里可能还带着 target triple 后缀。
fn locate_backend() -> Option<PathBuf> {
    let exe = std::env::current_exe().ok()?;
    let dir = exe.parent()?;

    let mut candidates: Vec<PathBuf> = Vec::new();
    let names = [
        "fsfw-backend.exe".to_string(),
        "fsfw-backend".to_string(),
        format!("fsfw-backend-{}.exe", target_triple()),
    ];
    for name in &names {
        candidates.push(dir.join(name));
    }

    if let Ok(entries) = fs::read_dir(dir) {
        for entry in entries.flatten() {
            let name = entry.file_name().to_string_lossy().to_string();
            if name.starts_with("fsfw-backend") && !name.ends_with(".log") {
                candidates.push(entry.path());
            }
        }
    }

    candidates.into_iter().find(|path| path.is_file())
}

fn target_triple() -> String {
    // 运行期没有编译期的 TARGET 常量，这里按常见 Windows 目标给一个默认值；
    // 真正兜底的是上面的目录扫描。
    if cfg!(target_arch = "aarch64") {
        "aarch64-pc-windows-msvc".to_string()
    } else {
        "x86_64-pc-windows-msvc".to_string()
    }
}

fn hide_console(command: &mut Command) {
    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        const CREATE_NO_WINDOW: u32 = 0x0800_0000;
        command.creation_flags(CREATE_NO_WINDOW);
    }
    #[cfg(not(windows))]
    {
        let _ = command;
    }
}

fn append_log(data_dir: &Path, line: &str) {
    let path = data_dir.join("logs").join("shell.log");
    let Ok(mut file) = OpenOptions::new().create(true).append(true).open(&path) else {
        return;
    };
    let now = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or(0);
    let _ = writeln!(file, "[{now}] {line}");
}

/// 取日志末尾若干行，启动失败时直接展示给用户。
fn log_tail(path: &Path, lines: usize) -> String {
    let mut file: File = match File::open(path) {
        Ok(file) => file,
        Err(_) => return String::new(),
    };
    let len = file.metadata().map(|m| m.len()).unwrap_or(0);
    let start = len.saturating_sub(16 * 1024);
    if file.seek(SeekFrom::Start(start)).is_err() {
        return String::new();
    }
    let mut buf = String::new();
    if file.read_to_string(&mut buf).is_err() {
        return String::new();
    }
    let collected: Vec<&str> = buf.lines().rev().take(lines).collect();
    collected.into_iter().rev().collect::<Vec<_>>().join("\n")
}
