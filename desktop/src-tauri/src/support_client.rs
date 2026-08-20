use crate::{
    hosts::{HostKind, HostState, Registry, RuntimeHosts},
    keychain::TokenStore,
    state::{DaemonView, Phase},
    support::{self, SelectedHost, SelectedTransport},
};
use std::io::Read;
use std::time::Duration;

pub(crate) const LOCAL_HOST_ID: &str = "local";
const MAX_RESPONSE_BYTES: u64 = 160 * 1024 * 1024;

#[derive(Clone)]
pub(crate) struct Target {
    pub id: String,
    pub label: String,
    pub base_url: String,
    pub token: String,
    pub diagnostic: SelectedHost,
}

#[derive(Debug)]
pub(crate) enum FetchOutcome {
    Archive(Vec<u8>),
    Partial(&'static str),
}

pub(crate) fn resolve_target(
    host_id: &str,
    daemon: &DaemonView,
    registry: &Registry,
    runtime: &RuntimeHosts,
    tokens: &dyn TokenStore,
) -> Result<Target, &'static str> {
    if host_id == LOCAL_HOST_ID {
        return Ok(Target {
            id: LOCAL_HOST_ID.into(),
            label: "Local".into(),
            base_url: daemon.base_url.clone(),
            token: String::new(),
            diagnostic: SelectedHost {
                id: LOCAL_HOST_ID.into(),
                label: "Local".into(),
                transport: SelectedTransport::Local,
                state: daemon_state(daemon.state).into(),
                phase: String::new(),
                platform: std::env::consts::OS.into(),
                arch: std::env::consts::ARCH.into(),
                prerequisites: Vec::new(),
                daemon_version: daemon.daemon_version.clone(),
                error_code: support::diagnostic_error_code(&daemon.message),
            },
        });
    }
    let record = registry.get(host_id).map_err(|_| "unknown_host")?;
    let view = runtime.view(record.clone());
    let (transport, base_url, token) = match record.kind {
        HostKind::Ssh => (
            SelectedTransport::Ssh,
            if view.state == HostState::Ready {
                view.base_url.clone()
            } else {
                String::new()
            },
            String::new(),
        ),
        HostKind::Https => (
            SelectedTransport::Https,
            record.https_base_url.clone(),
            tokens
                .get(host_id)
                .map_err(|_| "authentication_failed")?
                .unwrap_or_default(),
        ),
    };
    Ok(Target {
        id: record.id,
        label: record.label,
        base_url,
        token,
        diagnostic: SelectedHost {
            id: view.id,
            label: view.label,
            transport,
            state: support::diagnostic_host_state(&view.state).into(),
            phase: view.phase,
            platform: view.platform,
            arch: view.arch,
            prerequisites: view.prerequisites,
            daemon_version: view.last_daemon_version,
            error_code: support::diagnostic_error_code(&view.message),
        },
    })
}

fn daemon_state(state: Phase) -> &'static str {
    match state {
        Phase::Starting => "starting",
        Phase::Ready => "ready",
        Phase::Failed => "failed",
        Phase::Down => "down",
    }
}

pub(crate) fn fetch(target: &Target, include_agent_data: bool) -> FetchOutcome {
    if target.base_url.is_empty() {
        return FetchOutcome::Partial("host_unreachable");
    }
    let url = format!(
        "{}/api/daemon/support-bundle?include_agent_data={}&iteration_limit=10",
        target.base_url.trim_end_matches('/'),
        usize::from(include_agent_data)
    );
    let agent = ureq::AgentBuilder::new()
        .timeout_connect(Duration::from_secs(5))
        .timeout_read(Duration::from_secs(30))
        .build();
    let mut request = agent.get(&url);
    if !target.token.is_empty() {
        request = request.set("Authorization", &format!("Bearer {}", target.token));
    }
    let response = match request.call() {
        Ok(response) => response,
        Err(ureq::Error::Status(404, _)) => return FetchOutcome::Partial("unsupported_daemon"),
        Err(ureq::Error::Status(401 | 403, _)) => {
            return FetchOutcome::Partial("authentication_failed")
        }
        Err(ureq::Error::Status(413, _)) => return FetchOutcome::Partial("response_too_large"),
        Err(_) => return FetchOutcome::Partial("host_unreachable"),
    };
    if !response
        .header("Content-Type")
        .is_some_and(|value| value.split(';').next() == Some("application/zip"))
    {
        return FetchOutcome::Partial("unsupported_daemon");
    }
    if response
        .header("Content-Length")
        .and_then(|value| value.parse::<u64>().ok())
        .is_some_and(|length| length > MAX_RESPONSE_BYTES)
    {
        return FetchOutcome::Partial("response_too_large");
    }
    let mut body = Vec::new();
    if response
        .into_reader()
        .take(MAX_RESPONSE_BYTES + 1)
        .read_to_end(&mut body)
        .is_err()
    {
        return FetchOutcome::Partial("host_unreachable");
    }
    if body.len() as u64 > MAX_RESPONSE_BYTES {
        return FetchOutcome::Partial("response_too_large");
    }
    FetchOutcome::Archive(body)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::hosts::{Registry, RuntimeHosts, SaveHttpsInput, SaveSshInput};
    use crate::keychain::{MemoryKeychain, TokenStore};
    use crate::state::{DaemonView, Phase};
    use crate::tunnel;
    use std::io::{Read, Write};
    use std::net::TcpListener;
    use std::sync::Arc;
    use std::thread;

    fn local_view(base_url: &str) -> DaemonView {
        DaemonView {
            state: Phase::Ready,
            base_url: base_url.into(),
            daemon_version: "0.11.0".into(),
            app_version: "0.11.0".into(),
            base_dir: "/private/base".into(),
            pid: 42,
            adopted: true,
            message: String::new(),
        }
    }

    #[test]
    fn resolve_target_supports_local_ssh_and_https_without_exposing_transport_secrets() {
        let dir = tempfile::tempdir().unwrap();
        let registry = Registry::new(dir.path().join("hosts.json"));
        let runtime = RuntimeHosts::default();
        let tokens = MemoryKeychain::default();

        let local = resolve_target(
            LOCAL_HOST_ID,
            &local_view("http://127.0.0.1:9991"),
            &registry,
            &runtime,
            &tokens,
        )
        .unwrap();
        assert_eq!(local.id, "local");
        assert_eq!(local.base_url, "http://127.0.0.1:9991");
        assert!(local.token.is_empty());
        assert_eq!(
            local.diagnostic.transport,
            crate::support::SelectedTransport::Local
        );

        let ssh = registry
            .save_ssh(SaveSshInput {
                id: None,
                label: "Build box".into(),
                ssh_alias: "private-alias".into(),
                remote_install_dir: String::new(),
                remote_port: 9990,
            })
            .unwrap();
        runtime.set_tunnel(&tunnel::Event {
            host_id: ssh.id.clone(),
            state: tunnel::State::Ready,
            local_port: 18444,
            status: None,
            message: String::new(),
        });
        let ssh_target = resolve_target(
            &ssh.id,
            &local_view("http://127.0.0.1:9991"),
            &registry,
            &runtime,
            &tokens,
        )
        .unwrap();
        assert_eq!(ssh_target.base_url, "http://127.0.0.1:18444");
        assert!(ssh_target.token.is_empty());
        assert!(!format!("{:?}", ssh_target.diagnostic).contains("private-alias"));

        let https = registry
            .save_https(SaveHttpsInput {
                id: None,
                label: "Prod API".into(),
                https_base_url: "https://prod.internal/".into(),
            })
            .unwrap();
        tokens.set(&https.id, "keychain-secret").unwrap();
        let https_target = resolve_target(
            &https.id,
            &local_view("http://127.0.0.1:9991"),
            &registry,
            &runtime,
            &tokens,
        )
        .unwrap();
        assert_eq!(https_target.base_url, "https://prod.internal");
        assert_eq!(https_target.token, "keychain-secret");
        assert!(!format!("{:?}", https_target.diagnostic).contains("keychain-secret"));
    }

    #[test]
    fn resolve_target_rejects_only_unknown_hosts_and_keeps_unavailable_hosts_exportable() {
        let dir = tempfile::tempdir().unwrap();
        let registry = Registry::new(dir.path().join("hosts.json"));
        let runtime = RuntimeHosts::default();
        let tokens = MemoryKeychain::default();

        assert_eq!(
            resolve_target("missing", &local_view(""), &registry, &runtime, &tokens).err(),
            Some("unknown_host")
        );
        let local = resolve_target("local", &local_view(""), &registry, &runtime, &tokens).unwrap();
        assert!(local.base_url.is_empty());
        let ssh = registry
            .save_ssh(SaveSshInput {
                id: None,
                label: "Build box".into(),
                ssh_alias: "secret-alias".into(),
                remote_install_dir: String::new(),
                remote_port: 9990,
            })
            .unwrap();
        let disconnected = resolve_target(
            &ssh.id,
            &local_view("http://127.0.0.1:9991"),
            &registry,
            &runtime,
            &tokens,
        )
        .unwrap();
        assert!(disconnected.base_url.is_empty());
        assert!(matches!(
            fetch(&disconnected, false),
            FetchOutcome::Partial("host_unreachable")
        ));
    }

    fn serve_once(
        status: &str,
        headers: &[(&str, &str)],
        body: &[u8],
    ) -> (
        String,
        Arc<std::sync::Mutex<String>>,
        thread::JoinHandle<()>,
    ) {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let address = listener.local_addr().unwrap();
        let captured = Arc::new(std::sync::Mutex::new(String::new()));
        let sink = captured.clone();
        let status = status.to_string();
        let headers = headers
            .iter()
            .map(|(key, value)| (key.to_string(), value.to_string()))
            .collect::<Vec<_>>();
        let body = body.to_vec();
        let thread = thread::spawn(move || {
            let (mut stream, _) = listener.accept().unwrap();
            let mut request = [0u8; 8192];
            let read = stream.read(&mut request).unwrap();
            *sink.lock().unwrap() = String::from_utf8_lossy(&request[..read]).to_string();
            write!(stream, "HTTP/1.1 {status}\r\n").unwrap();
            let has_content_length = headers
                .iter()
                .any(|(key, _)| key.eq_ignore_ascii_case("Content-Length"));
            for (key, value) in headers {
                write!(stream, "{key}: {value}\r\n").unwrap();
            }
            if !has_content_length {
                write!(stream, "Content-Length: {}\r\n", body.len()).unwrap();
            }
            write!(stream, "Connection: close\r\n\r\n").unwrap();
            stream.write_all(&body).unwrap();
        });
        (format!("http://{address}"), captured, thread)
    }

    fn target(base_url: String, token: &str) -> Target {
        Target {
            id: "host-1".into(),
            label: "Build box".into(),
            base_url,
            token: token.into(),
            diagnostic: crate::support::SelectedHost {
                id: "host-1".into(),
                label: "Build box".into(),
                transport: crate::support::SelectedTransport::Https,
                state: "ready".into(),
                phase: String::new(),
                platform: String::new(),
                arch: String::new(),
                prerequisites: Vec::new(),
                daemon_version: "0.11.0".into(),
                error_code: None,
            },
        }
    }

    #[test]
    fn fetch_sends_exact_query_and_bearer_then_returns_archive() {
        let (base_url, captured, server) =
            serve_once("200 OK", &[("Content-Type", "application/zip")], b"zip");
        let outcome = fetch(&target(base_url, "top-secret"), true);
        server.join().unwrap();
        assert!(matches!(outcome, FetchOutcome::Archive(body) if body == b"zip"));
        let request = captured.lock().unwrap().clone();
        assert!(request.starts_with(
            "GET /api/daemon/support-bundle?include_agent_data=1&iteration_limit=10 HTTP/1.1"
        ));
        assert!(request
            .to_ascii_lowercase()
            .contains("authorization: bearer top-secret"));
    }

    #[test]
    fn fetch_classifies_compatibility_auth_and_oversize_without_leaking_response() {
        for (status, expected) in [
            ("404 Not Found", "unsupported_daemon"),
            ("401 Unauthorized", "authentication_failed"),
            ("403 Forbidden", "authentication_failed"),
        ] {
            let (base_url, _, server) = serve_once(
                status,
                &[("Content-Type", "application/json")],
                b"PRIVATE_RESPONSE",
            );
            let outcome = fetch(&target(base_url, "secret-token"), false);
            server.join().unwrap();
            assert!(matches!(&outcome, FetchOutcome::Partial(code) if *code == expected));
            assert!(!format!("{outcome:?}").contains("PRIVATE_RESPONSE"));
            assert!(!format!("{outcome:?}").contains("secret-token"));
        }

        let (base_url, _, server) = serve_once(
            "200 OK",
            &[
                ("Content-Type", "application/zip"),
                ("Content-Length", "167772161"),
            ],
            b"",
        );
        let outcome = fetch(&target(base_url, ""), false);
        server.join().unwrap();
        assert!(matches!(
            outcome,
            FetchOutcome::Partial("response_too_large")
        ));
    }
}
