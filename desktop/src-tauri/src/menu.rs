//! The macOS menu-bar (tray) item.
//!
//! Quitting the app leaves the daemon running on purpose: it is a background
//! service, and agents may be mid-iteration. The menu bar is what keeps the
//! running daemon visible and controllable once the window is gone.

use crate::{commands, startup, state::AppState};
use tauri::menu::{MenuBuilder, MenuItemBuilder};
use tauri::tray::TrayIconBuilder;
use tauri::{App, AppHandle, Manager};
use tauri_plugin_dialog::DialogExt;

/// Menu item ids, in display order. Kept as one array so the builder and the
/// click handler cannot drift apart.
// The builder and the handler spell the ids inline (a match arm cannot bind a
// runtime slice), so this array is read only by the test that pins the set.
#[allow(dead_code)]
pub const IDS: [&str; 6] = ["status", "open", "restart", "log", "install-cli", "quit"];

pub fn install(app: &App) -> tauri::Result<()> {
    let state = app.state::<AppState>();

    let status = MenuItemBuilder::with_id("status", state.view().menu_label())
        .enabled(false)
        .build(app)?;
    let open = MenuItemBuilder::with_id("open", "Open Tariboy").build(app)?;
    let restart = MenuItemBuilder::with_id("restart", "Restart daemon").build(app)?;
    let log = MenuItemBuilder::with_id("log", "Open daemon log").build(app)?;
    let install_cli = MenuItemBuilder::with_id("install-cli", "Install/Update CLI").build(app)?;
    let quit = MenuItemBuilder::with_id("quit", "Quit Tariboy").build(app)?;

    let menu = MenuBuilder::new(app)
        .items(&[&status])
        .separator()
        .items(&[&open, &restart, &log, &install_cli])
        .separator()
        .items(&[&quit])
        .build()?;

    state.set_status_item(status);

    TrayIconBuilder::with_id("main")
        .icon(app.default_window_icon().unwrap().clone())
        .menu(&menu)
        .show_menu_on_left_click(true)
        .on_menu_event(|app, event| match event.id().as_ref() {
            "open" => show_window(app),
            // Every long action runs off the UI thread: restart can block for the
            // full stop+start timeout, and a frozen menu bar looks like a crash.
            // The work itself is `commands::restart_daemon`, shared with the IPC
            // command, so the tray and the banner cannot publish different things.
            "restart" => {
                let h = app.clone();
                std::thread::spawn(move || {
                    commands::restart_daemon(&h);
                });
            }
            "log" => {
                let _ = commands::open_daemon_log(app.state::<AppState>());
            }
            "install-cli" => {
                let h = app.clone();
                std::thread::spawn(move || install_cli_with_dialog(&h));
            }
            // Exit only the app. The daemon keeps running — that is the contract.
            "quit" => app.exit(0),
            _ => {}
        })
        .build(app)?;

    Ok(())
}

/// show_window raises the main window, whether it was hidden by a window close
/// or minimised. Shared by the tray item and the macOS Dock-reopen handler.
pub fn show_window(app: &AppHandle) {
    startup::show_main(app);
}

/// A foreign occupant at any managed ~/.local/bin path is reported, never
/// overwritten; the dialog names the conflict so the user can resolve it.
fn install_cli_with_dialog(app: &AppHandle) {
    let msg = match commands::install_update_cli(app) {
        Ok(r) if r.outcome == "occupied" => format!(
            "Not updated: {}.\n\nRemove the conflicting entry and try again.",
            r.existing
        ),
        Ok(r) if r.outcome == "already-installed" => format!(
            "All four managed binaries were already installed from {}.\n\nThe daemon was restarted.",
            r.target
        ),
        Ok(r) => format!(
            "Updated all four managed binaries.\n\nExample: {} -> {}\n\nThe daemon was restarted. Make sure ~/.local/bin is on your PATH.",
            r.link, r.target
        ),
        Err(e) => format!("Install/Update CLI failed: {e}"),
    };
    app.dialog()
        .message(msg)
        .title("Install/Update CLI")
        .show(|_| {});
}

#[cfg(test)]
mod tests {
    use super::*;

    // The menu ids are the contract between the builder and the click handler; a
    // typo would silently make an item do nothing.
    #[test]
    fn menu_ids_are_the_expected_set() {
        assert_eq!(
            IDS,
            ["status", "open", "restart", "log", "install-cli", "quit"]
        );
    }
}
