// Mobile entry point. The Android app is the browser-mode SPA in a WebView: it
// talks to servers the user adds over their own HTTP API, so the host exposes
// no commands or plugins and carries none of the desktop daemon, SSH, tunnel,
// PTY or updater code in main.rs.
#[cfg(mobile)]
#[tauri::mobile_entry_point]
pub fn run() {
    tauri::Builder::default()
        .run(tauri::generate_context!())
        .expect("error while running Tariboy");
}
