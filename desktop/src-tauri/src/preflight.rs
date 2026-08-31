use serde::{Deserialize, Serialize};
use std::path::Path;
use std::time::Duration;

pub const SCRIPT: &str = r#"set -eu
json_escape() {
  # Environment values and tool versions are untrusted text. Replace JSON
  # control bytes rather than allowing one odd version banner to corrupt the
  # single JSON document, then escape the two printable JSON metacharacters.
  printf '%s' "$1" | LC_ALL=C tr '\000-\037' '?' | sed 's/\\/\\\\/g; s/"/\\"/g'
}
tool_json() {
  tool="$1"
  if command -v "$tool" >/dev/null 2>&1; then
    if [ "$tool" = tmux ]; then
      version=$(tmux -V 2>&1 | sed -n '1p' || true)
    else
      version=$("$tool" --version 2>&1 | sed -n '1p' || true)
    fi
    printf '{"available":true,"version":"%s"}' "$(json_escape "$version")"
  else
    printf '{"available":false,"version":""}'
  fi
}
platform=$(uname -s)
arch=$(uname -m)
home=${HOME:-}
free_disk_kb=0
if [ -n "$home" ]; then
  free_disk_kb=$(df -Pk "$home" 2>/dev/null | awk 'NR==2 {print $4}')
fi
case "$free_disk_kb" in ''|*[!0-9]*) free_disk_kb=0 ;; esac
if [ -n "$home" ] && mkdir -p "$home/.local" 2>/dev/null && [ -w "$home/.local" ]; then
  writable_local=true
else
  writable_local=false
fi
printf '{"platform":"%s","arch":"%s","home":"%s","free_disk_kb":%s,"writable_local":%s,' \
  "$(json_escape "$platform")" "$(json_escape "$arch")" "$(json_escape "$home")" \
  "$free_disk_kb" "$writable_local"
printf '"tmux":'; tool_json tmux
printf ',"flock":'; tool_json flock
printf ',"python3":'; tool_json python3
printf ',"claude":'; tool_json claude
printf ',"codex":'; tool_json codex
printf ',"opencode":'; tool_json opencode
printf '}\n'
"#;

#[derive(Debug, Clone, Deserialize, PartialEq, Eq, Serialize)]
pub struct Tool {
    pub available: bool,
    pub version: String,
}

#[derive(Debug, Clone, Deserialize, PartialEq, Eq, Serialize)]
pub struct Result {
    pub platform: String,
    pub arch: String,
    pub home: String,
    pub free_disk_kb: u64,
    pub writable_local: bool,
    pub tmux: Tool,
    pub flock: Tool,
    pub python3: Tool,
    pub claude: Tool,
    pub codex: Tool,
    pub opencode: Tool,
    #[serde(default)]
    pub prerequisites: Vec<String>,
    #[serde(default)]
    pub install_supported: bool,
}

pub fn parse(stdout: &str) -> std::result::Result<Result, String> {
    let mut result: Result =
        serde_json::from_str(stdout).map_err(|error| format!("invalid preflight JSON: {error}"))?;
    result.install_supported = result.platform == "Linux"
        && result.arch == "x86_64"
        && result.writable_local
        && result.flock.available
        && result.python3.available;
    let mut prerequisites = Vec::new();
    if result.platform != "Linux" {
        prerequisites.push("Linux".to_string());
    }
    if result.arch != "x86_64" {
        prerequisites.push("x86_64".to_string());
    }
    if !result.writable_local {
        prerequisites.push("writable ~/.local".to_string());
    }
    if !result.flock.available {
        prerequisites.push("flock".to_string());
    }
    if !result.python3.available {
        prerequisites.push("python3".to_string());
    }
    for (name, tool) in [
        ("tmux", &result.tmux),
        ("claude", &result.claude),
        ("codex", &result.codex),
        ("opencode", &result.opencode),
    ] {
        if !tool.available {
            prerequisites.push(name.to_string());
        }
    }
    result.prerequisites = prerequisites;
    Ok(result)
}

pub fn run(
    transport: &crate::ssh::Transport,
    operation: &crate::ssh::Operation,
    alias: &str,
    control_socket: &Path,
    timeout: Duration,
    sink: crate::ssh::OutputSink,
) -> std::result::Result<Result, crate::ssh::SshError> {
    let stdout = transport.run_control(
        operation,
        crate::ssh::ControlCommand {
            alias,
            socket: control_socket,
            remote_args: &["sh", "-s"],
            stdin: SCRIPT.as_bytes(),
            timeout,
        },
        sink,
    )?;
    parse(&stdout).map_err(|message| crate::ssh::SshError {
        code: "preflight_failed".into(),
        message,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{ssh, testbin};
    use std::sync::{Arc, Mutex};

    fn fixture(platform: &str, arch: &str, tmux: bool) -> String {
        format!(
            r#"{{
              "platform":"{platform}","arch":"{arch}","home":"/home/u",
              "free_disk_kb":2048,"writable_local":true,
              "tmux":{{"available":{tmux},"version":"tmux 3.4"}},
              "flock":{{"available":true,"version":"flock 2.39"}},
              "python3":{{"available":true,"version":"Python 3.12.3"}},
              "claude":{{"available":true,"version":"2.0"}},
              "codex":{{"available":false,"version":""}},
              "opencode":{{"available":false,"version":""}}
            }}"#
        )
    }

    #[test]
    fn linux_x86_64_can_install_and_missing_tools_are_named() {
        let result = parse(&fixture("Linux", "x86_64", false)).unwrap();
        assert!(result.install_supported);
        assert_eq!(result.prerequisites, vec!["tmux", "codex", "opencode"]);
    }

    #[test]
    fn unsupported_platform_and_arch_block_install() {
        let result = parse(&fixture("Darwin", "arm64", true)).unwrap();
        assert!(!result.install_supported);
        assert!(result.prerequisites.contains(&"Linux".to_string()));
        assert!(result.prerequisites.contains(&"x86_64".to_string()));
    }

    #[test]
    fn missing_flock_blocks_transactional_install() {
        let json = fixture("Linux", "x86_64", true).replace(
            r#""flock":{"available":true"#,
            r#""flock":{"available":false"#,
        );
        let result = parse(&json).unwrap();
        assert!(!result.install_supported);
        assert!(result.prerequisites.contains(&"flock".to_string()));
    }

    #[test]
    fn missing_python_blocks_install_and_is_reported() {
        let json = fixture("Linux", "x86_64", true).replace(
            r#""python3":{"available":true"#,
            r#""python3":{"available":false"#,
        );
        let result = parse(&json).unwrap();
        assert!(!result.install_supported);
        assert!(result.prerequisites.contains(&"python3".to_string()));
    }

    #[test]
    fn exactly_one_json_document_is_required() {
        assert!(parse(&(fixture("Linux", "x86_64", true) + "\n{}")).is_err());
        assert!(parse("not json").is_err());
    }

    #[test]
    fn run_uses_control_socket_fixed_sh_stdin_and_streams_output() {
        let dir = tempfile::tempdir().unwrap();
        let log = dir.path().join("argv.log");
        let script_copy = dir.path().join("script");
        let json = fixture("Linux", "x86_64", true);
        let behavior = format!(
            "cat > '{}'\nprintf '%s\\n' '{}'",
            script_copy.display(),
            json.replace('\'', "'\\''")
        );
        let ssh_path =
            testbin::executable(dir.path(), "ssh", &testbin::argv_logger(&log, &behavior));
        let transport = ssh::Transport::new(ssh::Binaries {
            ssh: ssh_path,
            scp: dir.path().join("scp"),
        });
        let operations = ssh::Operations::default();
        let operation = operations.begin("host-1").unwrap();
        let events = Arc::new(Mutex::new(Vec::new()));
        let capture = events.clone();
        let sink: ssh::OutputSink = Arc::new(move |event| capture.lock().unwrap().push(event));
        let socket = dir.path().join("host-1.sock");

        let result = run(
            &transport,
            &operation,
            "prod alias",
            &socket,
            Duration::from_secs(1),
            sink,
        )
        .unwrap();

        assert!(result.install_supported);
        assert_eq!(
            testbin::invocations(&log)[0],
            vec!["-S", socket.to_str().unwrap(), "prod alias", "sh", "-s"]
        );
        assert_eq!(std::fs::read_to_string(script_copy).unwrap(), SCRIPT);
        assert!(!events.lock().unwrap().is_empty());
    }

    #[test]
    fn fixed_script_sanitizes_control_bytes_in_tool_versions() {
        let dir = tempfile::tempdir().unwrap();
        testbin::executable(dir.path(), "claude", "printf 'v1\\tbad\\r\\n'");
        let path = format!("{}:/usr/bin:/bin", dir.path().display());

        let output = std::process::Command::new("/bin/sh")
            .arg("-c")
            .arg(SCRIPT)
            .env("HOME", dir.path())
            .env("PATH", path)
            .output()
            .unwrap();

        assert!(output.status.success());
        let parsed = parse(&String::from_utf8(output.stdout).unwrap()).unwrap();
        assert!(parsed.claude.available);
        assert_eq!(parsed.claude.version, "v1?bad?");
    }
}
