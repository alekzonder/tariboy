//! Page zoom of the main window, changed from the app menu and kept in
//! `zoom.json` across restarts.

use serde::{Deserialize, Serialize};
use std::fs;
use std::path::Path;
use std::sync::Mutex;
use uuid::Uuid;

pub const DEFAULT: f64 = 1.0;
const MIN: f64 = 0.5;
const MAX: f64 = 2.0;

const STEP: f64 = 0.1;

#[derive(Serialize, Deserialize)]
struct ZoomFile {
    zoom: f64,
}

/// Ids of the View menu items, in display order.
pub const MENU_IDS: [&str; 3] = ["zoom-in", "zoom-out", "zoom-reset"];

/// The zoom the main window has now; set at startup and by every menu change.
pub struct Current(pub Mutex<f64>);

/// The zoom a menu item asks for, or None when `id` is not a zoom item.
pub fn next(id: &str, current: f64) -> Option<f64> {
    match id {
        "zoom-in" => Some(step(current + STEP)),
        "zoom-out" => Some(step(current - STEP)),
        "zoom-reset" => Some(DEFAULT),
        _ => None,
    }
}

// Rounding to whole percents keeps repeated steps from drifting (1.0 + 0.1 * 3).
fn step(value: f64) -> f64 {
    ((value * 100.0).round() / 100.0).clamp(MIN, MAX)
}

/// A missing, unreadable or out-of-range file gives the default zoom.
pub fn load(path: &Path) -> f64 {
    fs::read(path)
        .ok()
        .and_then(|bytes| serde_json::from_slice::<ZoomFile>(&bytes).ok())
        .map(|file| file.zoom)
        .filter(|zoom| (MIN..=MAX).contains(zoom))
        .unwrap_or(DEFAULT)
}

/// Writes a temporary file and renames it over `path`, so a crash never
/// leaves a half-written `zoom.json`.
pub fn save(path: &Path, zoom: f64) -> Result<(), String> {
    if !zoom.is_finite() {
        return Err(format!("invalid zoom {zoom}"));
    }
    let parent = path
        .parent()
        .ok_or_else(|| format!("zoom file has no parent: {}", path.display()))?;
    fs::create_dir_all(parent).map_err(|error| format!("create {}: {error}", parent.display()))?;
    let temp = parent.join(format!(".zoom.json.tmp-{}", Uuid::new_v4()));
    let bytes =
        serde_json::to_vec(&ZoomFile { zoom }).map_err(|error| format!("encode zoom: {error}"))?;
    let result = fs::write(&temp, bytes)
        .and_then(|()| fs::rename(&temp, path))
        .map_err(|error| format!("write {}: {error}", path.display()));
    if result.is_err() {
        let _ = fs::remove_file(&temp);
    }
    result
}

#[cfg(test)]
mod tests {
    use super::*;

    fn zoom_in(current: f64) -> f64 {
        next("zoom-in", current).unwrap()
    }

    fn zoom_out(current: f64) -> f64 {
        next("zoom-out", current).unwrap()
    }

    #[test]
    fn steps_by_ten_percent_without_drift() {
        let mut zoom = DEFAULT;
        for _ in 0..3 {
            zoom = zoom_in(zoom);
        }
        assert_eq!(zoom, 1.3);
        for _ in 0..5 {
            zoom = zoom_out(zoom);
        }
        assert_eq!(zoom, 0.8);
    }

    #[test]
    fn menu_ids_map_to_zoom_changes() {
        assert_eq!(next("zoom-in", 1.0), Some(1.1));
        assert_eq!(next("zoom-out", 1.0), Some(0.9));
        assert_eq!(next("zoom-reset", 1.7), Some(DEFAULT));
        assert_eq!(next("quit", 1.0), None);
        for id in MENU_IDS {
            assert!(next(id, 1.0).is_some(), "{id}");
        }
    }

    #[test]
    fn stays_within_range() {
        assert_eq!(zoom_in(MAX), MAX);
        assert_eq!(zoom_in(1.95), MAX);
        assert_eq!(zoom_out(MIN), MIN);
        assert_eq!(zoom_out(0.55), MIN);
    }

    #[test]
    fn save_then_load_round_trips() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("nested").join("zoom.json");
        save(&path, 1.2).unwrap();
        assert_eq!(load(&path), 1.2);
        save(&path, 0.7).unwrap();
        assert_eq!(load(&path), 0.7);
        // Only zoom.json remains: the temporary file was renamed over it.
        let names: Vec<_> = std::fs::read_dir(path.parent().unwrap())
            .unwrap()
            .map(|e| e.unwrap().file_name())
            .collect();
        assert_eq!(names, ["zoom.json"]);
    }

    #[test]
    fn unreadable_or_invalid_file_loads_default() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("zoom.json");
        assert_eq!(load(&path), DEFAULT, "missing file");
        for body in [
            "",
            "{",
            "[]",
            r#"{"zoom":"big"}"#,
            r#"{"zoom":5}"#,
            r#"{"zoom":0.1}"#,
        ] {
            std::fs::write(&path, body).unwrap();
            assert_eq!(load(&path), DEFAULT, "body {body:?}");
        }
    }

    #[test]
    fn save_rejects_non_finite_value() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("zoom.json");
        assert!(save(&path, f64::NAN).is_err());
        assert!(!path.exists());
    }
}
