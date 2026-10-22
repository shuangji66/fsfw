// 卸载时的用户数据清理。
//
// MSI 卸载会删掉安装目录、快捷方式和注册表项，但不会碰用户在 Documents 下的数据。
// WiX 里安排了一个自定义动作，在 RemoveFiles 之前以
//     fsfw.exe --uninstall-clean-docs --ui-level=<UILevel> --clean-docs-mode=<CLEANDOCS>
// 调用本程序；这里负责询问并（可选）删除 Documents\fpkstudio。
//
// 无提示卸载（例如企业静默卸载）可以传 MSI 属性 CLEANDOCS=1 / CLEANDOCS=0：
//   msiexec /x FSFW.msi /qn CLEANDOCS=0
// 未提供该属性时：有交互界面（UILevel >= 4）弹询问框；静默 / 基本 UI（/qn、/qb）
// 默认**保留**数据——无人值守卸载不该停在一个没人点的 MessageBox 上。
use std::{
    fs,
    path::{Path, PathBuf},
};

use crate::dialogs;

/// 处理卸载模式；返回 true 表示本次进程就是卸载清理，调用方应直接结束。
pub fn run_if_requested() -> bool {
    let Some(args) = parse_args() else {
        return false;
    };
    cleanup(args.mode.as_deref(), args.ui_level);
    true
}

/// 卸载模式解析出来的参数。
struct Args {
    /// `CLEANDOCS` 的值：Some("1") 删、Some("0") 留，其它（含空）表示要问。
    mode: Option<String>,
    /// MSI 的 UILevel：2 静默、3 基本 UI、4 精简、5 完整。取不到按「有界面」处理。
    ui_level: Option<u32>,
}

/// 解析参数：返回 None 表示不是卸载模式。
fn parse_args() -> Option<Args> {
    let mut requested = false;
    let mut args = Args {
        mode: None,
        ui_level: None,
    };
    let mut argv = std::env::args().skip(1).peekable();
    while let Some(arg) = argv.next() {
        match arg.as_str() {
            "--uninstall-clean-docs" => requested = true,
            "--clean-docs-mode" => {
                // MSI 属性为空时会展开成空字符串，cmd 解析后可能直接没有下一个参数。
                // WiX 现在用 `--clean-docs-mode=` 形式，这里保留空格形式给手工调用。
                // peek 一下再消费：下一个参数是另一个开关时不能把它当成值吞掉。
                if let Some(value) = argv.peek() {
                    if !value.trim().is_empty() && !value.starts_with("--") {
                        args.mode = argv.next();
                    }
                }
            }
            "--ui-level" => {
                if let Some(value) = argv.peek() {
                    if !value.starts_with("--") {
                        args.ui_level = argv.next().and_then(|value| value.trim().parse().ok());
                    }
                }
            }
            other => {
                if let Some(value) = other.strip_prefix("--clean-docs-mode=") {
                    if !value.trim().is_empty() {
                        args.mode = Some(value.trim().to_string());
                    }
                } else if let Some(value) = other.strip_prefix("--ui-level=") {
                    args.ui_level = value.trim().parse().ok();
                }
            }
        }
    }
    requested.then_some(args)
}

fn cleanup(mode: Option<&str>, ui_level: Option<u32>) {
    // WebView2 的缓存目录由 Tauri 固定放在 %LOCALAPPDATA%\<identifier>，与用户数据无关，
    // 卸载时总是清掉，避免留下几百 MB 的缓存。
    if let Some(local) = dirs::data_local_dir() {
        let profile = local.join("com.fpkstudio.fsfw");
        if profile.exists() {
            let _ = fs::remove_dir_all(&profile);
        }
    }

    let Some(docs) = documents_data_dir() else {
        return;
    };
    if !docs.exists() {
        return;
    }

    let remove = match mode {
        Some("1") => true,
        Some("0") => false,
        // 静默 / 基本 UI 下没有人能点确认框：默认保留数据，避免卸载流程挂住。
        // 有交互界面时（没传 UILevel 也按有界面处理）才询问。
        _ if ui_level.is_some_and(|level| level < 4) => false,
        _ => ask(&docs),
    };
    if !remove {
        return;
    }

    if let Err(err) = fs::remove_dir_all(&docs) {
        dialogs::error(
            "删除用户数据失败",
            &format!(
                "无法完全删除：{}\n\n{}\n\n可以稍后手动删除该目录。",
                docs.display(),
                err
            ),
        );
    }
}

fn documents_data_dir() -> Option<PathBuf> {
    dirs::document_dir().map(|documents| documents.join("fpkstudio"))
}

fn ask(docs: &Path) -> bool {
    dialogs::confirm(
        "卸载 fpkstudio for Windows",
        &format!(
            "是否同时删除用户数据？\n\n{}\n\n\
             其中包含项目、导出产物、日志和设置。选择「否」会保留这些文件，\
             以后重新安装仍可继续使用。\n\n\
             Delete this folder as well? Choose No to keep your projects and settings.",
            docs.display()
        ),
    )
}
