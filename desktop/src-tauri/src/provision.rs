//! Transactional installation of the bundled Linux release through one
//! authenticated OpenSSH ControlMaster.

use crate::bundle::PlatformBundle;
use crate::ssh::{self, ControlCommand, Operation, OutputEvent, OutputSink, UploadCommand};
use serde::{Deserialize, Serialize};
use std::path::{Path, PathBuf};
use std::time::Duration;

const CREATE_STAGE_SCRIPT: &[u8] = br#"set -eu
: "${HOME:?HOME is required}"
staging=${1-}
case "$staging" in
  .stage-*) ;;
  *) echo "invalid staging basename" >&2; exit 64 ;;
esac
case "${staging#.stage-}" in ""|*[!A-Za-z0-9-]*) exit 64 ;; esac
root=$HOME/.local/lib/tariboy
umask 077
mkdir -p "$root"
test ! -e "$root/$staging"
mkdir "$root/$staging"
"#;

const CLEANUP_STAGE_SCRIPT: &[u8] = br#"set -eu
: "${HOME:?HOME is required}"
staging=${1-}
case "$staging" in
  .stage-*) ;;
  *) exit 64 ;;
esac
case "${staging#.stage-}" in ""|*[!A-Za-z0-9-]*) exit 64 ;; esac
rm -rf "$HOME/.local/lib/tariboy/$staging"
"#;

const STATUS_SCRIPT: &[u8] = br#"set -eu
: "${HOME:?HOME is required}"
expected=${1-}
port=${2-}
case "$expected" in ""|*[!A-Za-z0-9._-]*|.*|*/*) exit 64 ;; esac
case "$port" in ""|*[!0-9]*) exit 64 ;; esac
cli=$HOME/.local/bin/tariboy
test "$("$cli" --version)" = "$expected"
if ! "$cli" daemon status --json >/dev/null 2>&1; then
  TARIBOY_HTTP_ADDR="127.0.0.1:$port" "$cli" daemon start >/dev/null
fi
exec "$cli" daemon status --json
"#;

const RESTART_STATUS_SCRIPT: &[u8] = br#"set -eu
: "${HOME:?HOME is required}"
expected=${1-}
port=${2-}
case "$expected" in ""|*[!A-Za-z0-9._-]*|.*|*/*) exit 64 ;; esac
case "$port" in ""|*[!0-9]*) exit 64 ;; esac
cli=$HOME/.local/bin/tariboy
test "$("$cli" --version)" = "$expected"
"$cli" daemon stop >/dev/null
TARIBOY_HTTP_ADDR="127.0.0.1:$port" "$cli" daemon start >/dev/null
exec "$cli" daemon status --json
"#;

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Result {
    pub state: String,
    pub version: String,
    pub http_addr: String,
    pub message: String,
}

pub struct VersionSwitch<'a> {
    pub next: &'a str,
    pub previous: &'a str,
}

#[derive(Debug, Deserialize)]
pub struct StagedRelease {
    pub previous: String,
    version: String,
}

#[derive(Debug, Deserialize)]
struct DaemonStatus {
    version: String,
    #[serde(default)]
    http_addr: String,
}

#[derive(Debug, Deserialize)]
struct Activation {
    previous: String,
    version: String,
}

trait Remote {
    fn run(
        &self,
        remote_args: &[String],
        stdin: &[u8],
        timeout: Duration,
        sink: OutputSink,
    ) -> std::result::Result<String, ssh::SshError>;

    fn upload(
        &self,
        local_files: &[PathBuf],
        remote_dir: &str,
        timeout: Duration,
        sink: OutputSink,
    ) -> std::result::Result<(), ssh::SshError>;
}

struct SshRemote<'a> {
    transport: &'a ssh::Transport,
    operation: &'a Operation,
    alias: &'a str,
    socket: &'a Path,
}

impl Remote for SshRemote<'_> {
    fn run(
        &self,
        remote_args: &[String],
        stdin: &[u8],
        timeout: Duration,
        sink: OutputSink,
    ) -> std::result::Result<String, ssh::SshError> {
        let args = remote_args.iter().map(String::as_str).collect::<Vec<_>>();
        self.transport.run_control(
            self.operation,
            ControlCommand {
                alias: self.alias,
                socket: self.socket,
                remote_args: &args,
                stdin,
                timeout,
            },
            sink,
        )
    }

    fn upload(
        &self,
        local_files: &[PathBuf],
        remote_dir: &str,
        timeout: Duration,
        sink: OutputSink,
    ) -> std::result::Result<(), ssh::SshError> {
        self.transport.upload(
            self.operation,
            UploadCommand {
                alias: self.alias,
                socket: self.socket,
                local_files,
                remote_dir,
                timeout,
            },
            sink,
        )
    }
}

pub fn run(
    transport: &ssh::Transport,
    operation: &Operation,
    alias: &str,
    socket: &Path,
    bundle: &PlatformBundle,
    remote_port: u16,
    sink: OutputSink,
) -> std::result::Result<Result, ssh::SshError> {
    let remote = SshRemote {
        transport,
        operation,
        alias,
        socket,
    };
    run_with(&remote, operation, bundle, remote_port, sink)
}

pub fn stage(
    transport: &ssh::Transport,
    operation: &Operation,
    alias: &str,
    socket: &Path,
    bundle: &PlatformBundle,
    sink: OutputSink,
) -> std::result::Result<StagedRelease, ssh::SshError> {
    let remote = SshRemote {
        transport,
        operation,
        alias,
        socket,
    };
    stage_with(&remote, operation, bundle, sink)
}

fn stage_with(
    remote: &dyn Remote,
    operation: &Operation,
    bundle: &PlatformBundle,
    sink: OutputSink,
) -> std::result::Result<StagedRelease, ssh::SshError> {
    validate_version(&bundle.version)?;
    let staging = format!(".stage-{}", uuid::Uuid::new_v4().simple());
    phase(operation, &sink, "stage");
    remote.run(
        &shell_args(&staging),
        CREATE_STAGE_SCRIPT,
        Duration::from_secs(20),
        sink.clone(),
    )?;
    let result = (|| {
        phase(operation, &sink, "upload");
        let mut files = bundle.files_for_upload();
        files.push(bundle.file("remote-install.sh"));
        remote.upload(
            &files,
            &format!("~/.local/lib/tariboy/{staging}/"),
            Duration::from_secs(120),
            sink.clone(),
        )?;
        phase(operation, &sink, "stage_release");
        let output = remote.run(
            &[
                "sh".into(),
                "-s".into(),
                "--".into(),
                bundle.version.clone(),
                staging.clone(),
                "stage".into(),
            ],
            include_bytes!("remote-install.sh"),
            Duration::from_secs(60),
            sink.clone(),
        )?;
        let staged: StagedRelease =
            serde_json::from_str(&output).map_err(|error| ssh::SshError {
                code: "remote_stage_invalid".into(),
                message: format!("parse remote stage result: {error}"),
            })?;
        validate_version(&staged.version)?;
        if staged.version != bundle.version {
            return Err(ssh::SshError {
                code: "remote_stage_invalid".into(),
                message: "remote stage returned a different version".into(),
            });
        }
        if !staged.previous.is_empty() {
            validate_version(&staged.previous)?;
        }
        Ok(staged)
    })();
    if result.is_err() {
        let _ = remote.run(
            &shell_args(&staging),
            CLEANUP_STAGE_SCRIPT,
            Duration::from_secs(10),
            sink,
        );
    }
    result
}

pub fn activate_and_restart(
    transport: &ssh::Transport,
    operation: &Operation,
    alias: &str,
    socket: &Path,
    versions: VersionSwitch<'_>,
    remote_port: u16,
    sink: OutputSink,
) -> std::result::Result<Result, ssh::SshError> {
    validate_version(versions.next)?;
    if !versions.previous.is_empty() {
        validate_version(versions.previous)?;
    }
    let remote = SshRemote {
        transport,
        operation,
        alias,
        socket,
    };
    activate_restart_with(
        &remote,
        operation,
        versions.next,
        versions.previous,
        remote_port,
        sink,
    )
}

fn activate_restart_with(
    remote: &dyn Remote,
    operation: &Operation,
    version: &str,
    previous_version: &str,
    remote_port: u16,
    sink: OutputSink,
) -> std::result::Result<Result, ssh::SshError> {
    validate_version(version)?;
    if !previous_version.is_empty() {
        validate_version(previous_version)?;
    }
    let activation = match activate(remote, operation, version, previous_version, sink.clone()) {
        Ok(activation) => activation,
        Err(error) => {
            return rollback_after_update_failure(
                remote,
                operation,
                previous_version,
                version,
                remote_port,
                sink,
                error,
            );
        }
    };
    if activation.previous != previous_version {
        return rollback_after_update_failure(
            remote,
            operation,
            &activation.previous,
            version,
            remote_port,
            sink,
            ssh::SshError {
                code: "remote_activate_invalid".into(),
                message: format!(
                    "remote activation reported previous version {:?}, expected {:?}",
                    activation.previous, previous_version
                ),
            },
        );
    }
    phase(operation, &sink, "restart");
    match restart(remote, version, remote_port, sink.clone()) {
        Ok(result) if result.version == version => Ok(result),
        Ok(result) => rollback_after_update_failure(
            remote,
            operation,
            &activation.previous,
            version,
            remote_port,
            sink,
            ssh::SshError {
                code: "version_mismatch".into(),
                message: format!(
                    "remote daemon reports {}, expected {version}",
                    result.version
                ),
            },
        ),
        Err(error) => rollback_after_update_failure(
            remote,
            operation,
            &activation.previous,
            version,
            remote_port,
            sink,
            error,
        ),
    }
}

fn activate(
    remote: &dyn Remote,
    operation: &Operation,
    version: &str,
    expected_previous: &str,
    sink: OutputSink,
) -> std::result::Result<Activation, ssh::SshError> {
    validate_version(version)?;
    if !expected_previous.is_empty() {
        validate_version(expected_previous)?;
    }
    phase(operation, &sink, "activate");
    let token = format!(".stage-{}", uuid::Uuid::new_v4().simple());
    let output = remote.run(
        &[
            "sh".into(),
            "-s".into(),
            "--".into(),
            version.into(),
            token,
            "activate".into(),
            expected_previous.into(),
        ],
        include_bytes!("remote-install.sh"),
        Duration::from_secs(60),
        sink,
    )?;
    let activation: Activation = serde_json::from_str(&output).map_err(|error| ssh::SshError {
        code: "remote_activate_invalid".into(),
        message: format!("parse remote activation result: {error}"),
    })?;
    if activation.version != version {
        return Err(ssh::SshError {
            code: "remote_activate_invalid".into(),
            message: "remote activation returned a different version".into(),
        });
    }
    if !activation.previous.is_empty() {
        validate_version(&activation.previous)?;
    }
    Ok(activation)
}

fn restart(
    remote: &dyn Remote,
    version: &str,
    remote_port: u16,
    sink: OutputSink,
) -> std::result::Result<Result, ssh::SshError> {
    validate_version(version)?;
    let output = remote.run(
        &[
            "sh".into(),
            "-s".into(),
            "--".into(),
            version.into(),
            remote_port.to_string(),
        ],
        RESTART_STATUS_SCRIPT,
        Duration::from_secs(30),
        sink,
    )?;
    parse_result(&output, version)
}

fn rollback_after_update_failure(
    remote: &dyn Remote,
    operation: &Operation,
    previous_version: &str,
    failed_version: &str,
    remote_port: u16,
    sink: OutputSink,
    original: ssh::SshError,
) -> std::result::Result<Result, ssh::SshError> {
    if previous_version.is_empty() {
        return Err(original);
    }
    phase(operation, &sink, "rollback");
    let activation = activate(
        remote,
        operation,
        previous_version,
        failed_version,
        sink.clone(),
    );
    let restart = restart(remote, previous_version, remote_port, sink);
    let rollback_error = match restart {
        Ok(result) if result.version == previous_version => None,
        Ok(result) => Some(format!(
            "daemon reported {}, expected {previous_version}",
            result.version
        )),
        Err(error) => Some(error.message),
    };
    Err(match rollback_error {
        None => original,
        Some(rollback_error) => ssh::SshError {
            code: original.code,
            message: format!(
                "{}; rollback to {} also failed: {}",
                original.message,
                previous_version,
                match activation {
                    Ok(_) => rollback_error,
                    Err(error) => format!("{}; restart: {rollback_error}", error.message),
                }
            ),
        },
    })
}

fn run_with(
    remote: &dyn Remote,
    operation: &Operation,
    bundle: &PlatformBundle,
    remote_port: u16,
    sink: OutputSink,
) -> std::result::Result<Result, ssh::SshError> {
    validate_version(&bundle.version)?;
    let staging = format!(".stage-{}", uuid::Uuid::new_v4().simple());
    phase(operation, &sink, "stage");
    remote.run(
        &shell_args(&staging),
        CREATE_STAGE_SCRIPT,
        Duration::from_secs(20),
        sink.clone(),
    )?;

    let result = (|| {
        phase(operation, &sink, "upload");
        let mut files = bundle.files_for_upload();
        files.push(bundle.file("remote-install.sh"));
        remote.upload(
            &files,
            &format!("~/.local/lib/tariboy/{staging}/"),
            Duration::from_secs(120),
            sink.clone(),
        )?;

        phase(operation, &sink, "verify_install");
        remote.run(
            &[
                "sh".into(),
                "-s".into(),
                "--".into(),
                bundle.version.clone(),
                staging.clone(),
            ],
            include_bytes!("remote-install.sh"),
            Duration::from_secs(60),
            sink.clone(),
        )?;

        phase(operation, &sink, "status");
        let output = remote.run(
            &[
                "sh".into(),
                "-s".into(),
                "--".into(),
                bundle.version.clone(),
                remote_port.to_string(),
            ],
            STATUS_SCRIPT,
            Duration::from_secs(30),
            sink.clone(),
        )?;
        parse_result(&output, &bundle.version)
    })();

    if result.is_err() {
        let _ = remote.run(
            &shell_args(&staging),
            CLEANUP_STAGE_SCRIPT,
            Duration::from_secs(10),
            sink,
        );
    }
    result
}

fn shell_args(value: &str) -> Vec<String> {
    vec!["sh".into(), "-s".into(), "--".into(), value.to_string()]
}

fn phase(operation: &Operation, sink: &OutputSink, name: &str) {
    sink(OutputEvent {
        operation_id: operation.id.clone(),
        host_id: operation.host_id.clone(),
        stream: "phase".into(),
        text: name.into(),
        prompt: None,
    });
}

fn parse_result(output: &str, expected: &str) -> std::result::Result<Result, ssh::SshError> {
    let mut documents = serde_json::Deserializer::from_str(output).into_iter::<DaemonStatus>();
    let status = documents
        .next()
        .ok_or_else(|| invalid_status("remote status returned no JSON"))?
        .map_err(|error| invalid_status(&format!("parse remote status: {error}")))?;
    if documents.next().is_some() {
        return Err(invalid_status(
            "remote status returned more than one JSON document",
        ));
    }
    let mismatch = status.version != expected;
    Ok(Result {
        state: if mismatch { "degraded" } else { "ready" }.into(),
        version: status.version,
        http_addr: status.http_addr,
        message: if mismatch {
            "version_mismatch".into()
        } else {
            String::new()
        },
    })
}

fn invalid_status(message: &str) -> ssh::SshError {
    ssh::SshError {
        code: "remote_status_invalid".into(),
        message: message.into(),
    }
}

fn validate_version(version: &str) -> std::result::Result<(), ssh::SshError> {
    if version.is_empty()
        || version.len() > 128
        || version.starts_with('.')
        || !version
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'.' | b'_' | b'-'))
    {
        return Err(ssh::SshError {
            code: "invalid_version".into(),
            message: "version must be 1-128 ASCII letters, digits, '.', '_' or '-'".into(),
        });
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::bundle::BINARIES;
    use std::sync::Mutex;
    use std::{fs, process::Command};

    #[derive(Default)]
    struct FakeRemote {
        calls: Mutex<Vec<String>>,
        fail: Mutex<Option<String>>,
        fail_restart_version: Mutex<Option<String>>,
        invalid_activate_version: Mutex<Option<String>>,
        wrong_restart_version: Mutex<Option<String>>,
        activation_previous: Mutex<Option<String>>,
        status: Mutex<String>,
    }

    impl FakeRemote {
        fn failing(step: &str) -> Self {
            Self {
                fail: Mutex::new(Some(step.into())),
                status: Mutex::new(r#"{"version":"0.9.0","http_addr":"127.0.0.1:9990"}"#.into()),
                ..Self::default()
            }
        }

        fn calls(&self) -> Vec<String> {
            self.calls.lock().unwrap().clone()
        }
    }

    impl Remote for FakeRemote {
        fn run(
            &self,
            args: &[String],
            stdin: &[u8],
            _timeout: Duration,
            _sink: OutputSink,
        ) -> std::result::Result<String, ssh::SshError> {
            let kind = if stdin == CREATE_STAGE_SCRIPT {
                "stage"
            } else if stdin == CLEANUP_STAGE_SCRIPT {
                "cleanup"
            } else if stdin == include_bytes!("remote-install.sh") {
                match args.get(5).map(String::as_str) {
                    Some("stage") => "stage_release",
                    Some("activate") => "activate",
                    _ => "verify_install",
                }
            } else if stdin == RESTART_STATUS_SCRIPT {
                "restart"
            } else {
                "status"
            };
            self.calls
                .lock()
                .unwrap()
                .push(format!("run:{kind}:{}", args.join("|")));
            if self.fail.lock().unwrap().as_deref() == Some(kind) {
                return Err(ssh::SshError {
                    code: "test_failure".into(),
                    message: kind.into(),
                });
            }
            if kind == "restart"
                && self.fail_restart_version.lock().unwrap().as_deref()
                    == args.get(3).map(String::as_str)
            {
                return Err(ssh::SshError {
                    code: "restart_failed".into(),
                    message: format!("restart {} failed", args[3]),
                });
            }
            if kind == "activate" {
                let version = args.get(3).cloned().unwrap_or_default();
                let previous = self
                    .activation_previous
                    .lock()
                    .unwrap()
                    .clone()
                    .unwrap_or_else(|| if version == "old" { "new" } else { "old" }.into());
                if args.get(6).map(String::as_str) != Some(previous.as_str()) {
                    return Err(ssh::SshError {
                        code: "release_conflict".into(),
                        message: format!("current {previous}, expected {:?}", args.get(6)),
                    });
                }
                if self.invalid_activate_version.lock().unwrap().as_deref()
                    == Some(version.as_str())
                {
                    return Ok("verification noise\n{}".into());
                }
                return Ok(format!(
                    r#"{{"previous":"{previous}","version":"{version}"}}"#
                ));
            }
            if kind == "status" {
                Ok(self.status.lock().unwrap().clone())
            } else if kind == "restart" {
                let version = args.get(3).cloned().unwrap_or_default();
                let reported = self
                    .wrong_restart_version
                    .lock()
                    .unwrap()
                    .clone()
                    .unwrap_or_else(|| version.clone());
                Ok(format!(
                    r#"{{"version":"{reported}","http_addr":"127.0.0.1:9990"}}"#
                ))
            } else if kind == "stage_release" {
                let version = args.get(3).cloned().unwrap_or_default();
                Ok(format!(r#"{{"previous":"old","version":"{version}"}}"#))
            } else {
                Ok(String::new())
            }
        }

        fn upload(
            &self,
            files: &[PathBuf],
            remote_dir: &str,
            _timeout: Duration,
            _sink: OutputSink,
        ) -> std::result::Result<(), ssh::SshError> {
            self.calls.lock().unwrap().push(format!(
                "upload:{}:{}",
                files
                    .iter()
                    .map(|path| path.file_name().unwrap().to_string_lossy())
                    .collect::<Vec<_>>()
                    .join(","),
                remote_dir
            ));
            if self.fail.lock().unwrap().as_deref() == Some("upload") {
                return Err(ssh::SshError {
                    code: "test_failure".into(),
                    message: "upload".into(),
                });
            }
            Ok(())
        }
    }

    fn fixture() -> (tempfile::TempDir, PlatformBundle, Operation, OutputSink) {
        let dir = tempfile::tempdir().unwrap();
        for name in BINARIES {
            std::fs::write(dir.path().join(name), b"bin").unwrap();
        }
        for name in ["SHA256SUMS", "VERSION", "remote-install.sh"] {
            std::fs::write(dir.path().join(name), b"data").unwrap();
        }
        let bundle = PlatformBundle {
            dir: dir.path().to_path_buf(),
            version: "0.9.0".into(),
        };
        let (_tx, rx) = std::sync::mpsc::sync_channel(1);
        let operation = Operation {
            id: "op".into(),
            host_id: "host".into(),
            replies: rx,
            cancel: ssh::CancelToken::default(),
        };
        let sink: OutputSink = std::sync::Arc::new(|_| {});
        (dir, bundle, operation, sink)
    }

    #[test]
    fn exact_phase_order_and_versioned_install_contract() {
        let (_dir, bundle, operation, _sink) = fixture();
        let remote = FakeRemote {
            status: Mutex::new(r#"{"version":"0.9.0","http_addr":"127.0.0.1:9990"}"#.into()),
            ..FakeRemote::default()
        };
        let phases = std::sync::Arc::new(Mutex::new(Vec::new()));
        let captured = phases.clone();
        let sink: OutputSink = std::sync::Arc::new(move |event| {
            if event.stream == "phase" {
                captured.lock().unwrap().push(event.text);
            }
        });
        let result = run_with(&remote, &operation, &bundle, 9990, sink).unwrap();
        assert_eq!(result.version, "0.9.0");
        assert_eq!(
            *phases.lock().unwrap(),
            ["stage", "upload", "verify_install", "status"]
        );
        let calls = remote.calls();
        assert_eq!(calls.len(), 4);
        assert!(calls[0].starts_with("run:stage:sh|-s|--|.stage-"));
        assert!(calls[1].starts_with(
            "upload:tariboyd,tariboy,tariboy-shim,tariboy-plugin-telegram,SHA256SUMS,VERSION,remote-install.sh:~/.local/lib/tariboy/.stage-"
        ));
        assert!(calls[2].contains("run:verify_install:sh|-s|--|0.9.0|.stage-"));
        assert_eq!(calls[3], "run:status:sh|-s|--|0.9.0|9990");
    }

    #[test]
    fn upload_and_verification_failures_roll_back_only_staging() {
        for step in ["upload", "verify_install"] {
            let (_dir, bundle, operation, sink) = fixture();
            let remote = FakeRemote::failing(step);
            assert!(run_with(&remote, &operation, &bundle, 9990, sink).is_err());
            let calls = remote.calls();
            assert!(calls
                .last()
                .unwrap()
                .starts_with("run:cleanup:sh|-s|--|.stage-"));
            assert!(calls.iter().all(|call| !call.contains("rm-current")));
        }
    }

    #[test]
    fn update_stages_release_before_activation() {
        let (_dir, bundle, operation, sink) = fixture();
        let remote = FakeRemote::default();

        stage_with(&remote, &operation, &bundle, sink).unwrap();

        let calls = remote.calls();
        assert!(calls[0].starts_with("run:stage:"));
        assert!(calls[1].starts_with("upload:"));
        assert!(calls[2].contains("run:stage_release:sh|-s|--|0.9.0|.stage-"));
        assert!(calls[2].ends_with("|stage"));
        assert!(calls.iter().all(|call| !call.contains("activate")));
    }

    #[test]
    fn update_restart_failure_reactivates_and_restarts_previous_version() {
        let (_dir, _bundle, operation, sink) = fixture();
        let remote = FakeRemote {
            fail_restart_version: Mutex::new(Some("new".into())),
            ..FakeRemote::default()
        };

        let error =
            activate_restart_with(&remote, &operation, "new", "old", 9990, sink).unwrap_err();

        assert_eq!(error.code, "restart_failed");
        let calls = remote.calls();
        assert!(calls[0].contains("run:activate:sh|-s|--|new|.stage-"));
        assert!(calls[1].contains("run:restart:sh|-s|--|new|9990"));
        assert!(calls[2].contains("run:activate:sh|-s|--|old|.stage-"));
        assert!(calls[3].contains("run:restart:sh|-s|--|old|9990"));
    }

    #[test]
    fn update_uses_the_staged_symlink_version_not_the_running_daemon_version() {
        let (_dir, _bundle, operation, sink) = fixture();
        let remote = FakeRemote {
            activation_previous: Mutex::new(Some("new".into())),
            ..FakeRemote::default()
        };

        let result = activate_restart_with(&remote, &operation, "new", "new", 9990, sink).unwrap();

        assert_eq!(result.version, "new");
        assert_eq!(remote.calls().len(), 2);
    }

    #[test]
    fn malformed_post_switch_activation_output_rolls_back_known_previous_version() {
        let (_dir, _bundle, operation, sink) = fixture();
        let remote = FakeRemote {
            invalid_activate_version: Mutex::new(Some("new".into())),
            ..FakeRemote::default()
        };

        let error =
            activate_restart_with(&remote, &operation, "new", "old", 9990, sink).unwrap_err();

        assert_eq!(error.code, "remote_activate_invalid");
        let calls = remote.calls();
        assert!(calls[0].contains("run:activate:sh|-s|--|new|.stage-"));
        assert!(calls[1].contains("run:activate:sh|-s|--|old|.stage-"));
        assert!(calls[2].contains("run:restart:sh|-s|--|old|9990"));
    }

    #[test]
    fn rollback_rejects_daemon_reporting_the_wrong_previous_version() {
        let (_dir, _bundle, operation, sink) = fixture();
        let remote = FakeRemote {
            fail_restart_version: Mutex::new(Some("new".into())),
            wrong_restart_version: Mutex::new(Some("unexpected".into())),
            ..FakeRemote::default()
        };

        let error =
            activate_restart_with(&remote, &operation, "new", "old", 9990, sink).unwrap_err();

        assert_eq!(error.code, "restart_failed");
        assert!(error.message.contains("rollback to old also failed"));
        assert!(error.message.contains("reported unexpected"));
    }

    #[test]
    fn status_requires_exactly_one_valid_json_document() {
        assert!(parse_result("", "x").is_err());
        assert!(parse_result("{}", "x").is_err());
        assert!(parse_result(r#"{"version":"x","http_addr":""} {}"#, "x").is_err());
        let mismatch =
            parse_result(r#"{"version":"old","http_addr":"127.0.0.1:9990"}"#, "new").unwrap();
        assert_eq!(mismatch.state, "degraded");
        assert_eq!(mismatch.message, "version_mismatch");
    }

    #[test]
    fn rejects_remote_shell_metacharacters_in_versions_before_running_ssh() {
        let (_dir, bundle, operation, sink) = fixture();
        let remote = FakeRemote::default();
        for version in ["bad;touch /tmp/pwn", "$(id)", "-oProxyCommand=x", ".hidden"] {
            assert!(
                activate_restart_with(&remote, &operation, version, "old", 9990, sink.clone())
                    .is_err()
            );
        }
        assert!(remote.calls().is_empty());

        let mut unsafe_bundle = bundle;
        unsafe_bundle.version = "1.0;id".into();
        assert!(stage_with(&remote, &operation, &unsafe_bundle, sink).is_err());
        assert!(remote.calls().is_empty());
    }

    fn write_release(dir: &Path, version: &str, valid_checksums: bool) {
        fs::create_dir_all(dir).unwrap();
        for name in BINARIES {
            fs::write(dir.join(name), format!("{name}\n")).unwrap();
        }
        fs::write(dir.join("VERSION"), format!("{version}\n")).unwrap();
        let mut sums = String::new();
        for name in BINARIES {
            let output = Command::new("sha256sum")
                .arg(dir.join(name))
                .output()
                .unwrap();
            let hash = String::from_utf8(output.stdout)
                .unwrap()
                .split_whitespace()
                .next()
                .unwrap()
                .to_string();
            if valid_checksums {
                sums.push_str(&format!("{hash}  {name}\n"));
            } else {
                sums.push_str(&format!("00{hash}  {name}\n"));
            }
        }
        fs::write(dir.join("SHA256SUMS"), sums).unwrap();
    }

    #[test]
    fn installer_switches_links_only_after_valid_checksums() {
        let home = tempfile::tempdir().unwrap();
        let root = home.path().join(".local/lib/tariboy");
        let bin = home.path().join(".local/bin");
        fs::create_dir_all(&bin).unwrap();
        let old = root.join("old");
        write_release(&old, "old", true);
        for name in BINARIES {
            std::os::unix::fs::symlink(old.join(name), bin.join(name)).unwrap();
        }

        let bad = root.join(".stage-bad");
        write_release(&bad, "new", false);
        let failure = Command::new("sh")
            .arg(concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/src/remote-install.sh"
            ))
            .args(["new", ".stage-bad"])
            .env("HOME", home.path())
            .status()
            .unwrap();
        assert!(!failure.success());
        for name in BINARIES {
            assert_eq!(fs::read_link(bin.join(name)).unwrap(), old.join(name));
        }
        assert!(!root.join("new").exists());

        let good = root.join(".stage-good");
        write_release(&good, "new", true);
        let success = Command::new("sh")
            .arg(concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/src/remote-install.sh"
            ))
            .args(["new", ".stage-good"])
            .env("HOME", home.path())
            .status()
            .unwrap();
        assert!(success.success());
        for name in BINARIES {
            assert_eq!(
                fs::read_link(bin.join(name)).unwrap(),
                root.join("new").join(name)
            );
        }
    }

    #[test]
    fn installer_switches_optional_store_binary_with_the_release() {
        let home = tempfile::tempdir().unwrap();
        let stage = home
            .path()
            .join(".local/lib/tariboy/.stage-with-store");
        write_release(&stage, "new", true);
        fs::write(stage.join("tariboy-store"), b"tariboy-store\n").unwrap();
        let output = Command::new("sha256sum")
            .arg(stage.join("tariboy-store"))
            .output()
            .unwrap();
        let sums = stage.join("SHA256SUMS");
        let mut contents = fs::read(&sums).unwrap();
        contents.extend_from_slice(&output.stdout);
        fs::write(sums, contents).unwrap();

        let status = Command::new("sh")
            .arg(concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/src/remote-install.sh"
            ))
            .args(["new", ".stage-with-store"])
            .env("HOME", home.path())
            .status()
            .unwrap();

        assert!(status.success());
        assert_eq!(
            fs::read_link(home.path().join(".local/bin/tariboy-store")).unwrap(),
            home.path()
                .join(".local/lib/tariboy/new/tariboy-store")
        );
    }

    #[test]
    fn installer_removes_only_managed_legacy_tools_link() {
        let home = tempfile::tempdir().unwrap();
        let root = home.path().join(".local/lib/tariboy");
        let bin = home.path().join(".local/bin");
        let old = root.join("old");
        fs::create_dir_all(&bin).unwrap();
        write_release(&old, "old", true);
        fs::write(old.join("tariboy-tools"), b"old tools\n").unwrap();
        std::os::unix::fs::symlink(old.join("tariboy-tools"), bin.join("tariboy-tools")).unwrap();
        write_release(&root.join(".stage-new"), "new", true);

        let status = Command::new("sh")
            .arg(concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/src/remote-install.sh"
            ))
            .args(["new", ".stage-new"])
            .env("HOME", home.path())
            .status()
            .unwrap();

        assert!(status.success());
        assert!(!fs::symlink_metadata(bin.join("tariboy-tools")).is_ok());

        let foreign = home.path().join("foreign-tools");
        fs::write(&foreign, b"foreign\n").unwrap();
        std::os::unix::fs::symlink(&foreign, bin.join("tariboy-tools")).unwrap();
        write_release(&root.join(".stage-next"), "next", true);
        let status = Command::new("sh")
            .arg(concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/src/remote-install.sh"
            ))
            .args(["next", ".stage-next"])
            .env("HOME", home.path())
            .status()
            .unwrap();
        assert!(status.success());
        assert_eq!(fs::read_link(bin.join("tariboy-tools")).unwrap(), foreign);
    }

    #[test]
    fn installer_refuses_foreign_symlinks_without_replacing_any_link() {
        let home = tempfile::tempdir().unwrap();
        let root = home.path().join(".local/lib/tariboy");
        let bin = home.path().join(".local/bin");
        let foreign = home.path().join("foreign-package/bin");
        fs::create_dir_all(&bin).unwrap();
        fs::create_dir_all(&foreign).unwrap();
        let mut original = Vec::new();
        for name in BINARIES {
            let target = foreign.join(name);
            fs::write(&target, b"foreign\n").unwrap();
            std::os::unix::fs::symlink(&target, bin.join(name)).unwrap();
            original.push(target);
        }
        write_release(&root.join(".stage-new"), "new", true);

        let output = Command::new("sh")
            .arg(concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/src/remote-install.sh"
            ))
            .args(["new", ".stage-new"])
            .env("HOME", home.path())
            .output()
            .unwrap();

        assert!(!output.status.success());
        assert!(
            String::from_utf8_lossy(&output.stderr).contains("foreign symlink"),
            "{}",
            String::from_utf8_lossy(&output.stderr)
        );
        for (name, target) in BINARIES.iter().zip(original) {
            assert_eq!(fs::read_link(bin.join(name)).unwrap(), target);
        }
        assert!(!root.join("new").exists());
    }

    #[test]
    fn stage_mode_publishes_release_without_switching_links_then_activate_switches() {
        let home = tempfile::tempdir().unwrap();
        let root = home.path().join(".local/lib/tariboy");
        let bin = home.path().join(".local/bin");
        let old = root.join("old");
        write_release(&old, "old", true);
        fs::create_dir_all(&bin).unwrap();
        for name in BINARIES {
            std::os::unix::fs::symlink(old.join(name), bin.join(name)).unwrap();
        }
        write_release(&root.join(".stage-update"), "update", true);
        let script = concat!(env!("CARGO_MANIFEST_DIR"), "/src/remote-install.sh");

        let staged = Command::new("sh")
            .arg(script)
            .args(["update", ".stage-update", "stage"])
            .env("HOME", home.path())
            .output()
            .unwrap();
        assert!(staged.status.success());
        let staged_release: StagedRelease = serde_json::from_slice(&staged.stdout).unwrap();
        assert_eq!(staged_release.version, "update");
        assert_eq!(staged_release.previous, "old");
        assert!(root.join("update").is_dir());
        assert_eq!(
            fs::read_link(bin.join("tariboy")).unwrap(),
            old.join("tariboy")
        );

        let activated = Command::new("sh")
            .arg(script)
            .args(["update", ".stage-activate", "activate", "old"])
            .env("HOME", home.path())
            .output()
            .unwrap();
        assert!(activated.status.success());
        let activation: Activation = serde_json::from_slice(&activated.stdout).unwrap();
        assert_eq!(activation.version, "update");
        assert_eq!(activation.previous, "old");
        for name in BINARIES {
            assert_eq!(
                fs::read_link(home.path().join(".local/bin").join(name)).unwrap(),
                root.join("update").join(name)
            );
        }
    }

    #[test]
    fn activate_compare_and_swap_does_not_clobber_a_concurrent_release() {
        let home = tempfile::tempdir().unwrap();
        let root = home.path().join(".local/lib/tariboy");
        let bin = home.path().join(".local/bin");
        fs::create_dir_all(&bin).unwrap();
        for version in ["old", "update", "concurrent"] {
            write_release(&root.join(version), version, true);
        }
        for name in BINARIES {
            std::os::unix::fs::symlink(root.join("old").join(name), bin.join(name)).unwrap();
        }
        for name in BINARIES {
            fs::remove_file(bin.join(name)).unwrap();
            std::os::unix::fs::symlink(root.join("concurrent").join(name), bin.join(name)).unwrap();
        }

        let output = Command::new("sh")
            .arg(concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/src/remote-install.sh"
            ))
            .args(["update", ".stage-cas", "activate", "old"])
            .env("HOME", home.path())
            .output()
            .unwrap();

        assert!(!output.status.success());
        assert!(String::from_utf8_lossy(&output.stderr).contains("release conflict"));
        for name in BINARIES {
            assert_eq!(
                fs::read_link(bin.join(name)).unwrap(),
                root.join("concurrent").join(name)
            );
        }
    }

    #[test]
    fn installer_rejects_path_like_staging_names() {
        let home = tempfile::tempdir().unwrap();
        for staging in [".stage-a/../escape", "../stage", ".stage-", ".stage-a_b"] {
            let status = Command::new("sh")
                .arg(concat!(
                    env!("CARGO_MANIFEST_DIR"),
                    "/src/remote-install.sh"
                ))
                .args(["1.0.0", staging])
                .env("HOME", home.path())
                .status()
                .unwrap();
            assert!(!status.success(), "accepted {staging}");
        }
    }

    #[test]
    fn installer_rolls_back_every_link_when_a_mid_switch_move_fails() {
        let home = tempfile::tempdir().unwrap();
        let root = home.path().join(".local/lib/tariboy");
        let bin = home.path().join(".local/bin");
        fs::create_dir_all(&bin).unwrap();
        let old = root.join("old");
        write_release(&old, "old", true);
        for name in BINARIES {
            std::os::unix::fs::symlink(old.join(name), bin.join(name)).unwrap();
        }
        write_release(&root.join(".stage-new"), "new", true);

        let tools = tempfile::tempdir().unwrap();
        let counter = tools.path().join("mv-count");
        crate::testbin::executable(
            tools.path(),
            "mv",
            r#"count=0
if test -f "$MV_TEST_COUNTER"; then count=$(cat "$MV_TEST_COUNTER"); fi
count=$((count + 1))
printf '%s' "$count" > "$MV_TEST_COUNTER"
if test "$count" -eq 4; then exit 70; fi
exec /bin/mv "$@""#,
        );
        let path = format!(
            "{}:{}",
            tools.path().display(),
            std::env::var("PATH").unwrap_or_default()
        );
        let status = Command::new("sh")
            .arg(concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/src/remote-install.sh"
            ))
            .args(["new", ".stage-new"])
            .env("HOME", home.path())
            .env("PATH", path)
            .env("MV_TEST_COUNTER", &counter)
            .status()
            .unwrap();

        assert!(!status.success());
        for name in BINARIES {
            assert_eq!(fs::read_link(bin.join(name)).unwrap(), old.join(name));
        }
        assert!(!root.join("new").exists());
    }

    #[test]
    fn concurrent_installers_publish_one_complete_link_set() {
        let home = tempfile::tempdir().unwrap();
        let root = home.path().join(".local/lib/tariboy");
        write_release(&root.join(".stage-one"), "one", true);
        write_release(&root.join(".stage-two"), "two", true);

        let tools = tempfile::tempdir().unwrap();
        crate::testbin::executable(
            tools.path(),
            "mv",
            r#"sleep 0.02
exec /bin/mv "$@""#,
        );
        let path = format!(
            "{}:{}",
            tools.path().display(),
            std::env::var("PATH").unwrap_or_default()
        );
        let script = concat!(env!("CARGO_MANIFEST_DIR"), "/src/remote-install.sh");
        let mut first = Command::new("sh")
            .arg(script)
            .args(["one", ".stage-one"])
            .env("HOME", home.path())
            .env("PATH", &path)
            .stdout(std::process::Stdio::null())
            .spawn()
            .unwrap();
        let mut second = Command::new("sh")
            .arg(script)
            .args(["two", ".stage-two"])
            .env("HOME", home.path())
            .env("PATH", &path)
            .stdout(std::process::Stdio::null())
            .spawn()
            .unwrap();
        assert!(first.wait().unwrap().success());
        assert!(second.wait().unwrap().success());

        let bin = home.path().join(".local/bin");
        let targets = BINARIES
            .iter()
            .map(|name| fs::read_link(bin.join(name)).unwrap())
            .collect::<Vec<_>>();
        let release = targets[0].parent().unwrap().to_path_buf();
        assert!(
            release == root.join("one") || release == root.join("two"),
            "unexpected release {}",
            release.display()
        );
        for (name, target) in BINARIES.iter().zip(targets) {
            assert_eq!(target, release.join(name));
        }
        assert!(root.join(".install.lock").is_file());
    }

    #[test]
    fn killed_lock_owner_does_not_block_the_next_install() {
        let home = tempfile::tempdir().unwrap();
        let root = home.path().join(".local/lib/tariboy");
        fs::create_dir_all(&root).unwrap();
        write_release(&root.join(".stage-next"), "next", true);
        let lock = root.join(".install.lock");
        let ready = home.path().join("lock-ready");
        let mut owner = Command::new("sh")
            .args([
                "-c",
                "exec 9>\"$1\"; flock 9; : >\"$2\"; exec sleep 30",
                "lock-owner",
            ])
            .arg(&lock)
            .arg(&ready)
            .spawn()
            .unwrap();
        let deadline = std::time::Instant::now() + Duration::from_secs(2);
        while !ready.exists() && std::time::Instant::now() < deadline {
            std::thread::sleep(Duration::from_millis(10));
        }
        assert!(ready.exists(), "lock owner never became ready");
        owner.kill().unwrap();
        owner.wait().unwrap();

        let status = Command::new("sh")
            .arg(concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/src/remote-install.sh"
            ))
            .args(["next", ".stage-next"])
            .env("HOME", home.path())
            .status()
            .unwrap();
        assert!(status.success());
        for name in BINARIES {
            assert_eq!(
                fs::read_link(home.path().join(".local/bin").join(name)).unwrap(),
                root.join("next").join(name)
            );
        }
    }
}
