//! Platform binary bundles shipped inside the native Desktop package.

use std::path::{Path, PathBuf};

/// The canonical Go-side internal/version.Version string, injected by build.rs.
/// It remains separate from CARGO_PKG_VERSION so bundled payloads and the
/// running daemon are checked against the same release value.
pub const VERSION: &str = env!("TARIBOY_VERSION");
pub const BINARIES: [&str; 6] = [
    "tariboyd",
    "tariboy",
    "tariboy-tasks",
    "tariboy-mail",
    "tariboy-shim",
    "tariboy-plugin-telegram",
];
pub struct ManagedLink {
    pub name: &'static str,
    pub source: &'static str,
}

pub const MANAGED_LINKS: [ManagedLink; 8] = [
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
        name: "tariboy-shim",
        source: "tariboy-shim",
    },
    ManagedLink {
        name: "tariboy-plugin-telegram",
        source: "tariboy-plugin-telegram",
    },
];
const VERSION_FILE: &str = "VERSION";
const CHECKSUM_FILE: &str = "SHA256SUMS";
const INSTALLER_FILE: &str = "remote-install.sh";

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Platform {
    DarwinArm64,
    LinuxX86_64,
    LinuxAarch64,
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

    /// remote maps an SSH server's `uname -s`/`uname -m` to the set uploaded
    /// to it. Only pairs with a packaged build are supported.
    pub fn remote(platform: &str, arch: &str) -> Option<Self> {
        match (platform, arch) {
            ("Linux", "x86_64") => Some(Self::LinuxX86_64),
            ("Linux", "aarch64" | "arm64") => Some(Self::LinuxAarch64),
            ("Darwin", "arm64") => Some(Self::DarwinArm64),
            _ => None,
        }
    }

    /// is_remote reports a set used only on servers, so every validation of it
    /// requires the upload files.
    fn is_remote(self) -> bool {
        matches!(self, Self::LinuxX86_64 | Self::LinuxAarch64)
    }

    fn directory(self) -> &'static str {
        match self {
            Self::DarwinArm64 => "darwin-arm64",
            Self::LinuxX86_64 => "linux-x86_64",
            Self::LinuxAarch64 => "linux-aarch64",
        }
    }
}

#[derive(Debug, Clone)]
pub struct PlatformBundle {
    pub dir: PathBuf,
    pub version: String,
}

impl PlatformBundle {
    pub fn file(&self, name: &str) -> PathBuf {
        self.dir.join(name)
    }

    pub fn files_for_upload(&self) -> Vec<PathBuf> {
        let mut files = BINARIES
            .iter()
            .map(|name| self.file(name))
            .collect::<Vec<_>>();
        files.push(self.file(CHECKSUM_FILE));
        files.push(self.file(VERSION_FILE));
        files
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
        validate_platform_bundle(
            &self.bin_dir.join(platform.directory()),
            platform.is_remote(),
            VERSION,
        )
    }

    /// remote returns the validated upload set for a server reporting
    /// `uname -s` = `platform` and `uname -m` = `arch`.
    pub fn remote(&self, platform: &str, arch: &str) -> Result<PlatformBundle, String> {
        let target = Platform::remote(platform, arch)
            .ok_or_else(|| format!("no bundled server build for {platform}/{arch}"))?;
        validate_platform_bundle(&self.bin_dir.join(target.directory()), true, VERSION)
            .map_err(|error| format!("{} bundle: {error}", target.directory()))
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

fn validate_platform_bundle(
    dir: &Path,
    uploadable: bool,
    expected_version: &str,
) -> Result<PlatformBundle, String> {
    for name in BINARIES {
        let path = dir.join(name);
        if !path.is_file() {
            return Err(format!("bundled binary is missing: {}", path.display()));
        }
    }
    if uploadable {
        let checksums = dir.join(CHECKSUM_FILE);
        if !checksums.is_file() {
            return Err(format!(
                "bundled checksum file is missing: {}",
                checksums.display()
            ));
        }
        let installer = dir.join(INSTALLER_FILE);
        if !installer.is_file() {
            return Err(format!(
                "bundled remote installer is missing: {}",
                installer.display()
            ));
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
        version: actual.to_string(),
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
        if platform.is_remote() {
            std::fs::write(
                dir.join(CHECKSUM_FILE),
                BINARIES
                    .iter()
                    .map(|name| format!("00  {name}\n"))
                    .collect::<String>(),
            )
            .unwrap();
            std::fs::write(dir.join(INSTALLER_FILE), b"#!/bin/sh\n").unwrap();
        }
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
        let linux = bundle.platform(Platform::LinuxX86_64).unwrap();
        let uploads = linux.files_for_upload();
        assert_eq!(uploads.len(), 8);
        for payload in ["tariboy-tasks", "tariboy-mail"] {
            assert_eq!(
                uploads
                    .iter()
                    .filter(|path| path.ends_with(payload))
                    .count(),
                1
            );
        }
        assert!(!uploads.iter().any(|path| path.ends_with("ttasks")));
        assert!(!uploads.iter().any(|path| path.ends_with("tmail")));
        assert_eq!(linux.version, VERSION);
    }

    #[test]
    fn rejects_missing_binary_checksum_or_version_mismatch() {
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
        std::fs::remove_file(dir.join("tariboy-shim")).unwrap();
        assert!(bundle
            .platform(Platform::LinuxX86_64)
            .unwrap_err()
            .contains("tariboy-shim"));

        std::fs::write(dir.join("tariboy-shim"), b"binary").unwrap();
        std::fs::remove_file(dir.join(CHECKSUM_FILE)).unwrap();
        assert!(bundle
            .platform(Platform::LinuxX86_64)
            .unwrap_err()
            .contains(CHECKSUM_FILE));

        std::fs::write(dir.join(CHECKSUM_FILE), b"00  tariboyd\n").unwrap();
        std::fs::write(dir.join(INSTALLER_FILE), b"#!/bin/sh\n").unwrap();
        std::fs::write(dir.join(VERSION_FILE), b"other-version\n").unwrap();
        assert!(bundle
            .platform(Platform::LinuxX86_64)
            .unwrap_err()
            .contains("version mismatch"));
    }

    #[test]
    fn remote_platform_follows_the_server_platform_and_machine_name() {
        assert_eq!(
            Platform::remote("Linux", "x86_64"),
            Some(Platform::LinuxX86_64)
        );
        assert_eq!(
            Platform::remote("Linux", "aarch64"),
            Some(Platform::LinuxAarch64)
        );
        assert_eq!(
            Platform::remote("Linux", "arm64"),
            Some(Platform::LinuxAarch64)
        );
        assert_eq!(
            Platform::remote("Darwin", "arm64"),
            Some(Platform::DarwinArm64)
        );
        assert_eq!(Platform::remote("Darwin", "x86_64"), None);
        assert_eq!(Platform::remote("Linux", "armv7l"), None);
        assert_eq!(Platform::remote("FreeBSD", "x86_64"), None);
        assert_eq!(Platform::remote("", ""), None);
    }

    #[test]
    fn remote_darwin_bundle_requires_upload_files_but_local_does_not() {
        let root = tempfile::tempdir().unwrap();
        let dir = write_bundle(root.path(), Platform::DarwinArm64, VERSION);
        let bundle = Bundle::new(root.path().to_path_buf());
        assert!(bundle.platform(Platform::DarwinArm64).is_ok());
        let error = bundle.remote("Darwin", "arm64").unwrap_err();
        assert!(error.contains("darwin-arm64"), "{error}");
        assert!(error.contains(CHECKSUM_FILE), "{error}");

        std::fs::write(dir.join(CHECKSUM_FILE), b"00  tariboyd\n").unwrap();
        std::fs::write(dir.join(INSTALLER_FILE), b"#!/bin/sh\n").unwrap();
        let remote = bundle.remote("Darwin", "arm64").unwrap();
        assert_eq!(remote.dir, dir);
        assert_eq!(remote.files_for_upload().len(), 8);
        assert!(bundle
            .remote("Darwin", "x86_64")
            .unwrap_err()
            .contains("Darwin/x86_64"));
    }

    #[test]
    fn remote_bundle_is_picked_by_server_arch() {
        let root = tempfile::tempdir().unwrap();
        write_bundle(root.path(), Platform::LinuxX86_64, VERSION);
        let arm = write_bundle(root.path(), Platform::LinuxAarch64, VERSION);
        let bundle = Bundle::new(root.path().to_path_buf());

        assert_eq!(
            bundle.remote("Linux", "x86_64").unwrap().dir,
            root.path().join("linux-x86_64")
        );
        assert_eq!(bundle.remote("Linux", "aarch64").unwrap().dir, arm);
        assert_eq!(
            bundle
                .remote("Linux", "aarch64")
                .unwrap()
                .files_for_upload()
                .len(),
            8
        );
        assert!(bundle
            .remote("Linux", "armv7l")
            .unwrap_err()
            .contains("armv7l"));

        std::fs::remove_file(arm.join(INSTALLER_FILE)).unwrap();
        let error = bundle.remote("Linux", "aarch64").unwrap_err();
        assert!(error.contains("linux-aarch64"), "{error}");
        assert!(error.contains(INSTALLER_FILE), "{error}");
    }
}
