//! "Install CLI": expose the bundled `tariboy` binary on PATH.
//!
//! A symlink rather than a copy, on purpose. The CLI and the daemon talk over the
//! same control socket and share a schema; pinning them together means an app
//! update moves both at once, with no second install step to forget. A copy would
//! silently drift.
//!
//! Anything at the target path that we did not put there is never overwritten.

use std::path::{Path, PathBuf};

const BUNDLE_IDENTIFIER: &str = "app.tariboy.desktop";

/// A link is recognised as ours by the canonical resource layout it points
/// into. Noncanonical or unverifiable links are never replaced.
fn bundle_root(path: &Path) -> Option<&Path> {
    let platform = path.parent()?;
    let bin = platform.parent()?;
    let resources = bin.parent()?;
    let contents = resources.parent()?;
    let app = contents.parent()?;
    (platform.file_name()? == "darwin-arm64"
        && bin.file_name()? == "bin"
        && resources.file_name()? == "Resources"
        && contents.file_name()? == "Contents"
        && app.file_name()?.to_str()? == "Tariboy.app")
        .then_some(app)
}

fn bundle_identifier(app: &Path) -> Option<String> {
    let value = plist::Value::from_file(app.join("Contents").join("Info.plist")).ok()?;
    value
        .as_dictionary()?
        .get("CFBundleIdentifier")?
        .as_string()
        .map(str::to_string)
}

fn is_managed_bundle_binary(target: &Path, link: &Path, binaries: &[&str]) -> bool {
    let Some(name) = link.file_name().and_then(|name| name.to_str()) else {
        return false;
    };
    if target.file_name().and_then(|target| target.to_str()) != Some(name)
        || !binaries.contains(&name)
    {
        return false;
    }
    if bundle_root(target).is_none() {
        return false;
    }
    let resolved_target = if target.is_absolute() {
        target.to_path_buf()
    } else {
        link.parent().unwrap_or_else(|| Path::new(".")).join(target)
    };
    if !resolved_target.exists() {
        return false;
    }
    bundle_root(&resolved_target)
        .and_then(bundle_identifier)
        .as_deref()
        == Some(BUNDLE_IDENTIFIER)
}

#[derive(Debug, PartialEq, Eq)]
pub enum Outcome {
    /// The link now points at the bundled CLI (freshly made or re-pointed).
    Created,
    /// It already pointed exactly there.
    AlreadyInstalled,
    /// Something foreign is in the way; `existing` describes it for the dialog.
    Occupied { existing: String },
}

struct Change {
    link: PathBuf,
    temporary: PathBuf,
    backup: Option<PathBuf>,
}

/// install_all updates a complete same-basename binary set as one transaction.
/// Every source and destination is preflighted before the first filesystem
/// mutation, so a foreign occupant can never leave a partial installation.
pub fn install_all(
    link_dir: &Path,
    source_dir: &Path,
    binaries: &[&str],
) -> std::io::Result<Outcome> {
    install_all_with_rename(link_dir, source_dir, binaries, &|from, to| {
        std::fs::rename(from, to)
    })
}

fn install_all_with_rename(
    link_dir: &Path,
    source_dir: &Path,
    binaries: &[&str],
    rename: &dyn Fn(&Path, &Path) -> std::io::Result<()>,
) -> std::io::Result<Outcome> {
    let mut pending = Vec::new();
    for binary in binaries {
        let source = source_dir.join(binary);
        let link = link_dir.join(binary);
        if !source.is_file() {
            return Err(std::io::Error::new(
                std::io::ErrorKind::NotFound,
                format!("bundled binary is missing: {}", source.display()),
            ));
        }
        match std::fs::read_link(&link) {
            Ok(target) if target == source => continue,
            Ok(target) if is_managed_bundle_binary(&target, &link, binaries) => {
                pending.push((link, source));
            }
            Ok(target) => {
                return Ok(Outcome::Occupied {
                    existing: format!("{} holds symlink -> {}", link.display(), target.display()),
                });
            }
            Err(_) => match std::fs::symlink_metadata(&link) {
                Ok(metadata) => {
                    let kind = if metadata.is_dir() {
                        "directory"
                    } else {
                        "regular file"
                    };
                    return Ok(Outcome::Occupied {
                        existing: format!("{kind} at {}", link.display()),
                    });
                }
                Err(metadata_error) if metadata_error.kind() == std::io::ErrorKind::NotFound => {
                    pending.push((link, source));
                }
                Err(metadata_error) => return Err(metadata_error),
            },
        }
    }
    if pending.is_empty() {
        return Ok(Outcome::AlreadyInstalled);
    }

    std::fs::create_dir_all(link_dir)?;
    let mut changes: Vec<Change> = Vec::with_capacity(pending.len());
    for (link, source) in pending {
        let temporary = link_dir.join(format!(
            ".tariboy-cli-{}-{}.tmp",
            link.file_name().unwrap_or_default().to_string_lossy(),
            uuid::Uuid::new_v4()
        ));
        if let Err(error) = symlink(&source, &temporary) {
            for change in &changes {
                let _ = std::fs::remove_file(&change.temporary);
            }
            return Err(error);
        }
        changes.push(Change {
            link,
            temporary,
            backup: None,
        });
    }

    let mut applied = 0;
    for index in 0..changes.len() {
        if std::fs::symlink_metadata(&changes[index].link).is_ok() {
            let backup = link_dir.join(format!(
                ".tariboy-cli-{}-{}.backup",
                changes[index]
                    .link
                    .file_name()
                    .unwrap_or_default()
                    .to_string_lossy(),
                uuid::Uuid::new_v4()
            ));
            if let Err(error) = rename(&changes[index].link, &backup) {
                rollback(&changes[..applied]);
                cleanup_staged(&changes);
                return Err(error);
            }
            changes[index].backup = Some(backup);
        }
        applied = index + 1;
        if let Err(error) = rename(&changes[index].temporary, &changes[index].link) {
            rollback(&changes[..applied]);
            cleanup_staged(&changes);
            return Err(error);
        }
    }

    for change in &changes {
        if let Some(backup) = &change.backup {
            let _ = std::fs::remove_file(backup);
        }
    }
    Ok(Outcome::Created)
}

fn rollback(changes: &[Change]) {
    for change in changes.iter().rev() {
        let _ = std::fs::remove_file(&change.link);
        if let Some(backup) = &change.backup {
            let _ = std::fs::rename(backup, &change.link);
        }
    }
}

fn cleanup_staged(changes: &[Change]) {
    for change in changes {
        let _ = std::fs::remove_file(&change.temporary);
        if let Some(backup) = &change.backup {
            if !change.link.exists() {
                let _ = std::fs::rename(backup, &change.link);
            }
            let _ = std::fs::remove_file(backup);
        }
    }
}

fn symlink(src: &Path, dst: &Path) -> std::io::Result<()> {
    std::os::unix::fs::symlink(src, dst)
}

pub fn default_bin_dir(getenv: &dyn Fn(&str) -> Option<String>) -> Result<PathBuf, String> {
    let home = getenv("HOME")
        .filter(|h| !h.is_empty())
        .ok_or_else(|| "HOME is not set".to_string())?;
    Ok(Path::new(&home).join(".local").join("bin"))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::bundle::BINARIES;

    fn bundled(dir: &std::path::Path, app: &str) -> PathBuf {
        bundled_with_identifier(dir, app, "app.tariboy.desktop")
    }

    fn bundled_with_identifier(dir: &std::path::Path, app: &str, identifier: &str) -> PathBuf {
        let p = dir
            .join(app)
            .join("Contents")
            .join("Resources")
            .join("bin")
            .join("darwin-arm64");
        std::fs::create_dir_all(&p).unwrap();
        std::fs::write(
            dir.join(app).join("Contents").join("Info.plist"),
            format!(
                r#"<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>{identifier}</string>
</dict></plist>
"#
            ),
        )
        .unwrap();
        let bin = p.join("tariboy");
        std::fs::write(&bin, b"#!/bin/sh\n").unwrap();
        bin
    }

    fn bundled_set(dir: &Path, app: &str) -> PathBuf {
        let cli = bundled(dir, app);
        let source_dir = cli.parent().unwrap().to_path_buf();
        for binary in BINARIES {
            std::fs::write(source_dir.join(binary), b"#!/bin/sh\n").unwrap();
        }
        source_dir
    }

    #[test]
    fn installs_all_bundled_binaries_as_same_name_links() {
        let dir = tempfile::tempdir().unwrap();
        let source_dir = bundled_set(dir.path(), "Tariboy.app");
        let link_dir = dir.path().join("local-bin");

        assert_eq!(
            install_all(&link_dir, &source_dir, &BINARIES).unwrap(),
            Outcome::Created
        );
        for binary in BINARIES {
            assert_eq!(
                std::fs::read_link(link_dir.join(binary)).unwrap(),
                source_dir.join(binary)
            );
        }
        assert_eq!(
            install_all(&link_dir, &source_dir, &BINARIES).unwrap(),
            Outcome::AlreadyInstalled
        );
    }

    #[test]
    fn foreign_occupant_aborts_the_whole_set_before_changes() {
        let dir = tempfile::tempdir().unwrap();
        let source_dir = bundled_set(dir.path(), "Tariboy.app");
        let link_dir = dir.path().join("local-bin");
        std::fs::create_dir_all(&link_dir).unwrap();
        std::fs::write(link_dir.join("tariboy-plugin-telegram"), b"foreign").unwrap();

        match install_all(&link_dir, &source_dir, &BINARIES).unwrap() {
            Outcome::Occupied { existing } => {
                assert!(existing.contains("tariboy-plugin-telegram"));
                assert!(existing.contains("regular file"));
            }
            other => panic!("outcome = {other:?}, want Occupied"),
        }
        for binary in ["tariboyd", "tariboy", "tariboy-shim"] {
            assert!(!link_dir.join(binary).exists());
        }
        assert_eq!(
            std::fs::read(link_dir.join("tariboy-plugin-telegram")).unwrap(),
            b"foreign"
        );
    }

    #[test]
    fn foreign_bundle_symlink_aborts_the_whole_set() {
        let dir = tempfile::tempdir().unwrap();
        let source_dir = bundled_set(dir.path(), "Tariboy.app");
        let foreign_cli = bundled_with_identifier(
            &dir.path().join("foreign"),
            "Tariboy.app",
            "com.example.foreign",
        );
        let link_dir = dir.path().join("local-bin");
        std::fs::create_dir_all(&link_dir).unwrap();
        std::os::unix::fs::symlink(&foreign_cli, link_dir.join("tariboy")).unwrap();

        assert!(matches!(
            install_all(&link_dir, &source_dir, &BINARIES).unwrap(),
            Outcome::Occupied { .. }
        ));
        assert_eq!(
            std::fs::read_link(link_dir.join("tariboy")).unwrap(),
            foreign_cli
        );
        for binary in ["tariboyd", "tariboy-shim"] {
            assert!(!link_dir.join(binary).exists());
        }
    }

    #[test]
    fn lowercase_bundle_symlink_is_not_managed() {
        let dir = tempfile::tempdir().unwrap();
        let source_dir = bundled_set(&dir.path().join("current"), "Tariboy.app");
        let noncanonical_cli = bundled(&dir.path().join("lowercase"), "tariboy.app");
        let link_dir = dir.path().join("local-bin");
        std::fs::create_dir_all(&link_dir).unwrap();
        std::os::unix::fs::symlink(&noncanonical_cli, link_dir.join("tariboy")).unwrap();

        assert!(matches!(
            install_all(&link_dir, &source_dir, &BINARIES).unwrap(),
            Outcome::Occupied { .. }
        ));
        assert_eq!(
            std::fs::read_link(link_dir.join("tariboy")).unwrap(),
            noncanonical_cli
        );
    }

    #[test]
    fn dangling_lowercase_bundle_symlink_is_not_managed() {
        let dir = tempfile::tempdir().unwrap();
        let source_dir = bundled_set(&dir.path().join("current"), "Tariboy.app");
        let lowercase_root = dir.path().join("lowercase");
        let noncanonical_cli = bundled(&lowercase_root, "tariboy.app");
        let link_dir = dir.path().join("local-bin");
        std::fs::create_dir_all(&link_dir).unwrap();
        std::os::unix::fs::symlink(&noncanonical_cli, link_dir.join("tariboy")).unwrap();
        std::fs::remove_dir_all(lowercase_root).unwrap();

        assert!(matches!(
            install_all(&link_dir, &source_dir, &BINARIES).unwrap(),
            Outcome::Occupied { .. }
        ));
        assert_eq!(
            std::fs::read_link(link_dir.join("tariboy")).unwrap(),
            noncanonical_cli
        );
    }

    #[test]
    fn repoints_every_link_from_an_older_managed_bundle() {
        let dir = tempfile::tempdir().unwrap();
        let old = bundled_set(&dir.path().join("old"), "Tariboy.app");
        let new = bundled_set(&dir.path().join("new"), "Tariboy.app");
        let link_dir = dir.path().join("local-bin");
        std::fs::create_dir_all(&link_dir).unwrap();
        for binary in BINARIES {
            std::os::unix::fs::symlink(old.join(binary), link_dir.join(binary)).unwrap();
        }

        assert_eq!(
            install_all(&link_dir, &new, &BINARIES).unwrap(),
            Outcome::Created
        );
        for binary in BINARIES {
            assert_eq!(
                std::fs::read_link(link_dir.join(binary)).unwrap(),
                new.join(binary)
            );
        }
    }

    #[test]
    fn missing_source_aborts_before_creating_any_link() {
        let dir = tempfile::tempdir().unwrap();
        let source_dir = bundled_set(dir.path(), "Tariboy.app");
        std::fs::remove_file(source_dir.join("tariboy-shim")).unwrap();
        let link_dir = dir.path().join("local-bin");

        assert!(install_all(&link_dir, &source_dir, &BINARIES).is_err());
        for binary in BINARIES {
            assert!(!link_dir.join(binary).exists());
        }
    }

    #[test]
    fn switch_failure_rolls_back_every_original_link() {
        let dir = tempfile::tempdir().unwrap();
        let old = bundled_set(&dir.path().join("old"), "Tariboy.app");
        let new = bundled_set(&dir.path().join("new"), "Tariboy.app");
        let link_dir = dir.path().join("local-bin");
        std::fs::create_dir_all(&link_dir).unwrap();
        for binary in BINARIES {
            std::os::unix::fs::symlink(old.join(binary), link_dir.join(binary)).unwrap();
        }
        let calls = std::cell::Cell::new(0);
        let rename = |from: &Path, to: &Path| {
            let call = calls.get() + 1;
            calls.set(call);
            if call == 4 {
                return Err(std::io::Error::other("injected switch failure"));
            }
            std::fs::rename(from, to)
        };

        assert!(install_all_with_rename(&link_dir, &new, &BINARIES, &rename).is_err());
        for binary in BINARIES {
            assert_eq!(
                std::fs::read_link(link_dir.join(binary)).unwrap(),
                old.join(binary)
            );
        }
        assert!(!std::fs::read_dir(&link_dir).unwrap().any(|entry| entry
            .unwrap()
            .file_name()
            .to_string_lossy()
            .starts_with(".tariboy-cli-")));
    }

    #[test]
    fn default_bin_dir_is_under_home_local_bin() {
        let get = |key: &str| (key == "HOME").then(|| "/home/u".to_string());
        assert_eq!(
            default_bin_dir(&get).unwrap(),
            PathBuf::from("/home/u/.local/bin")
        );
    }
}
