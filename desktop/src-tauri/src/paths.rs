//! Mirror of internal/paths.Resolve. The app, the Go daemon and the `tariboy`
//! CLI must agree on the base data dir and the runtime dir (control socket,
//! pidfile, log) or the app would probe one socket while the daemon binds
//! another — and then start a SECOND daemon on a shared base dir, which
//! CLAUDE.md forbids outright (two daemons reconcile the same agents: duplicated
//! iterations, reaped sessions).
//!
//! Only Resolve's semantics are mirrored, not `New`/`runtimeFor`: like the CLI,
//! the desktop app always takes the production path.

use std::path::{Path, PathBuf};

const MAX_SSH_CONTROL_DIR_BYTES: usize = 40;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Paths {
    pub base: PathBuf,
    pub runtime: PathBuf,
    /// Tauri-owned application data. `resolve` uses `base` as a safe fallback;
    /// setup replaces it with `AppHandle::path().app_data_dir()`.
    pub app_data: PathBuf,
}

impl Paths {
    pub fn with_app_data(mut self, app_data: PathBuf) -> Self {
        self.app_data = app_data;
        self
    }
    pub fn socket(&self) -> PathBuf {
        self.runtime.join("tariboyd.sock")
    }
    pub fn pid_file(&self) -> PathBuf {
        self.runtime.join("tariboyd.pid")
    }
    pub fn log_file(&self) -> PathBuf {
        self.runtime.join("tariboyd.log")
    }
    pub fn hosts_file(&self) -> PathBuf {
        self.app_data.join("hosts.json")
    }
    pub fn ssh_control_dir(&self) -> PathBuf {
        use std::os::unix::ffi::OsStrExt;

        let candidate = self.runtime.join("ssh");
        if candidate.as_os_str().as_bytes().len() <= MAX_SSH_CONTROL_DIR_BYTES {
            return candidate;
        }
        PathBuf::from(format!("/tmp/tariboy-{}", unsafe { libc::geteuid() }))
    }
}

/// Create or validate the predictable per-user control directory. A path below
/// /tmp is safe only when it is a real directory owned by this effective uid;
/// rejecting symlinks and foreign owners prevents another local user from
/// redirecting SSH control sockets.
pub fn prepare_ssh_control_dir(path: &Path) -> Result<(), String> {
    use std::os::unix::fs::{DirBuilderExt, MetadataExt, PermissionsExt};

    let parent = path
        .parent()
        .ok_or_else(|| format!("SSH control directory has no parent: {}", path.display()))?;
    std::fs::create_dir_all(parent)
        .map_err(|error| format!("create SSH control parent {}: {error}", parent.display()))?;
    let mut builder = std::fs::DirBuilder::new();
    builder.mode(0o700);
    if let Err(error) = builder.create(path) {
        if error.kind() != std::io::ErrorKind::AlreadyExists {
            return Err(format!(
                "create SSH control directory {}: {error}",
                path.display()
            ));
        }
    }
    let metadata = std::fs::symlink_metadata(path)
        .map_err(|error| format!("inspect SSH control directory {}: {error}", path.display()))?;
    if !metadata.file_type().is_dir() {
        return Err(format!(
            "SSH control directory is not a real directory: {}",
            path.display()
        ));
    }
    let expected_uid = unsafe { libc::geteuid() };
    if metadata.uid() != expected_uid {
        return Err(format!(
            "SSH control directory {} is owned by uid {}, expected {}",
            path.display(),
            metadata.uid(),
            expected_uid
        ));
    }
    std::fs::set_permissions(path, std::fs::Permissions::from_mode(0o700))
        .map_err(|error| format!("secure SSH control directory {}: {error}", path.display()))
}

/// resolve picks the base dir from $TARIBOY_BASE_DIR (else $HOME/.tariboy)
/// and the runtime dir from $TARIBOY_RUNTIME_DIR (else $HOME/.tariboyd,
/// falling back to base when HOME is unset). An empty variable counts as unset,
/// matching Go's `getenv(k) == ""` checks.
pub fn resolve(getenv: &dyn Fn(&str) -> Option<String>) -> Result<Paths, String> {
    let get = |k: &str| getenv(k).filter(|v| !v.is_empty());
    let home = get("HOME");
    let base = match get("TARIBOY_BASE_DIR") {
        Some(b) => PathBuf::from(b),
        None => match &home {
            Some(h) => Path::new(h).join(".tariboy"),
            None => {
                return Err(
                    "cannot resolve base dir: neither TARIBOY_BASE_DIR nor HOME is set".into(),
                )
            }
        },
    };
    let runtime = match get("TARIBOY_RUNTIME_DIR") {
        Some(r) => PathBuf::from(r),
        None => match &home {
            Some(h) => Path::new(h).join(".tariboyd"),
            None => base.clone(),
        },
    };
    Ok(Paths {
        app_data: base.clone(),
        base,
        runtime,
    })
}

/// env_getter reads the real process environment.
pub fn env_getter(key: &str) -> Option<String> {
    std::env::var(key).ok()
}

/// Keep normal Tauri path resolution in production while allowing the packaged
/// smoke to prove that hosts, logs, and SSH control sockets never touch the
/// operator's real application data.
pub fn app_data_dir(default: PathBuf, getenv: &dyn Fn(&str) -> Option<String>) -> PathBuf {
    getenv("TARIBOY_DESKTOP_APP_DATA_DIR")
        .filter(|value| !value.is_empty())
        .map(PathBuf::from)
        .unwrap_or(default)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    fn env(pairs: &[(&str, &str)]) -> impl Fn(&str) -> Option<String> {
        let m: HashMap<String, String> =
            pairs.iter().map(|(k, v)| (k.to_string(), v.to_string())).collect();
        move |k: &str| m.get(k).cloned()
    }

    // Mirrors internal/paths/paths_test.go:TestResolveExplicitBase.
    #[test]
    fn explicit_base_dir_wins() {
        let p = resolve(&env(&[("TARIBOY_BASE_DIR", "/tmp/sa"), ("HOME", "/home/u")])).unwrap();
        assert_eq!(p.base, PathBuf::from("/tmp/sa"));
    }

    // Mirrors TestResolveHomeFallback.
    #[test]
    fn falls_back_to_home() {
        let p = resolve(&env(&[("HOME", "/home/u")])).unwrap();
        assert_eq!(p.base, PathBuf::from("/home/u/.tariboy"));
    }

    // Mirrors TestResolveNoEnv.
    #[test]
    fn errors_without_base_or_home() {
        assert!(resolve(&env(&[])).is_err());
    }

    // Mirrors TestResolveRuntimeDefaultsToTariboyd: the runtime dir is
    // ~/.tariboyd, NOT under base — sockets must stay short.
    #[test]
    fn runtime_defaults_to_dot_tariboyd() {
        let p = resolve(&env(&[("HOME", "/home/u")])).unwrap();
        assert_eq!(p.runtime, PathBuf::from("/home/u/.tariboyd"));
        assert_eq!(p.socket(), PathBuf::from("/home/u/.tariboyd/tariboyd.sock"));
        assert_eq!(p.pid_file(), PathBuf::from("/home/u/.tariboyd/tariboyd.pid"));
        assert_eq!(p.log_file(), PathBuf::from("/home/u/.tariboyd/tariboyd.log"));
    }

    // Mirrors TestResolveRuntimeEnvOverride — the hook every isolated test run
    // depends on.
    #[test]
    fn runtime_env_overrides() {
        let p = resolve(&env(&[("HOME", "/home/u"), ("TARIBOY_RUNTIME_DIR", "/tmp/rt")])).unwrap();
        assert_eq!(p.socket(), PathBuf::from("/tmp/rt/tariboyd.sock"));
    }

    // An empty value is "unset", matching Go's `getenv(k) == ""` checks.
    #[test]
    fn empty_values_are_treated_as_unset() {
        let p = resolve(&env(&[
            ("TARIBOY_BASE_DIR", ""),
            ("TARIBOY_RUNTIME_DIR", ""),
            ("HOME", "/home/u"),
        ]))
        .unwrap();
        assert_eq!(p.base, PathBuf::from("/home/u/.tariboy"));
        assert_eq!(p.runtime, PathBuf::from("/home/u/.tariboyd"));
    }

    // HOME unset but base given: runtime has nowhere short to go, so it follows
    // base (Go does the same and lets the socket-length check complain later).
    #[test]
    fn runtime_follows_base_without_home() {
        let p = resolve(&env(&[("TARIBOY_BASE_DIR", "/tmp/sa")])).unwrap();
        assert_eq!(p.runtime, PathBuf::from("/tmp/sa"));
    }

    #[test]
    fn native_host_state_uses_persistent_and_runtime_roots() {
        let p = resolve(&env(&[("HOME", "/home/u")]))
            .unwrap()
            .with_app_data(PathBuf::from("/native/app-data"));
        assert_eq!(p.hosts_file(), PathBuf::from("/native/app-data/hosts.json"));
        assert_eq!(p.ssh_control_dir(), PathBuf::from("/home/u/.tariboyd/ssh"));
    }

    #[test]
    fn ssh_control_socket_fits_macos_unix_path_with_openssh_suffix() {
        use std::os::unix::ffi::OsStrExt;

        const MACOS_SUN_PATH_BYTES: usize = 104;
        const OPENSSH_TEMP_SUFFIX_BUDGET: usize = 24;
        let p = resolve(&env(&[
            ("HOME", "/Users/a.very.long.corporate.username"),
            (
                "TARIBOY_RUNTIME_DIR",
                "/Users/a.very.long.corporate.username/Library/Application Support/app.tariboy.desktop/runtime",
            ),
        ]))
        .unwrap()
        .with_app_data(PathBuf::from(
            "/Users/a.very.long.corporate.username/Library/Application Support/app.tariboy.desktop",
        ));
        let socket = crate::ssh::control_socket(
            &p.ssh_control_dir(),
            "f94d49b9-33cf-4802-8727-789d9ff61eed",
        )
        .unwrap();
        assert_eq!(
            p.ssh_control_dir(),
            PathBuf::from(format!("/tmp/tariboy-{}", unsafe { libc::geteuid() }))
        );
        let required = socket.as_os_str().as_bytes().len()
            + 1 // "." before OpenSSH's temporary suffix
            + OPENSSH_TEMP_SUFFIX_BUDGET
            + 1; // terminating NUL

        assert!(
            required <= MACOS_SUN_PATH_BYTES,
            "SSH control path needs {required} bytes including OpenSSH suffix, macOS allows {MACOS_SUN_PATH_BYTES}: {}",
            socket.display()
        );
    }

    #[test]
    fn ssh_control_directory_is_owner_only_and_rejects_symlinks() {
        use std::os::unix::fs::{symlink, PermissionsExt};

        let root = tempfile::tempdir().unwrap();
        let control = root.path().join("control");
        std::fs::create_dir(&control).unwrap();
        std::fs::set_permissions(&control, std::fs::Permissions::from_mode(0o755)).unwrap();

        prepare_ssh_control_dir(&control).unwrap();
        assert_eq!(
            std::fs::symlink_metadata(&control)
                .unwrap()
                .permissions()
                .mode()
                & 0o777,
            0o700
        );

        let link = root.path().join("redirect");
        symlink(&control, &link).unwrap();
        let error = prepare_ssh_control_dir(&link).unwrap_err();
        assert!(
            error.contains("not a real directory"),
            "unexpected error: {error}"
        );
    }

    #[test]
    fn desktop_app_data_can_be_isolated_for_packaged_smoke() {
        let default = PathBuf::from("/native/app-data");
        assert_eq!(
            app_data_dir(
                default.clone(),
                &env(&[("TARIBOY_DESKTOP_APP_DATA_DIR", "/tmp/smoke-app-data")])
            ),
            PathBuf::from("/tmp/smoke-app-data")
        );
        assert_eq!(
            app_data_dir(
                default.clone(),
                &env(&[("TARIBOY_DESKTOP_APP_DATA_DIR", "")])
            ),
            default
        );
    }
}
