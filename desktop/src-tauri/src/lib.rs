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

/// The one decision for every top-level navigation. Only the app's fixed
/// origin, `tauri.localhost`, loads inside the WebView, plus `dev`, the dev
/// server a debug build is served from. The page shown now never counts: a
/// foreign page must not make its own origin the app's.
pub fn navigation_for(dev: Option<&Url>, target: &Url) -> Navigation {
    let from_dev = dev.is_some_and(|dev| dev.origin() == target.origin());
    match target.scheme() {
        "http" | "https" if from_dev || target.host_str() == Some("tauri.localhost") => {
            Navigation::Stay
        }
        "http" | "https" if target.host_str().is_some() => Navigation::Browser,
        // about:blank and blob: belong to the page's own documents.
        "about" | "blob" => Navigation::Stay,
        _ => Navigation::Block,
    }
}

#[cfg(any(mobile, test))]
fn handle_navigation<E>(
    dev: Option<&Url>,
    target: &Url,
    open: impl FnOnce(String) -> Result<(), E> + Send + 'static,
) -> bool {
    match navigation_for(dev, target) {
        Navigation::Stay => true,
        Navigation::Browser => {
            let address = target.as_str().to_owned();
            // Android's opener waits for the UI thread, which must leave this callback first.
            tauri::async_runtime::spawn_blocking(move || open_browser_link(address, open));
            false
        }
        Navigation::Block => false,
    }
}

#[cfg(any(mobile, test))]
fn open_browser_link<E>(address: String, open: impl FnOnce(String) -> Result<(), E>) {
    if open(address).is_err() {
        // Neither the URL nor the error is safe to log: either can carry a token.
        eprintln!("could not open a web link in the browser");
    }
}

#[cfg(mobile)]
#[tauri::mobile_entry_point]
pub fn run() {
    use tauri::Manager;
    use tauri_plugin_opener::OpenerExt;

    let external_links = tauri::plugin::Builder::<tauri::Wry>::new("external-links")
        .on_navigation(|webview, target| {
            let dev = if cfg!(debug_assertions) {
                webview.app_handle().config().build.dev_url.clone()
            } else {
                None
            };
            let app = webview.app_handle().clone();
            handle_navigation(dev.as_ref(), target, move |address| {
                app.opener().open_url(address, None::<&str>)
            })
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
    use std::sync::mpsc;
    use std::time::Duration;

    fn url(raw: &str) -> Url {
        Url::parse(raw).unwrap()
    }

    #[test]
    fn browser_navigation_returns_without_waiting_for_the_opener() {
        let (started_tx, started_rx) = mpsc::channel();
        let (release_tx, release_rx) = mpsc::channel();
        let (returned_tx, returned_rx) = mpsc::channel();
        let (opened_tx, opened_rx) = mpsc::channel();
        let target = "https://example.com/path?token=secret#fragment";

        let handler = std::thread::spawn(move || {
            let allowed = handle_navigation(None, &url(target), move |address| {
                started_tx.send(()).unwrap();
                release_rx.recv().unwrap();
                opened_tx.send(address).unwrap();
                Ok::<_, ()>(())
            });
            returned_tx.send(allowed).unwrap();
        });

        // Bound a regression failure, but use channels to order the opener and callback.
        let started = started_rx.recv_timeout(Duration::from_secs(5));
        let returned = returned_rx.recv_timeout(Duration::from_secs(5));
        release_tx.send(()).unwrap();
        handler.join().unwrap();
        assert_eq!(started, Ok(()));
        assert_eq!(returned, Ok(false));
        assert_eq!(
            opened_rx.recv_timeout(Duration::from_secs(5)).unwrap(),
            target
        );
        assert!(opened_rx.recv_timeout(Duration::from_secs(5)).is_err());
    }

    #[test]
    fn internal_and_blocked_navigation_never_launch_an_opener() {
        let dev = url("http://192.168.1.5:1420/");
        for (target, allowed) in [
            ("http://tauri.localhost/#/servers/local/tasks", true),
            ("http://192.168.1.5:1420/#/x", true),
            ("about:blank", true),
            ("javascript:alert(1)", false),
            ("file:///private/data", false),
            ("intent://scan/#Intent;scheme=zxing;end", false),
        ] {
            let (opened_tx, opened_rx) = mpsc::channel();
            assert_eq!(
                handle_navigation(Some(&dev), &url(target), move |address| {
                    opened_tx.send(address).unwrap();
                    Ok::<_, ()>(())
                }),
                allowed,
                "{target}"
            );
            assert_eq!(opened_rx.recv(), Err(mpsc::RecvError));
        }
    }

    #[test]
    fn an_opener_error_keeps_navigation_cancelled_and_allows_another_attempt() {
        for target in ["https://example.com/first", "http://example.com/again"] {
            let (opened_tx, opened_rx) = mpsc::channel();
            assert!(!handle_navigation(None, &url(target), move |address| {
                opened_tx.send(address).unwrap();
                Err("browser refused https://example.com/?token=secret")
            }));
            assert_eq!(
                opened_rx.recv_timeout(Duration::from_secs(5)).unwrap(),
                target
            );
            assert!(opened_rx.recv_timeout(Duration::from_secs(5)).is_err());
        }
    }

    #[test]
    fn browser_worker_handles_an_opener_error_without_panicking() {
        open_browser_link("https://example.com/?token=secret".to_owned(), |_| {
            Err("browser refused https://example.com/?token=secret")
        });
    }

    #[test]
    fn app_pages_load_in_the_webview() {
        for target in [
            "http://tauri.localhost/#/servers/local/tasks",
            "http://tauri.localhost/index.html",
            "https://tauri.localhost/",
            "about:blank",
        ] {
            assert_eq!(
                navigation_for(None, &url(target)),
                Navigation::Stay,
                "{target}"
            );
        }
        // A debug build is served from its dev server.
        let dev = url("http://192.168.1.5:1420/");
        assert_eq!(
            navigation_for(Some(&dev), &url("http://192.168.1.5:1420/#/x")),
            Navigation::Stay
        );
    }

    #[test]
    fn a_release_build_trusts_no_other_origin() {
        assert_eq!(
            navigation_for(None, &url("http://192.168.1.5:1420/#/x")),
            Navigation::Browser
        );
    }

    #[test]
    fn web_pages_open_in_the_browser() {
        let app = url("http://192.168.1.5:1420/");
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
