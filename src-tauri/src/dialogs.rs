// 原生消息框。
//
// 启动失败时 WebView 还没起来，前端弹窗帮不上忙；卸载模式更是连 Tauri 都没初始化，
// 所以这里直接走 user32 的 MessageBoxW。非 Windows 平台只为让 cargo check 能过。
#[cfg(windows)]
fn message_box(title: &str, text: &str, flags: u32) -> i32 {
    use windows_sys::Win32::UI::WindowsAndMessaging::{MessageBoxW, MB_SETFOREGROUND, MB_TOPMOST};

    let title: Vec<u16> = title.encode_utf16().chain(std::iter::once(0)).collect();
    let text: Vec<u16> = text.encode_utf16().chain(std::iter::once(0)).collect();
    unsafe {
        MessageBoxW(
            std::ptr::null_mut(),
            text.as_ptr(),
            title.as_ptr(),
            flags | MB_TOPMOST | MB_SETFOREGROUND,
        )
    }
}

#[cfg(not(windows))]
fn message_box(title: &str, text: &str, _flags: u32) -> i32 {
    eprintln!("{title}: {text}");
    0
}

pub fn error(title: &str, text: &str) {
    #[cfg(windows)]
    {
        use windows_sys::Win32::UI::WindowsAndMessaging::{MB_ICONERROR, MB_OK};
        message_box(title, text, MB_OK | MB_ICONERROR);
    }
    #[cfg(not(windows))]
    {
        let _ = message_box(title, text, 0);
    }
}

/// 是 / 否询问框，默认焦点在「否」。
pub fn confirm(title: &str, text: &str) -> bool {
    #[cfg(windows)]
    {
        use windows_sys::Win32::UI::WindowsAndMessaging::{
            IDYES, MB_DEFBUTTON2, MB_ICONQUESTION, MB_YESNO,
        };
        let result = message_box(title, text, MB_YESNO | MB_ICONQUESTION | MB_DEFBUTTON2);
        result == IDYES
    }
    #[cfg(not(windows))]
    {
        let _ = message_box(title, text, 0);
        false
    }
}

/// 用系统默认浏览器打开一个外部链接。
///
/// 设置里的「关于」仓库链接、以后可能加的帮助链接都走这里；不能让 WebView 自己
/// 导航过去——那会把整个应用界面替换成一个网页，还回不到工作区。
pub fn open_external(url: &str) -> Result<(), String> {
    // ShellExecuteW / xdg-open 会按 scheme 分派，`file:` 和自定义协议能直接拉起
    // 本机程序。这里只接受 http/https（调用方也做了一次同样的判断）。
    let lower = url.trim().to_ascii_lowercase();
    if !(lower.starts_with("http://") || lower.starts_with("https://")) {
        return Err(format!("只允许打开 http/https 链接：{url}"));
    }

    #[cfg(windows)]
    {
        use windows_sys::Win32::UI::Shell::ShellExecuteW;
        use windows_sys::Win32::UI::WindowsAndMessaging::SW_SHOWNORMAL;

        fn wide(value: &str) -> Vec<u16> {
            value.encode_utf16().chain(std::iter::once(0)).collect()
        }

        let operation = wide("open");
        let target = wide(url);
        let result = unsafe {
            ShellExecuteW(
                std::ptr::null_mut(),
                operation.as_ptr(),
                target.as_ptr(),
                std::ptr::null(),
                std::ptr::null(),
                SW_SHOWNORMAL,
            )
        };
        // ShellExecuteW 的返回值 > 32 才算成功（文档里的约定）。
        if result as isize <= 32 {
            return Err(format!(
                "无法用默认浏览器打开 {url}（ShellExecuteW 返回 {result:p}）"
            ));
        }
        Ok(())
    }

    #[cfg(not(windows))]
    {
        std::process::Command::new("xdg-open")
            .arg(url)
            .spawn()
            .map_err(|err| format!("无法用默认浏览器打开 {url}：{err}"))?;
        Ok(())
    }
}
