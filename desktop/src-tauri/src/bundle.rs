//! The native binary set shipped inside the Desktop package, and the names of
//! the server archives SSH servers download from a release.

use std::path::{Path, PathBuf};

/// The canonical Go-side internal/version.Version string, injected by build.rs.
/// It remains separate from CARGO_PKG_VERSION so bundled payloads and the
/// running daemon are checked against the same release value.
pub const VERSION: &str = env!("TARIBOY_VERSION");
pub const BINARIES: [&str; 7] = [
    "tariboyd",
    "tariboy",
    "tariboy-tasks",
    "tariboy-mail",
    "tariboy-scripts",
    "tariboy-shim",
    "tariboy-plugin-telegram",
];
pub struct ManagedLink {
    pub name: &'static str,
    pub source: &'static str,
}

pub const MANAGED_LINKS: [ManagedLink; 10] = [
    ManagedLink {
        name: "tariboyd",
        source: "tariboyd",
    },
    ManagedLink {
        name: "tariboy",
        source: "tariboy",
    },
    ManagedLink {
        name: "tariboy-tasks",
        source: "tariboy-tasks",
    },
    ManagedLink {
        name: "ttasks",
        source: "tariboy-tasks",
    },
    ManagedLink {
        name: "tariboy-mail",
        source: "tariboy-mail",
    },
    ManagedLink {
        name: "tmail",
        source: "tariboy-mail",
    },
    ManagedLink {
        name: "tariboy-scripts",
        source: "tariboy-scripts",
    },
    ManagedLink {
        name: "tscripts",
        source: "tariboy-scripts",
    },
    ManagedLink {
        name: "tariboy-shim",
        source: "tariboy-shim",
    },
    ManagedLink {
        name: "tariboy-plugin-telegram",
        source: "tariboy-plugin-telegram",
    },
];
const VERSION_FILE: &str = "VERSION";

/// server_archive names the release archive for an SSH server reporting
/// `uname -s` = `platform` and `uname -m` = `arch`.
pub fn server_archive(platform: &str, arch: &str, version: &str) -> Result<String, String> {
    let target = match (platform, arch) {
        ("Linux", "x86_64") => "linux-x86_64",
        ("Linux", "aarch64" | "arm64") => "linux-aarch64",
        ("Darwin", "arm64") => "darwin-arm64",
        _ => return Err(format!("no published server build for {platform}/{arch}")),
    };
    Ok(format!("tariboy_{version}_{target}.tar.gz"))
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Platform {
    DarwinArm64,
    LinuxX86_64,
}

impl Platform {
    #[allow(unreachable_code)]
    fn local() -> Self {
        #[cfg(target_os = "macos")]
        {
            return Self::DarwinArm64;
        }
        #[cfg(target_os = "linux")]
        {
            return Self::LinuxX86_64;
        }
        Self::DarwinArm64
    }

    fn directory(self) -> &'static str {
        match self {
            Self::DarwinArm64 => "darwin-arm64",
            Self::LinuxX86_64 => "linux-x86_64",
        }
    }
}

#[derive(Debug, Clone)]
pub struct PlatformBundle {
    pub dir: PathBuf,
}

impl PlatformBundle {
    pub fn file(&self, name: &str) -> PathBuf {
        self.dir.join(name)
    }
}

#[derive(Debug, Clone)]
pub struct Bundle {
    /// Root packaged `resources/bin` directory.
    pub bin_dir: PathBuf,
}

impl Bundle {
    pub fn new(bin_dir: PathBuf) -> Self {
        Self { bin_dir }
    }

    pub fn platform(&self, platform: Platform) -> Result<PlatformBundle, String> {
        validate_platform_bundle(&self.bin_dir.join(platform.directory()), VERSION)
    }

    fn local_dir(&self) -> PathBuf {
        self.bin_dir.join(Platform::local().directory())
    }

    pub fn local_bundle(&self) -> Result<PlatformBundle, String> {
        self.platform(Platform::local())
    }

    /// daemon_bin honours $TARIBOY_DAEMON_BIN — the same override
    /// internal/daemonctl.ResolveConfig reads — so a test run can substitute a
    /// wrapper without touching the bundle.
    pub fn daemon_bin(&self, getenv: &dyn Fn(&str) -> Option<String>) -> PathBuf {
        match getenv("TARIBOY_DAEMON_BIN").filter(|s| !s.is_empty()) {
            Some(s) => PathBuf::from(s),
            None => self.local_dir().join("tariboyd"),
        }
    }

    // Consumed once the daemon is spawned: the daemon resolves the shim beside
    // itself, so the bundle must ship it there.
    #[allow(dead_code)]
    pub fn shim_bin(&self) -> PathBuf {
        self.local_dir().join("tariboy-shim")
    }
    pub fn version(&self) -> &'static str {
        VERSION
    }
}

fn validate_platform_bundle(dir: &Path, expected_version: &str) -> Result<PlatformBundle, String> {
    for name in BINARIES {
        let path = dir.join(name);
        if !path.is_file() {
            return Err(format!("bundled binary is missing: {}", path.display()));
        }
    }
    let version_path = dir.join(VERSION_FILE);
    let actual = std::fs::read_to_string(&version_path)
        .map_err(|error| format!("read bundled version {}: {error}", version_path.display()))?;
    let actual = actual.trim();
    if actual != expected_version {
        return Err(format!(
            "bundled version mismatch in {}: found {actual:?}, expected {expected_version:?}",
            version_path.display()
        ));
    }
    Ok(PlatformBundle {
        dir: dir.to_path_buf(),
    })
}

/// resolve_bin_dir returns `Contents/Resources/bin` inside the packaged .app, or
/// the crate's `resources/bin` under `cargo tauri dev`. $TARIBOY_BIN_DIR
/// overrides both, which is how the dev loop can point at the repo's `bin/`.
pub fn resolve_bin_dir(app: &tauri::AppHandle) -> Result<PathBuf, String> {
    use tauri::Manager;
    if let Ok(d) = std::env::var("TARIBOY_BIN_DIR") {
        if !d.is_empty() {
            return Ok(PathBuf::from(d));
        }
    }
    app.path()
        .resolve("bin", tauri::path::BaseDirectory::Resource)
        .map_err(|e| format!("resolve bundled bin dir: {e}"))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn env(pairs: &[(&str, &str)]) -> impl Fn(&str) -> Option<String> {
        let m: std::collections::HashMap<String, String> = pairs
            .iter()
            .map(|(k, v)| (k.to_string(), v.to_string()))
            .collect();
        move |k: &str| m.get(k).cloned()
    }

    #[test]
    fn local_platform_matches_the_compile_target() {
        let expected = if cfg!(target_os = "linux") {
            Platform::LinuxX86_64
        } else {
            Platform::DarwinArm64
        };
        assert_eq!(Platform::local(), expected);
    }

    #[test]
    fn daemon_bin_defaults_to_the_bundled_binary() {
        let b = Bundle::new(PathBuf::from("/App/Contents/Resources/bin"));
        let directory = Platform::local().directory();
        assert_eq!(
            b.daemon_bin(&env(&[])),
            PathBuf::from(format!("/App/Contents/Resources/bin/{directory}/tariboyd"))
        );
    }

    // The same override the Go CLI honours. An isolated test run points it at a
    // wrapper script, which is the only way to satisfy CLAUDE.md's isolation rule.
    #[test]
    fn daemon_bin_honours_the_env_override() {
        let b = Bundle::new(PathBuf::from("/App/Contents/Resources/bin"));
        assert_eq!(
            b.daemon_bin(&env(&[("TARIBOY_DAEMON_BIN", "/tmp/wrap.sh")])),
            PathBuf::from("/tmp/wrap.sh")
        );
    }

    #[test]
    fn empty_override_is_ignored() {
        let b = Bundle::new(PathBuf::from("/bin"));
        let directory = Platform::local().directory();
        assert_eq!(
            b.daemon_bin(&env(&[("TARIBOY_DAEMON_BIN", "")])),
            PathBuf::from(format!("/bin/{directory}/tariboyd"))
        );
    }

    // The daemon looks for the shim next to itself, so the helpers share one dir.
    #[test]
    fn cli_and_helpers_sit_beside_the_daemon() {
        let b = Bundle::new(PathBuf::from("/bin"));
        let directory = Platform::local().directory();
        assert_eq!(
            b.shim_bin(),
            PathBuf::from(format!("/bin/{directory}/tariboy-shim"))
        );
    }

    #[test]
    // build.rs checks explicit overrides. This covers bare Cargo test runs,
    // where build.rs defaults TARIBOY_VERSION to the source value.
    fn runtime_version_matches_the_go_source() {
        let source =
            Path::new(env!("CARGO_MANIFEST_DIR")).join("../../internal/version/version.go");
        let go_source =
            std::fs::read_to_string(&source).expect("read ../../internal/version/version.go");
        let source_version = go_source
            .lines()
            .find_map(|line| {
                line.strip_prefix("const Version = \"")
                    .and_then(|value| value.strip_suffix('\"'))
            })
            .expect("parse internal/version.Version");

        assert_eq!(VERSION, source_version);
    }

    fn write_bundle(root: &Path, platform: Platform, version: &str) -> PathBuf {
        let dir = root.join(platform.directory());
        std::fs::create_dir_all(&dir).unwrap();
        for binary in BINARIES {
            std::fs::write(dir.join(binary), format!("#!/bin/sh\necho {version}\n")).unwrap();
        }
        std::fs::write(dir.join(VERSION_FILE), format!("{version}\n")).unwrap();
        dir
    }

    #[test]
    fn resolves_both_platform_layouts() {
        let root = tempfile::tempdir().unwrap();
        write_bundle(root.path(), Platform::DarwinArm64, VERSION);
        write_bundle(root.path(), Platform::LinuxX86_64, VERSION);
        let bundle = Bundle::new(root.path().to_path_buf());

        assert_eq!(
            bundle.platform(Platform::DarwinArm64).unwrap().dir,
            root.path().join("darwin-arm64")
        );
        assert_eq!(
            bundle.local_bundle().unwrap().dir,
            root.path().join(Platform::local().directory())
        );
        assert_eq!(
            bundle.platform(Platform::LinuxX86_64).unwrap().dir,
            root.path().join("linux-x86_64")
        );
    }

    // The native set needs no checksum file or installer: it never leaves
    // the Desktop host.
    #[test]
    fn rejects_missing_binary_or_version_mismatch() {
        let root = tempfile::tempdir().unwrap();
        let dir = write_bundle(root.path(), Platform::LinuxX86_64, VERSION);
        let bundle = Bundle::new(root.path().to_path_buf());

        std::fs::remove_file(dir.join("tariboy-tasks")).unwrap();
        assert!(bundle
            .platform(Platform::LinuxX86_64)
            .unwrap_err()
            .contains("tariboy-tasks"));
        std::fs::write(dir.join("tariboy-tasks"), b"binary").unwrap();
        std::fs::remove_file(dir.join("tariboy-mail")).unwrap();
        assert!(bundle
            .platform(Platform::LinuxX86_64)
            .unwrap_err()
            .contains("tariboy-mail"));
        std::fs::write(dir.join("tariboy-mail"), b"binary").unwrap();
        std::fs::remove_file(dir.join("tariboy-scripts")).unwrap();
        assert!(bundle
            .platform(Platform::LinuxX86_64)
            .unwrap_err()
            .contains("tariboy-scripts"));
        std::fs::write(dir.join("tariboy-scripts"), b"binary").unwrap();
        std::fs::remove_file(dir.join("tariboy-shim")).unwrap();
        assert!(bundle
            .platform(Platform::LinuxX86_64)
            .unwrap_err()
            .contains("tariboy-shim"));

        std::fs::write(dir.join("tariboy-shim"), b"binary").unwrap();
        assert!(bundle.platform(Platform::LinuxX86_64).is_ok());

        std::fs::write(dir.join(VERSION_FILE), b"other-version\n").unwrap();
        assert!(bundle
            .platform(Platform::LinuxX86_64)
            .unwrap_err()
            .contains("version mismatch"));
    }

    #[test]
    fn server_archive_follows_the_server_platform_and_machine_name() {
        for (platform, arch, target) in [
            ("Linux", "x86_64", "linux-x86_64"),
            ("Linux", "aarch64", "linux-aarch64"),
            ("Linux", "arm64", "linux-aarch64"),
            ("Darwin", "arm64", "darwin-arm64"),
        ] {
            assert_eq!(
                server_archive(platform, arch, "1.2.3").unwrap(),
                format!("tariboy_1.2.3_{target}.tar.gz")
            );
        }
        for (platform, arch) in [
            ("Darwin", "x86_64"),
            ("Linux", "armv7l"),
            ("FreeBSD", "x86_64"),
            ("", ""),
        ] {
            let error = server_archive(platform, arch, "1.2.3").unwrap_err();
            assert_eq!(
                error,
                format!("no published server build for {platform}/{arch}")
            );
        }
    }
}
