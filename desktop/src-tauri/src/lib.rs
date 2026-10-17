// Mobile entry point. The Android app is the browser-mode SPA in a WebView: it
// talks to servers the user adds over their own HTTP API, so the host exposes
// no commands and carries none of the desktop daemon, SSH, tunnel, PTY or
// updater code in main.rs. Its one plugin keeps web links out of the WebView.

use tauri::Url;

/// What happens to a navigation the WebView is about to make.
#[derive(Debug, PartialEq, Eq)]
pub enum Navigation {
    /// The app's own pages: the WebView loads it.
    Stay,
    /// A web page: the system browser opens it, the WebView stays put.
    Browser,
    /// Anything else (javascript:, file:, intent:, …): nothing opens.
    Block,
}

/// The one decision for every top-level navigation. `app` is the page the
/// WebView shows now; only its own origin may load inside the app.
pub fn navigation_for(app: Option<&Url>, target: &Url) -> Navigation {
    let same_origin = app.is_some_and(|app| app.origin() == target.origin());
    match target.scheme() {
        // tauri.localhost is the Android asset origin before the first page.
        "http" | "https" if same_origin || target.host_str() == Some("tauri.localhost") => {
            Navigation::Stay
        }
        "http" | "https" if target.host_str().is_some() => Navigation::Browser,
        // about:blank and blob: belong to the page's own documents.
        "about" | "blob" => Navigation::Stay,
        _ => Navigation::Block,
    }
}

#[cfg(mobile)]
#[tauri::mobile_entry_point]
pub fn run() {
    use tauri::Manager;
    use tauri_plugin_opener::OpenerExt;

    let external_links = tauri::plugin::Builder::<tauri::Wry>::new("external-links")
        .on_navigation(|webview, target| {
            match navigation_for(webview.url().ok().as_ref(), target) {
                Navigation::Stay => true,
                Navigation::Browser => {
                    if let Err(error) = webview
                        .app_handle()
                        .opener()
                        .open_url(target.as_str(), None::<&str>)
                    {
                        // The URL is not logged: it may carry a token in its query.
                        eprintln!("could not open a web link in the browser: {error}");
                    }
                    false
                }
                Navigation::Block => false,
            }
        })
        .build();

    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .plugin(external_links)
        .run(tauri::generate_context!())
        .expect("error while running Tariboy");
}

#[cfg(test)]
mod tests {
    use super::*;

    fn url(raw: &str) -> Url {
        Url::parse(raw).unwrap()
    }

    #[test]
    fn app_pages_load_in_the_webview() {
        let app = url("http://tauri.localhost/#/agents");
        for target in [
            "http://tauri.localhost/#/servers/local/tasks",
            "http://tauri.localhost/index.html",
            "https://tauri.localhost/",
            "about:blank",
        ] {
            assert_eq!(
                navigation_for(Some(&app), &url(target)),
                Navigation::Stay,
                "{target}"
            );
        }
        assert_eq!(
            navigation_for(None, &url("http://tauri.localhost/")),
            Navigation::Stay
        );
        // A dev server origin is the app's own when the app is served from it.
        let dev = url("http://192.168.1.5:1420/");
        assert_eq!(
            navigation_for(Some(&dev), &url("http://192.168.1.5:1420/#/x")),
            Navigation::Stay
        );
    }

    #[test]
    fn web_pages_open_in_the_browser() {
        let app = url("http://tauri.localhost/");
        for target in [
            "https://example.com/",
            "http://example.com/path?q=1",
            "https://github.com/alekzonder/tariboy/pull/1",
            "http://127.0.0.1:9990/api/daemon/status",
            "http://tauri.localhost.evil.com/",
        ] {
            assert_eq!(
                navigation_for(Some(&app), &url(target)),
                Navigation::Browser,
                "{target}"
            );
        }
    }

    #[test]
    fn other_schemes_are_blocked() {
        let app = url("http://tauri.localhost/");
        for target in [
            "javascript:alert(1)",
            "file:///data/data/app.tariboy.desktop/shared_prefs/x.xml",
            "intent://scan/#Intent;scheme=zxing;package=com.example;end",
            "data:text/html,<p>x</p>",
            "mailto:a@example.com",
            "tel:123",
            "content://media/external/images/1",
        ] {
            assert_eq!(
                navigation_for(Some(&app), &url(target)),
                Navigation::Block,
                "{target}"
            );
        }
    }
}
