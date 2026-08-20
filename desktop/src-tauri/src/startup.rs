use tauri::{AppHandle, Manager};

pub fn should_hide_on_close(label: &str) -> bool {
    label == "main"
}

/// Raises the main window for the tray Open action and macOS Dock reopen.
pub fn show_main(app: &AppHandle) {
    reveal_main(app);
}

fn reveal_main(app: &AppHandle) {
    if let Some(main) = app.get_webview_window("main") {
        let _ = main.unminimize();
        let _ = main.show();
        let _ = main.set_focus();
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn window<'a>(config: &'a serde_json::Value, label: &str) -> &'a serde_json::Value {
        config["app"]["windows"]
            .as_array()
            .expect("windows array")
            .iter()
            .find(|window| window["label"] == label)
            .unwrap_or_else(|| panic!("missing {label} window"))
    }

    #[test]
    fn startup_policy_hides_only_the_main_window_on_close() {
        assert!(should_hide_on_close("main"));
        assert!(!should_hide_on_close("other"));
    }

    #[test]
    fn tauri_config_starts_with_one_visible_main_window() {
        let config: serde_json::Value =
            serde_json::from_str(include_str!("../tauri.conf.json")).expect("valid Tauri config");
        let windows = config["app"]["windows"].as_array().expect("windows array");
        assert_eq!(windows.len(), 1);
        let main = window(&config, "main");

        assert_eq!(main["visible"], true);
        assert_eq!(main["decorations"], true);
        assert_eq!(main["titleBarStyle"], "Overlay");
        assert_eq!(main["hiddenTitle"], true);
        assert_eq!(main["trafficLightPosition"]["x"], 14);
        assert_eq!(main["trafficLightPosition"]["y"], 15);
    }

    #[test]
    fn main_window_capability_allows_titlebar_dragging() {
        let capability: serde_json::Value = serde_json::from_str(include_str!(
            "../capabilities/default.json"
        ))
        .expect("valid default capability");
        let permissions = capability["permissions"]
            .as_array()
            .expect("permissions array");

        assert!(permissions
            .iter()
            .any(|permission| permission == "core:window:allow-start-dragging"));
    }
}
