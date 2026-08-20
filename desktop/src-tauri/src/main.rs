// Prevents an extra console window on Windows in release. Harmless on macOS and
// kept so the crate stays portable if SP2/SP3 ever widen the target list.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod bundle;
mod cli_install;
mod commands;
mod daemon;
mod hosts;
mod keychain;
mod menu;
mod notifications;
mod paths;
mod preflight;
mod provision;
mod remote_health;
mod ssh;
mod startup;
mod state;
mod support;
mod support_client;
#[cfg(test)]
mod testbin;
mod tunnel;

use state::{AppState, DaemonView};
use std::time::Duration;
use tauri::{Manager, WindowEvent};
use tauri_plugin_dialog::DialogExt;

/// How long a start may take before it is reported as failed. Matches the spec's
/// 10s budget: long enough for a cold SQLite migration, short enough that a
/// broken binary does not look like a hang.
const READY_TIMEOUT: Duration = Duration::from_secs(10);
const POLL_INTERVAL: Duration = Duration::from_millis(100);

fn main() {
    let args = std::env::args().collect::<Vec<_>>();
    if args.get(1).map(String::as_str) == Some("--alpha-smoke-remove-host") {
        let result = (|| {
            if args.len() != 3 {
                return Err("usage: Tariboy --alpha-smoke-remove-host alpha-e2e".to_string());
            }
            let app_data = std::env::var("TARIBOY_DESKTOP_APP_DATA_DIR")
                .map_err(|_| "TARIBOY_DESKTOP_APP_DATA_DIR is required".to_string())?;
            commands::alpha_smoke_remove_host(std::path::Path::new(&app_data), &args[2])
        })();
        match result {
            Ok(()) => {
                println!("removed alpha-e2e");
                return;
            }
            Err(error) => {
                eprintln!("Tariboy alpha smoke: {error}");
                std::process::exit(64);
            }
        }
    }

    tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .invoke_handler(tauri::generate_handler![
            commands::daemon_status,
            commands::daemon_start,
            commands::daemon_restart,
            commands::daemon_stop,
            commands::task_notification_show,
            commands::task_notification_activate_test,
            commands::daemon_log,
            commands::open_daemon_log,
            commands::open_host_path_in_vscode,
            commands::open_external_url,
            commands::support_bundle_export,
            commands::install_cli,
            commands::hosts_list,
            commands::host_save_ssh,
            commands::host_save_https,
            commands::host_session_credentials,
            commands::host_has_token,
            commands::host_provision,
            commands::host_connect,
            commands::host_update,
            commands::host_prompt_reply,
            commands::host_remove,
        ])
        .setup(|app| {
            let handle = app.handle().clone();
            notifications::init(&handle);

            // A base dir we cannot resolve or create is the ONE fatal case: there
            // is no useful window to show without it, so fail loudly instead of
            // opening onto a permanently broken app.
            let app_data = match app.path().app_data_dir() {
                Ok(path) => path,
                Err(e) => return Err(fatal(&handle, &format!("cannot resolve app data dir: {e}"))),
            };
            let app_data = paths::app_data_dir(app_data, &paths::env_getter);
            let p = match paths::resolve(&paths::env_getter) {
                Ok(p) => p.with_app_data(app_data),
                Err(e) => return Err(fatal(&handle, &e)),
            };
            if let Err(e) = std::fs::create_dir_all(&p.base) {
                let msg = format!("cannot create base dir {}: {e}", p.base.display());
                return Err(fatal(&handle, &msg));
            }
            if let Err(e) = std::fs::create_dir_all(&p.app_data) {
                let msg = format!("cannot create app data dir {}: {e}", p.app_data.display());
                return Err(fatal(&handle, &msg));
            }
            if let Err(e) = paths::prepare_ssh_control_dir(&p.ssh_control_dir()) {
                return Err(fatal(&handle, &e));
            }

            let b = bundle::Bundle::new(bundle::resolve_bin_dir(&handle)?);
            let cfg = daemon::Config {
                socket: p.socket(),
                pid_file: p.pid_file(),
                log_file: p.log_file(),
                runtime_dir: p.runtime.clone(),
                daemon_bin: b.daemon_bin(&paths::env_getter),
                ready_timeout: READY_TIMEOUT,
                poll_interval: POLL_INTERVAL,
            };
            let version = b.version();
            app.manage(AppState::new(p, b, cfg));

            // Bounded (1s) adoption probe BEFORE the first paint: when a daemon is
            // already up — the common case, since the CLI may have started it —
            // the SPA's very first request goes to the right port.
            let st = app.state::<AppState>();
            let probe = daemon::probe(&st.cfg.socket, Duration::from_secs(1));
            let up = matches!(probe, daemon::Probe::Up(_));
            if let daemon::Probe::Up(status) = probe {
                st.set(
                    &handle,
                    DaemonView::ready(&status.unwrap_or_default(), true, version),
                );
            }

            menu::install(app)?;

            // Only the SPAWN is asynchronous: up to 10s must not hold the visible
            // main window. The SPA reports startup state while remote hosts remain
            // available.
            if !up {
                let h = handle.clone();
                std::thread::spawn(move || {
                    commands::bring_up(&h);
                });
            }

            let reconnect = handle.clone();
            std::thread::spawn(move || commands::connect_saved_hosts(&reconnect));

            Ok(())
        })
        .on_window_event(|window, event| {
            if startup::should_hide_on_close(window.label()) {
                if let WindowEvent::CloseRequested { api, .. } = event {
                    // Closing the window must not stop the daemon (agents may be
                    // mid-iteration) and must not quit the app — the menu bar stays.
                    api.prevent_close();
                    let _ = window.hide();
                }
            }
        })
        .build(tauri::generate_context!())
        .expect("error while building Tariboy")
        .run(|app, event| {
            if matches!(&event, tauri::RunEvent::ExitRequested { .. }) {
                let state = app.state::<AppState>();
                state.ssh_operations.cancel_all();
                state.tunnels.shutdown();
            }
            // Closing the window only hides it, so on macOS the app keeps running
            // with no window. Clicking its Dock icon then raises Reopen and
            // nothing else — without this the window would be unreachable except
            // through the menu bar, which reads as a hang.
            #[cfg(target_os = "macos")]
            {
                if let tauri::RunEvent::Reopen { .. } = event {
                    menu::show_window(app);
                }
            }
            #[cfg(not(target_os = "macos"))]
            let _ = (app, event);
        });
}

/// fatal shows a blocking dialog and returns an error that aborts setup.
fn fatal(app: &tauri::AppHandle, msg: &str) -> Box<dyn std::error::Error> {
    app.dialog()
        .message(msg)
        .title("Tariboy cannot start")
        .blocking_show();
    msg.to_string().into()
}
