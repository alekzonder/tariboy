use crate::daemon;
use std::io::{Read, Write};
use std::net::{Ipv4Addr, SocketAddrV4, TcpStream};
use std::time::Duration;

const MAX_RESPONSE: u64 = 1024 * 1024;

pub trait Probe: Send + Sync {
    fn status(&self, port: u16, timeout: Duration) -> Result<daemon::Status, String>;
}

#[derive(Default)]
pub struct HttpProbe;

impl Probe for HttpProbe {
    fn status(&self, port: u16, timeout: Duration) -> Result<daemon::Status, String> {
        probe(port, timeout)
    }
}

pub fn probe(port: u16, timeout: Duration) -> Result<daemon::Status, String> {
    let value = get_result(port, "/api/daemon/status", timeout)?;
    serde_json::from_value(value).map_err(|error| format!("decode tunnel daemon status: {error}"))
}

pub fn get_result(port: u16, path: &str, timeout: Duration) -> Result<serde_json::Value, String> {
    if !path.starts_with('/') || path.contains('\r') || path.contains('\n') {
        return Err("invalid tunnel health path".into());
    }
    let addr = SocketAddrV4::new(Ipv4Addr::LOCALHOST, port);
    let mut stream = TcpStream::connect_timeout(&addr.into(), timeout)
        .map_err(|error| format!("connect tunnel health endpoint: {error}"))?;
    stream
        .set_read_timeout(Some(timeout))
        .map_err(|error| format!("set tunnel health read timeout: {error}"))?;
    stream
        .set_write_timeout(Some(timeout))
        .map_err(|error| format!("set tunnel health write timeout: {error}"))?;
    let request = format!("GET {path} HTTP/1.0\r\nHost: localhost\r\nConnection: close\r\n\r\n");
    stream
        .write_all(request.as_bytes())
        .map_err(|error| format!("write tunnel health request: {error}"))?;
    let mut raw = Vec::new();
    stream
        .take(MAX_RESPONSE)
        .read_to_end(&mut raw)
        .map_err(|error| format!("read tunnel health response: {error}"))?;
    let text = String::from_utf8_lossy(&raw);
    let (head, body) = text
        .split_once("\r\n\r\n")
        .ok_or_else(|| "tunnel health returned an invalid HTTP response".to_string())?;
    let status = head
        .lines()
        .next()
        .and_then(|line| line.split_whitespace().nth(1))
        .and_then(|value| value.parse::<u16>().ok())
        .unwrap_or(0);
    if status != 200 {
        return Err(format!("tunnel health returned HTTP {status}"));
    }
    let envelope: serde_json::Value = serde_json::from_str(body)
        .map_err(|error| format!("decode tunnel API envelope: {error}"))?;
    if envelope.get("ok").and_then(serde_json::Value::as_bool) != Some(true) {
        return Err("tunnel API returned an error envelope".into());
    }
    envelope
        .get("result")
        .cloned()
        .ok_or_else(|| "tunnel API response has no result".into())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::net::TcpListener;
    use std::thread;

    #[test]
    fn probes_daemon_status_over_http_1_0_loopback() {
        let listener = TcpListener::bind((Ipv4Addr::LOCALHOST, 0)).unwrap();
        let port = listener.local_addr().unwrap().port();
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().unwrap();
            let mut request = [0u8; 4096];
            let n = stream.read(&mut request).unwrap();
            assert!(String::from_utf8_lossy(&request[..n])
                .starts_with("GET /api/daemon/status HTTP/1.0\r\n"));
            let body =
                r#"{"ok":true,"result":{"version":"1.2.3","pid":42,"http_addr":"127.0.0.1:9990"}}"#;
            write!(
                stream,
                "HTTP/1.0 200 OK\r\nContent-Length: {}\r\n\r\n{}",
                body.len(),
                body
            )
            .unwrap();
        });

        let status = probe(port, Duration::from_secs(1)).unwrap();

        assert_eq!(status.version, "1.2.3");
        assert_eq!(status.http_addr, "127.0.0.1:9990");
        server.join().unwrap();
    }

    #[test]
    fn rejects_non_success_or_invalid_status() {
        for response in [
            "HTTP/1.0 503 Service Unavailable\r\n\r\n",
            "HTTP/1.0 200 OK\r\n\r\n{}",
        ] {
            let listener = TcpListener::bind((Ipv4Addr::LOCALHOST, 0)).unwrap();
            let port = listener.local_addr().unwrap().port();
            let server = thread::spawn(move || {
                let (mut stream, _) = listener.accept().unwrap();
                let mut request = [0u8; 4096];
                let _ = stream.read(&mut request);
                stream.write_all(response.as_bytes()).unwrap();
            });
            assert!(probe(port, Duration::from_secs(1)).is_err());
            server.join().unwrap();
        }
    }
}
