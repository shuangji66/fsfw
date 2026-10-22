// FSFW（fpkstudio for Windows）的桌面外壳。
//
// 职责边界：
//   - 拉起 Go 后端（fsfw-backend.exe），等它把端口与令牌写进 run/server.json；
//   - 用系统 WebView2 打开前端产物，并在页面脚本执行前注入 window.__FSFW__；
//   - 托盘常驻：关闭窗口只是隐藏，托盘菜单里「退出」才真正结束进程；
//   - 单实例：第二次启动只把已有窗口叫到前台；
//   - 卸载模式：MSI 卸载时以 --uninstall-clean-docs 调用本程序，询问是否删除用户数据。
//
// 数据目录固定在 <Documents>\fpkstudio，后端负责在里面创建 projects / output / logs / run。
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod backend;
mod dialogs;
mod uninstall;

use tauri::{
    menu::{Menu, MenuItem, PredefinedMenuItem},
    tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent},
    Manager, RunEvent, WebviewUrl, WebviewWindowBuilder,
};

use backend::{BackendState, InitConfig, Started};

fn main() {
    // 卸载模式必须在任何 Tauri 初始化之前处理：这时没有窗口、没有后端，
    // 只需要弹一个询问框并删除（或保留）Documents\fpkstudio。
    if uninstall::run_if_requested() {
        return;
    }

    let app = tauri::Builder::default()
        // single-instance 必须是第一个插件：第二次启动的进程在插件初始化时就会退出。
        .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
            show_main(app);
        }))
        .plugin(tauri_plugin_dialog::init())
        .setup(|app| {
            if let Err(err) = startup(app) {
                dialogs::error("fpkstudio for Windows 启动失败", &err);
                std::process::exit(1);
            }
            Ok(())
        })
        .build(tauri::generate_context!())
        .unwrap_or_else(|err| {
            dialogs::error(
                "fpkstudio for Windows 启动失败",
                &format!("创建应用失败：{err}"),
            );
            std::process::exit(1);
        });

    app.run(|app_handle, event| {
        // 只在这里收尾：ExitRequested 可能被别的逻辑取消，提前杀后端会让界面失联。
        if matches!(event, RunEvent::Exit) {
            backend::stop(app_handle);
        }
    });
}

/// 启动后端、建窗口、装托盘。任何一步失败都返回一句能直接展示给用户的原因。
fn startup(app: &mut tauri::App) -> Result<(), String> {
    let version = app.package_info().version.to_string();
    let data_dir = backend::ensure_data_dir(app.handle(), &version)?;

    let Started {
        child,
        init,
        runtime_file,
    } = backend::start(&data_dir, &version)?;
    app.manage(BackendState::new(child, runtime_file));

    let script = init_script(&init)?;

    let window = WebviewWindowBuilder::new(app, "main", WebviewUrl::App("index.html".into()))
        .title("fpkstudio for Windows")
        .inner_size(1280.0, 820.0)
        .min_inner_size(960.0, 620.0)
        .center()
        .initialization_script(script)
        // 站外链接交给默认浏览器；应用本身只允许停在 Tauri 自己的页面上。
        .on_navigation(|url| {
            if is_app_url(url) {
                true
            } else {
                if let Err(err) = dialogs::open_external(url.as_str()) {
                    dialogs::error("打开链接失败", &err);
                }
                false
            }
        })
        .on_new_window(|url, _features| {
            if let Err(err) = dialogs::open_external(url.as_str()) {
                dialogs::error("打开链接失败", &err);
            }
            tauri::webview::NewWindowResponse::Deny
        })
        .build()
        .map_err(|err| format!("创建主窗口失败：{err}"))?;

    // 关闭窗口 = 收进托盘。真正退出走托盘菜单，那里会先停后端再退进程。
    let handle = window.clone();
    window.on_window_event(move |event| {
        if let tauri::WindowEvent::CloseRequested { api, .. } = event {
            api.prevent_close();
            let _ = handle.hide();
        }
    });

    install_tray(app, data_dir)?;
    show_main(app.handle());
    Ok(())
}

/// 注入到每个页面（含 dev server 页面）的最早一段脚本。
fn init_script(init: &InitConfig) -> Result<String, String> {
    let json =
        serde_json::to_string(init).map_err(|err| format!("序列化前端运行配置失败：{err}"))?;
    Ok(format!(
        "Object.defineProperty(window,'__FSFW__',{{value:Object.freeze({json}),writable:false,configurable:false}});"
    ))
}

/// 应用自己的页面：生产环境是 `http://tauri.localhost` / `tauri://`，
/// 开发环境是 Vite 的 `http://localhost:5273`。
fn is_app_url(url: &tauri::Url) -> bool {
    match url.scheme() {
        "tauri" | "about" | "data" | "blob" => true,
        "http" | "https" => {
            let host = url.host_str().unwrap_or_default();
            host == "tauri.localhost"
                || (tauri::is_dev() && matches!(host, "localhost" | "127.0.0.1"))
        }
        _ => false,
    }
}

fn show_main(app: &tauri::AppHandle) {
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.show();
        let _ = window.unminimize();
        let _ = window.set_focus();
    }
}

fn install_tray(app: &mut tauri::App, data_dir: std::path::PathBuf) -> Result<(), String> {
    let show = MenuItem::with_id(app, "show", "显示主窗口", true, None::<&str>)
        .map_err(|err| format!("创建托盘菜单失败：{err}"))?;
    let open_data = MenuItem::with_id(app, "open-data", "打开数据目录", true, None::<&str>)
        .map_err(|err| format!("创建托盘菜单失败：{err}"))?;
    let quit = MenuItem::with_id(app, "quit", "退出", true, None::<&str>)
        .map_err(|err| format!("创建托盘菜单失败：{err}"))?;
    let separator =
        PredefinedMenuItem::separator(app).map_err(|err| format!("创建托盘菜单失败：{err}"))?;
    let menu = Menu::with_items(app, &[&show, &open_data, &separator, &quit])
        .map_err(|err| format!("创建托盘菜单失败：{err}"))?;

    let mut builder = TrayIconBuilder::with_id("fsfw-tray")
        .menu(&menu)
        .tooltip("fpkstudio for Windows")
        .show_menu_on_left_click(false)
        .on_menu_event(move |app, event| match event.id.as_ref() {
            "show" => show_main(app),
            "open-data" => {
                if let Err(err) = backend::reveal(&data_dir) {
                    dialogs::error("打开数据目录失败", &err);
                }
            }
            "quit" => {
                // 先停后端再退进程：让它删掉 runtime 文件、关掉日志，避免留下
                // 一个还占着日志文件句柄的孤儿进程。
                backend::stop(app);
                app.exit(0);
            }
            _ => {}
        })
        .on_tray_icon_event(|tray, event| {
            // 左键单击托盘图标 = 把窗口叫回来；右键仍然出菜单。
            if let TrayIconEvent::Click {
                button: MouseButton::Left,
                button_state: MouseButtonState::Up,
                ..
            } = event
            {
                show_main(tray.app_handle());
            }
        });

    if let Some(icon) = app.default_window_icon().cloned() {
        builder = builder.icon(icon);
    }

    builder
        .build(app)
        .map_err(|err| format!("创建托盘图标失败：{err}"))?;
    Ok(())
}
