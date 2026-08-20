//! Local tariboyd lifecycle, mirroring internal/daemonctl.
//!
//! The daemon is a shared background service: the CLI, the desktop app and any
//! `tariboy` invocation all talk to the SAME process over the same control
//! socket. Adoption — attaching to a daemon we did not start — is therefore the
//! normal case, not an edge case, and `probe` is what makes it safe.

use serde::{Deserialize, Serialize};
use std::io::{Read, Write};
use std::os::unix::net::UnixStream;
use std::path::Path;
use std::time::Duration;

/// The subset of GET /api/daemon/status the app needs.
#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct Status {
    #[serde(default)]
    pub version: String,
    #[serde(default)]
    pub pid: i64,
    #[serde(default)]
    pub base_dir: String,
    /// Empty when the daemon was started with `--http-addr ""` (socket only).
    #[serde(default)]
    pub http_addr: String,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Probe {
    /// Nothing is listening on the control socket.
    Down,
    /// Something answered. `Some` when the ok-envelope parsed, `None` when the
    /// daemon replied with an error or unparseable body — still a live process.
    Up(Option<Status>),
}

/// probe issues GET /api/daemon/status over the unix control socket.
///
/// Mirrors internal/daemonctl.alive: ONLY a failed connect means down. Once the
/// socket accepts the connection, any later failure (timeout, truncated read,
/// error envelope) still proves a process is listening — and starting a second
/// daemon on a shared base dir is the one outcome that must never happen.
pub fn probe(sock: &Path, timeout: Duration) -> Probe {
    let stream = match UnixStream::connect(sock) {
        Ok(s) => s,
        Err(_) => return Probe::Down,
    };
    match request(stream, "/api/daemon/status", timeout) {
        Ok((_, body)) => Probe::Up(parse_status(&body)),
        Err(_) => Probe::Up(None),
    }
}

/// request writes one HTTP/1.0 GET and reads the whole response. HTTP/1.0 plus
/// `Connection: close` makes the server hang up after the body, so a plain
/// read-to-end terminates without a chunk parser or a content-length reader.
fn request(mut s: UnixStream, path: &str, timeout: Duration) -> std::io::Result<(u16, String)> {
    s.set_read_timeout(Some(timeout))?;
    s.set_write_timeout(Some(timeout))?;
    let req = format!("GET {path} HTTP/1.0\r\nHost: localhost\r\nConnection: close\r\n\r\n");
    s.write_all(req.as_bytes())?;
    let mut raw = Vec::new();
    s.read_to_end(&mut raw)?;
    let text = String::from_utf8_lossy(&raw).into_owned();
    let (head, body) = text.split_once("\r\n\r\n").unwrap_or((text.as_str(), ""));
    let code = head
        .lines()
        .next()
        .and_then(|l| l.split_whitespace().nth(1))
        .and_then(|c| c.parse::<u16>().ok())
        .unwrap_or(0);
    Ok((code, body.to_string()))
}

/// parse_status unwraps the daemon's {ok,result} | {ok,error} envelope. The
/// object is located by brace scanning so any transfer framing around it (a
/// chunked size line, a trailer) is skipped without a dedicated parser.
pub fn parse_status(body: &str) -> Option<Status> {
    let start = body.find('{')?;
    let end = body.rfind('}')?;
    if end < start {
        return None;
    }
    let v: serde_json::Value = serde_json::from_str(&body[start..=end]).ok()?;
    if !v.get("ok")?.as_bool()? {
        return None;
    }
    serde_json::from_value(v.get("result")?.clone()).ok()
}

use std::ffi::OsString;
use std::net::TcpListener;
use std::path::PathBuf;
use std::process::{Child, Command, Stdio};

/// Everything the lifecycle needs, assembled once at startup from `paths` and
/// `bundle` so no function reaches into the environment on its own.
#[derive(Debug, Clone)]
pub struct Config {
    pub socket: PathBuf,
    pub pid_file: PathBuf,
    pub log_file: PathBuf,
    pub runtime_dir: PathBuf,
    pub daemon_bin: PathBuf,
    pub ready_timeout: Duration,
    pub poll_interval: Duration,
}

/// The outcome of bringing the daemon up.
#[derive(Debug)]
pub struct Ready {
    pub status: Status,
    /// true when we attached to a daemon someone else started.
    pub adopted: bool,
    /// The process handle when WE started it, so a later exit can be reaped and
    /// reported instead of leaving a zombie and a stale "ready".
    pub child: Option<Child>,
}

/// DEFAULT_PORT is the well-known loopback port the browser UI has always used;
/// keeping it means an existing bookmark and `tariboy` docs still work.
pub const DEFAULT_PORT: u16 = 9990;

/// macOS GUI applications do not inherit the interactive shell's PATH. Keep
/// the operator's configured order, then add the standard package-manager and
/// system locations needed by host-side runtime tools such as tmux.
///
/// `~/.local/bin` leads the fallbacks because it is where this app installs its
/// own CLI and where harness installers put `claude`; an account-local install
/// is more specific than a system one, so it must win. This list is only a
/// safety net — a daemon started through the login shell already carries the
/// account's real PATH.
fn daemon_path(current: Option<OsString>, home: Option<OsString>) -> OsString {
    let mut paths = current
        .as_deref()
        .map(std::env::split_paths)
        .map(Iterator::collect::<Vec<_>>)
        .unwrap_or_default();
    let mut fallbacks: Vec<PathBuf> = Vec::new();
    if let Some(home) = home.as_deref().filter(|home| !home.is_empty()) {
        fallbacks.push(Path::new(home).join(".local").join("bin"));
    }
    fallbacks.extend(
        ["/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin"]
            .into_iter()
            .map(PathBuf::from),
    );
    for fallback in fallbacks {
        if !paths.contains(&fallback) {
            paths.push(fallback);
        }
    }
    std::env::join_paths(paths).unwrap_or_else(|_| current.unwrap_or_default())
}

fn valid_login_shell(path: &Path) -> Option<PathBuf> {
    use std::os::unix::fs::PermissionsExt;

    if !path.is_absolute() {
        return None;
    }
    let metadata = path.metadata().ok()?;
    if !metadata.is_file() || metadata.permissions().mode() & 0o111 == 0 {
        return None;
    }
    Some(path.to_path_buf())
}

/// Converts the shell field copied from an account record into a validated
/// executable path without retaining a pointer into the caller's NSS buffer.
fn account_record_shell(raw: &[u8]) -> Option<PathBuf> {
    use std::os::unix::ffi::OsStrExt;

    valid_login_shell(Path::new(std::ffi::OsStr::from_bytes(raw)))
}

/// Reads the effective account's configured login shell from the OS account
/// database. Account-record strings point into the caller-owned scratch buffer,
/// so the shell bytes are copied before the buffer is released or resized.
fn account_login_shell() -> Option<PathBuf> {
    const FALLBACK_BUFFER_SIZE: usize = 16 * 1024;
    const MAX_BUFFER_SIZE: usize = 1024 * 1024;

    let suggested = unsafe { libc::sysconf(libc::_SC_GETPW_R_SIZE_MAX) };
    let mut buffer_size = if suggested > 0 {
        suggested as usize
    } else {
        FALLBACK_BUFFER_SIZE
    }
    .clamp(1024, MAX_BUFFER_SIZE);

    loop {
        let mut record = std::mem::MaybeUninit::<libc::passwd>::uninit();
        let mut result: *mut libc::passwd = std::ptr::null_mut();
        let mut buffer = vec![0_u8; buffer_size];
        let status = unsafe {
            libc::getpwuid_r(
                libc::geteuid(),
                record.as_mut_ptr(),
                buffer.as_mut_ptr().cast::<libc::c_char>(),
                buffer.len(),
                &mut result,
            )
        };

        if status == libc::ERANGE && buffer_size < MAX_BUFFER_SIZE {
            buffer_size = (buffer_size * 2).min(MAX_BUFFER_SIZE);
            continue;
        }
        if status != 0 || result.is_null() {
            return None;
        }

        let shell = unsafe {
            let shell = (*result).pw_shell;
            if shell.is_null() {
                return None;
            }
            std::ffi::CStr::from_ptr(shell).to_bytes().to_owned()
        };
        return account_record_shell(&shell);
    }
}

fn daemon_shell(account: Option<PathBuf>, inherited: Option<OsString>) -> Option<PathBuf> {
    use std::os::unix::ffi::OsStrExt;

    account.as_deref().and_then(valid_login_shell).or_else(|| {
        inherited
            .as_deref()
            .and_then(|shell| account_record_shell(shell.as_bytes()))
    })
}

/// pick_port prefers DEFAULT_PORT and otherwise asks the OS for a free one.
/// There is a brief window between releasing the probe listener and the daemon
/// binding it; losing that race surfaces as a normal start failure (banner plus
/// log tail), which is the same path as any other bind error.
pub fn pick_port() -> u16 {
    if TcpListener::bind(("127.0.0.1", DEFAULT_PORT)).is_ok() {
        return DEFAULT_PORT;
    }
    TcpListener::bind(("127.0.0.1", 0))
        .and_then(|l| l.local_addr())
        .map(|a| a.port())
        .unwrap_or(0)
}

/// spawn starts tariboyd detached, appending stdout+stderr to the daemon log —
/// the same contract as internal/daemonctl.EnsureUp, so `tariboy daemon logs`
/// keeps working against a daemon this app started.
pub fn spawn(
    bin: &Path,
    http_addr: &str,
    runtime_dir: &Path,
    log_file: &Path,
) -> std::io::Result<Child> {
    spawn_with_environment(
        bin,
        http_addr,
        runtime_dir,
        log_file,
        LaunchEnvironment {
            account_shell: account_login_shell(),
            inherited_shell: std::env::var_os("SHELL"),
            inherited_path: std::env::var_os("PATH"),
            inherited_home: std::env::var_os("HOME"),
        },
    )
}

/// Everything about the launcher's own environment that decides how the daemon
/// is started. Read once in `spawn`, so every other function stays testable
/// without touching the process environment.
#[derive(Debug, Default)]
struct LaunchEnvironment {
    /// The login shell from the OS account record.
    account_shell: Option<PathBuf>,
    /// `SHELL` as the launcher inherited it; a GUI launch usually has none.
    inherited_shell: Option<OsString>,
    inherited_path: Option<OsString>,
    inherited_home: Option<OsString>,
}

fn spawn_with_environment(
    bin: &Path,
    http_addr: &str,
    runtime_dir: &Path,
    log_file: &Path,
    launch: LaunchEnvironment,
) -> std::io::Result<Child> {
    std::fs::create_dir_all(runtime_dir)?;
    let shell = daemon_shell(launch.account_shell, launch.inherited_shell);
    let path = daemon_path(launch.inherited_path, launch.inherited_home);

    // Born inside the account's login shell, the daemon — and therefore every
    // agent and harness it launches — inherits exactly the environment a new
    // terminal gets. `exec` keeps the pid the app tracks pointed at the daemon
    // itself, so setsid, the child handle and the exit watcher stay valid.
    if let Some(shell) = shell.as_deref() {
        let mut cmd = Command::new(shell);
        cmd.arg("-lic").arg(login_shell_command(bin, http_addr));
        // Tells tariboyd its environment already comes from the login shell,
        // so it skips its own redundant PATH probe.
        cmd.env("TARIBOY_SHELL_ENV", "1");
        configure_daemon_command(&mut cmd, &path, Some(shell), log_file)?;
        if let Ok(child) = cmd.spawn() {
            return Ok(child);
        }
        // The shell passed the permission checks yet could not be launched
        // (ENOEXEC, and the like). Losing the account environment degrades the
        // daemon; refusing to start it strands the app entirely.
    }

    let mut cmd = Command::new(bin);
    cmd.arg("--http-addr").arg(http_addr);
    configure_daemon_command(&mut cmd, &path, shell.as_deref(), log_file)?;
    cmd.spawn()
}

/// configure_daemon_command applies everything both launch shapes share: the
/// bootstrap PATH, the account shell, the daemon log as stdout+stderr, and the
/// new session.
fn configure_daemon_command(
    cmd: &mut Command,
    path: &OsString,
    shell: Option<&Path>,
    log_file: &Path,
) -> std::io::Result<()> {
    let log = std::fs::OpenOptions::new()
        .create(true)
        .append(true)
        .open(log_file)?;
    let errlog = log.try_clone()?;
    cmd.env("PATH", path)
        .stdin(Stdio::null())
        .stdout(Stdio::from(log))
        .stderr(Stdio::from(errlog));
    match shell {
        Some(shell) => {
            cmd.env("SHELL", shell);
        }
        None => {
            cmd.env_remove("SHELL");
        }
    }
    // setsid puts the daemon in its own session so quitting the app (or a Ctrl-C
    // in `cargo tauri dev`) never signals it. The daemon outliving the window is
    // the intended behaviour: it is a background service with agents running.
    unsafe {
        use std::os::unix::process::CommandExt;
        cmd.pre_exec(|| {
            libc::setsid();
            Ok(())
        });
    }
    Ok(())
}

/// login_shell_command builds the one command string the login shell runs.
/// Everything interpolated is single-quoted, so a bundle path with spaces or a
/// shell metacharacter cannot change what gets executed.
fn login_shell_command(bin: &Path, http_addr: &str) -> String {
    format!(
        "exec {} --http-addr {}",
        sh_quote(&bin.to_string_lossy()),
        sh_quote(http_addr)
    )
}

fn sh_quote(value: &str) -> String {
    format!("'{}'", value.replace('\'', r"'\''"))
}

/// wait_ready polls the control socket until the daemon answers.
pub fn wait_ready(sock: &Path, timeout: Duration, poll: Duration) -> Result<Status, String> {
    let deadline = std::time::Instant::now() + timeout;
    loop {
        if let Probe::Up(st) = probe(sock, Duration::from_secs(1)) {
            return Ok(st.unwrap_or_default());
        }
        if std::time::Instant::now() >= deadline {
            return Err(format!("tariboyd did not become ready in {timeout:?}"));
        }
        std::thread::sleep(poll);
    }
}

/// ensure_up adopts a running daemon or starts the bundled one.
///
/// Adoption comes first and is never skipped: the base dir is shared with the
/// CLI, and a second daemon on it would double-run every agent loop.
pub fn ensure_up(cfg: &Config) -> Result<Ready, String> {
    if let Probe::Up(st) = probe(&cfg.socket, Duration::from_secs(1)) {
        return Ok(Ready {
            status: st.unwrap_or_default(),
            adopted: true,
            child: None,
        });
    }

    let port = pick_port();
    if port == 0 {
        return Err("could not find a free loopback port for the daemon".into());
    }
    let addr = format!("127.0.0.1:{port}");
    let child = spawn(&cfg.daemon_bin, &addr, &cfg.runtime_dir, &cfg.log_file)
        .map_err(|e| format!("start {}: {e}", cfg.daemon_bin.display()))?;

    match wait_ready(&cfg.socket, cfg.ready_timeout, cfg.poll_interval) {
        Ok(mut status) => {
            // A daemon too old to report http_addr still listens where we told it
            // to, so fall back to the address we just chose.
            if status.http_addr.is_empty() {
                status.http_addr = addr;
            }
            Ok(Ready {
                status,
                adopted: false,
                child: Some(child),
            })
        }
        Err(e) => {
            // Reaped, NOT killed. A daemon that missed the ready_timeout may
            // still be coming up — a first-run SQLite migration on a cold cache
            // is exactly what the budget exists for — and killing it could cut a
            // migration in half. But dropping the handle is not an option either:
            // Child::drop neither kills nor waits, so the process would become a
            // ZOMBIE of this app when it eventually exits, and kill(pid, 0)
            // succeeds on a zombie — which is precisely the bug watch_exit was
            // written to fix (see its doc comment): a later stop() would poll a
            // corpse for the whole escalation budget and always SIGKILL. So hand
            // it to a detached reaper and let it live or die on its own.
            watch_exit(child, || {});
            Err(format!("{e}; last log:\n{}", log_tail(&cfg.log_file, 40)))
        }
    }
}

/// log_tail returns the last `n` lines of the daemon log, empty on any error.
/// Mirrors internal/daemonctl.tail, which is what a failed start prints.
pub fn log_tail(path: &Path, n: usize) -> String {
    let Ok(s) = std::fs::read_to_string(path) else {
        return String::new();
    };
    let lines: Vec<&str> = s.trim_end_matches('\n').lines().collect();
    let start = lines.len().saturating_sub(n);
    lines[start..].join("\n")
}

/// read_pid returns a plausible pid from the pidfile, or None for a missing,
/// unparseable or non-positive value.
pub fn read_pid(pid_file: &Path) -> Option<i32> {
    std::fs::read_to_string(pid_file)
        .ok()?
        .trim()
        .parse::<i32>()
        .ok()
        .filter(|p| *p > 0)
}

/// pid_alive reports whether a pid exists (kill(pid, 0) succeeds).
pub fn pid_alive(pid: i32) -> bool {
    unsafe { libc::kill(pid, 0) == 0 }
}

/// stop mirrors internal/daemonctl.Down: SIGTERM, poll, escalate to SIGKILL after
/// the timeout, then clear the pidfile.
///
/// A daemon that ANSWERS but has no pidfile is refused rather than guessed at:
/// the base dir is shared, and killing the wrong process would take out agent
/// sessions belonging to the CLI.
pub fn stop(cfg: &Config) -> Result<(), String> {
    let Some(pid) = read_pid(&cfg.pid_file) else {
        if let Probe::Up(_) = probe(&cfg.socket, Duration::from_secs(1)) {
            return Err(format!(
                "daemon appears up but no pidfile at {}; stop it manually",
                cfg.pid_file.display()
            ));
        }
        return Ok(());
    };

    unsafe { libc::kill(pid, libc::SIGTERM) };
    let deadline = std::time::Instant::now() + cfg.ready_timeout;
    while pid_alive(pid) {
        if std::time::Instant::now() >= deadline {
            unsafe { libc::kill(pid, libc::SIGKILL) };
            break;
        }
        std::thread::sleep(cfg.poll_interval);
    }
    let _ = std::fs::remove_file(&cfg.pid_file);
    Ok(())
}

/// wait_pid_gone blocks until the process is confirmed gone or the deadline
/// passes.
fn wait_pid_gone(pid: i32, timeout: Duration, poll: Duration) -> bool {
    let deadline = std::time::Instant::now() + timeout;
    loop {
        if !pid_alive(pid) {
            return true;
        }
        if std::time::Instant::now() >= deadline {
            return false;
        }
        std::thread::sleep(poll);
    }
}

/// restart stops then starts.
///
/// The old pid is captured BEFORE stopping and waited out afterwards: on the
/// SIGKILL path the dying daemon's socket can still answer for a moment, and
/// ensure_up gates on exactly that socket — so without the wait, restart could
/// "adopt" the corpse and report success while leaving nothing running. This is
/// the same hazard internal/daemonctl.Restart documents.
pub fn restart(cfg: &Config) -> Result<Ready, String> {
    let old = read_pid(&cfg.pid_file);
    stop(cfg)?;
    if let Some(pid) = old {
        wait_pid_gone(pid, cfg.ready_timeout, cfg.poll_interval);
    }
    ensure_up(cfg)
}

/// watch_exit takes ownership of a daemon WE started and reaps it the moment it
/// exits, then runs `on_exit`.
///
/// Reaping is not tidiness, it is correctness. An un-waited child becomes a
/// ZOMBIE, and `kill(pid, 0)` succeeds on a zombie — so `stop` would poll a
/// corpse for the entire escalation budget and then SIGKILL it every single
/// time, erasing the graceful/forced distinction and freezing the caller for the
/// full timeout. internal/daemonctl never hits this because it calls
/// Process.Release() and does not hold the daemon as a child; we keep the handle
/// so we can notice the daemon dying, which means we must reap it ourselves.
///
/// `on_exit` also gives death detection for free: it fires the instant the
/// daemon goes away, so the UI can flip to `down` without polling for it.
pub fn watch_exit(mut child: Child, on_exit: impl FnOnce() + Send + 'static) {
    std::thread::spawn(move || {
        let _ = child.wait();
        on_exit();
    });
}

// pub(crate) so state.rs's tests can reuse `fake_daemon` rather than growing a
// second copy of the same stub listener. Test-only, so it costs nothing shipped.
#[cfg(test)]
pub(crate) mod tests {
    use super::*;
    use std::io::{Read, Write};
    use std::os::unix::net::UnixListener;
    use std::path::PathBuf;
    use std::thread;

    pub(crate) const OK_BODY: &str = r#"{"ok":true,"result":{"version":"1.2.3","pid":4242,"base_dir":"/base","http_addr":"127.0.0.1:9990","schema_version":21}}"#;
    const ERR_BODY: &str = r#"{"ok":false,"error":{"code":"boom","message":"nope"}}"#;

    /// fake_daemon binds `sock` and answers every connection with `body`, the way
    /// tariboyd's unix listener would. It runs until the test process exits.
    pub(crate) fn fake_daemon(sock: PathBuf, body: &'static str) {
        let ln = UnixListener::bind(&sock).expect("bind fake socket");
        thread::spawn(move || {
            for conn in ln.incoming() {
                let Ok(mut c) = conn else { return };
                let mut buf = [0u8; 2048];
                let _ = c.read(&mut buf);
                let resp = format!(
                    "HTTP/1.0 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\n\r\n{}",
                    body.len(),
                    body
                );
                let _ = c.write_all(resp.as_bytes());
            }
        });
    }

    #[test]
    fn probe_reads_the_status_from_a_live_daemon() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("tariboyd.sock");
        fake_daemon(sock.clone(), OK_BODY);

        match probe(&sock, Duration::from_secs(2)) {
            Probe::Up(Some(st)) => {
                assert_eq!(st.version, "1.2.3");
                assert_eq!(st.pid, 4242);
                assert_eq!(st.base_dir, "/base");
                assert_eq!(st.http_addr, "127.0.0.1:9990");
            }
            other => panic!("probe = {other:?}, want Up(Some(..))"),
        }
    }

    #[test]
    fn probe_reports_down_when_the_socket_is_absent() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("nope.sock");
        assert_eq!(probe(&sock, Duration::from_secs(1)), Probe::Down);
    }

    // The rule that keeps a second daemon off a shared base dir: an API error is
    // still a live process.
    #[test]
    fn probe_treats_an_api_error_as_alive() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("tariboyd.sock");
        fake_daemon(sock.clone(), ERR_BODY);
        assert_eq!(probe(&sock, Duration::from_secs(2)), Probe::Up(None));
    }

    // A stale socket FILE with nothing listening must read as down, otherwise the
    // app would sit forever waiting for a daemon that will never answer.
    #[test]
    fn probe_reports_down_for_a_stale_socket_file() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("tariboyd.sock");
        std::fs::write(&sock, b"not a socket").unwrap();
        assert_eq!(probe(&sock, Duration::from_secs(1)), Probe::Down);
    }

    #[test]
    fn parse_status_unwraps_the_ok_envelope() {
        let st = parse_status(OK_BODY).expect("parse");
        assert_eq!(st.version, "1.2.3");
    }

    #[test]
    fn parse_status_rejects_an_error_envelope_and_garbage() {
        assert!(parse_status(ERR_BODY).is_none());
        assert!(parse_status("not json").is_none());
        assert!(parse_status("").is_none());
    }

    // A daemon predating the http_addr field must still parse; the app then
    // renders the "no HTTP listener" banner rather than crashing.
    #[test]
    fn parse_status_tolerates_a_missing_http_addr() {
        let st =
            parse_status(r#"{"ok":true,"result":{"version":"0.8.0","pid":1}}"#).expect("parse");
        assert_eq!(st.http_addr, "");
        assert_eq!(st.version, "0.8.0");
    }

    use std::time::Instant;

    fn cfg_for(dir: &std::path::Path, bin: PathBuf) -> Config {
        Config {
            socket: dir.join("tariboyd.sock"),
            pid_file: dir.join("tariboyd.pid"),
            log_file: dir.join("tariboyd.log"),
            runtime_dir: dir.to_path_buf(),
            daemon_bin: bin,
            ready_timeout: Duration::from_millis(1500),
            poll_interval: Duration::from_millis(50),
        }
    }

    /// stub_daemon writes an executable script that echoes its argv (to stdout,
    /// i.e. into the daemon log), captures its selected shell, path, pid and
    /// launcher marker in test-private files, and touches a marker file so a
    /// test can assert whether it ran at all.
    fn stub_daemon(dir: &std::path::Path, marker: &std::path::Path) -> PathBuf {
        let bin = dir.join("stub-tariboyd.sh");
        let shell_out = dir.join("daemon.shell");
        let path_out = dir.join("daemon.path");
        let pid_out = dir.join("daemon.pid");
        let marker_out = dir.join("daemon.shellenv");
        std::fs::write(
            &bin,
            format!(
                "#!/bin/sh\ntouch '{}'\necho \"argv: $@\"\nprintf '%s\\n' \"$SHELL\" > \
                 '{}'\nprintf '%s\\n' \"$PATH\" > '{}'\nprintf '%s\\n' \"$$\" > \
                 '{}'\nprintf '%s\\n' \"$TARIBOY_SHELL_ENV\" > '{}'\n",
                marker.display(),
                shell_out.display(),
                path_out.display(),
                pid_out.display(),
                marker_out.display()
            ),
        )
        .unwrap();
        use std::os::unix::fs::PermissionsExt;
        std::fs::set_permissions(&bin, std::fs::Permissions::from_mode(0o755)).unwrap();
        bin
    }

    /// stub_login_shell stands in for the account's real login shell: it records
    /// how the launcher invoked it, then runs the command string it was given so
    /// the daemon underneath still starts.
    fn stub_login_shell(dir: &std::path::Path) -> PathBuf {
        let shell = dir.join("login-shell");
        let argv_out = dir.join("shell.argv");
        std::fs::write(
            &shell,
            format!(
                "#!/bin/sh\nprintf '%s\\n' \"$@\" > '{}'\nexec /bin/sh -c \"$2\"\n",
                argv_out.display()
            ),
        )
        .unwrap();
        use std::os::unix::fs::PermissionsExt;
        std::fs::set_permissions(&shell, std::fs::Permissions::from_mode(0o755)).unwrap();
        shell
    }

    #[test]
    fn pick_port_returns_a_bindable_port() {
        let p = pick_port();
        assert!(p > 0);
        std::net::TcpListener::bind(("127.0.0.1", p)).expect("returned port is bindable");
    }

    // 9990 is preferred, but a busy 9990 must yield some other free port rather
    // than a start failure.
    #[test]
    fn pick_port_falls_back_when_9990_is_busy() {
        let held = std::net::TcpListener::bind(("127.0.0.1", 9990));
        let p = pick_port();
        if held.is_ok() {
            assert_eq!(p, 9990);
        } else {
            assert_ne!(p, 9990);
            assert!(p > 0);
        }
    }

    #[test]
    fn spawn_supplies_the_account_shell_bootstrap_path_and_http_addr() {
        let dir = tempfile::tempdir().unwrap();
        let marker = dir.path().join("ran");
        let bin = stub_daemon(dir.path(), &marker);
        let log = dir.path().join("tariboyd.log");
        let account_shell = stub_login_shell(dir.path());

        let mut child = spawn_with_environment(
            &bin,
            "127.0.0.1:9993",
            dir.path(),
            &log,
            LaunchEnvironment {
                account_shell: Some(account_shell.clone()),
                inherited_shell: Some(std::ffi::OsString::from("/bin/sh")),
                inherited_path: Some(std::ffi::OsString::from("/gui/bin")),
                ..Default::default()
            },
        )
        .expect("spawn");
        let _ = child.wait();

        assert!(marker.exists(), "stub daemon never executed");
        let text = std::fs::read_to_string(&log).unwrap();
        assert!(
            text.contains("--http-addr 127.0.0.1:9993"),
            "log did not capture the flags: {text}"
        );
        assert_eq!(
            std::fs::read_to_string(dir.path().join("daemon.shell"))
                .unwrap()
                .trim_end(),
            account_shell.as_os_str()
        );
        let daemon_path = std::fs::read_to_string(dir.path().join("daemon.path")).unwrap();
        assert!(
            daemon_path.starts_with("/gui/bin:"),
            "spawned daemon did not preserve the inherited PATH prefix"
        );
        assert!(
            daemon_path
                .split(':')
                .any(|entry| entry == "/opt/homebrew/bin"),
            "spawned daemon did not receive the Homebrew PATH fallback"
        );
    }

    #[test]
    fn account_record_shell_accepts_an_absolute_executable() {
        let dir = tempfile::tempdir().unwrap();
        let shell = dir.path().join("login-shell");
        std::fs::write(&shell, "#!/bin/sh\n").unwrap();
        use std::os::unix::ffi::OsStrExt;
        use std::os::unix::fs::PermissionsExt;
        std::fs::set_permissions(&shell, std::fs::Permissions::from_mode(0o755)).unwrap();

        assert_eq!(
            account_record_shell(shell.as_os_str().as_bytes()),
            Some(shell)
        );
    }

    #[test]
    fn account_record_shell_rejects_unusable_paths() {
        let dir = tempfile::tempdir().unwrap();
        let relative = "relative-shell";
        let non_executable = dir.path().join("non-executable");
        std::fs::write(&non_executable, "#!/bin/sh\n").unwrap();
        use std::os::unix::ffi::OsStrExt;

        for (name, candidate) in [
            ("empty", b"".as_slice()),
            ("relative", relative.as_bytes()),
            ("directory", dir.path().as_os_str().as_bytes()),
            ("non-executable", non_executable.as_os_str().as_bytes()),
        ] {
            assert_eq!(
                account_record_shell(candidate),
                None,
                "accepted unusable {name} shell"
            );
        }
    }

    #[test]
    fn daemon_shell_falls_back_only_to_a_valid_inherited_shell() {
        assert_eq!(
            daemon_shell(None, Some(std::ffi::OsString::from("/bin/sh"))),
            Some(PathBuf::from("/bin/sh"))
        );
        assert_eq!(
            daemon_shell(
                None,
                Some(std::ffi::OsString::from("relative-inherited-shell"))
            ),
            None
        );
        assert_eq!(daemon_shell(None, None), None);
    }

    #[test]
    fn daemon_path_adds_homebrew_bin_missing_from_gui_environment() {
        let got = daemon_path(Some(std::ffi::OsString::from("/usr/bin:/bin")), None);
        let paths = std::env::split_paths(&got).collect::<Vec<_>>();

        assert_eq!(paths[0], PathBuf::from("/usr/bin"));
        assert_eq!(paths[1], PathBuf::from("/bin"));
        assert!(
            paths.contains(&PathBuf::from("/opt/homebrew/bin")),
            "Apple Silicon Homebrew bin missing from daemon PATH: {paths:?}"
        );
        assert!(
            paths.contains(&PathBuf::from("/usr/local/bin")),
            "Intel Homebrew bin missing from daemon PATH: {paths:?}"
        );
    }

    /// The daemon must be born inside the account's login shell so every agent it
    /// launches inherits the same environment a terminal gets. Resolving only PATH
    /// from a separate probe left the daemon blind whenever that one probe failed.
    #[test]
    fn spawn_launches_the_daemon_through_the_login_shell() {
        let dir = tempfile::tempdir().unwrap();
        let marker = dir.path().join("ran");
        let bin = stub_daemon(dir.path(), &marker);
        let log = dir.path().join("tariboyd.log");
        let shell = stub_login_shell(dir.path());

        let mut child = spawn_with_environment(
            &bin,
            "127.0.0.1:9993",
            dir.path(),
            &log,
            LaunchEnvironment {
                account_shell: Some(shell.clone()),
                inherited_path: Some(std::ffi::OsString::from("/gui/bin")),
                ..Default::default()
            },
        )
        .expect("spawn");
        let pid = child.id();
        let _ = child.wait();

        assert!(marker.exists(), "stub daemon never executed");
        let argv = std::fs::read_to_string(dir.path().join("shell.argv"))
            .expect("login shell was not invoked at all");
        let args: Vec<&str> = argv.lines().collect();
        assert_eq!(
            args[0], "-lic",
            "login shell must run its startup files interactively: {args:?}"
        );
        assert!(
            args[1].starts_with("exec ") && args[1].contains(bin.to_str().unwrap()),
            "shell command string lost the daemon invocation: {args:?}"
        );
        let logged = std::fs::read_to_string(&log).unwrap();
        assert!(
            logged.contains("--http-addr 127.0.0.1:9993"),
            "daemon did not receive its flags through the shell: {logged}"
        );
        assert_eq!(
            std::fs::read_to_string(dir.path().join("daemon.pid"))
                .unwrap()
                .trim_end(),
            pid.to_string(),
            "the shell must exec the daemon so the pid the app tracks stays the daemon's"
        );
    }

    /// Running the login shell costs a second or more of the account's startup
    /// files. The daemon must be told that its environment already came from
    /// there, so it does not pay that cost a second time with its own PATH probe.
    #[test]
    fn spawn_through_the_login_shell_marks_the_environment_as_resolved() {
        let dir = tempfile::tempdir().unwrap();
        let marker = dir.path().join("ran");
        let bin = stub_daemon(dir.path(), &marker);
        let log = dir.path().join("tariboyd.log");
        let shell = stub_login_shell(dir.path());

        let mut child = spawn_with_environment(
            &bin,
            "127.0.0.1:9993",
            dir.path(),
            &log,
            LaunchEnvironment {
                account_shell: Some(shell),
                ..Default::default()
            },
        )
        .expect("spawn");
        let _ = child.wait();

        assert_eq!(
            std::fs::read_to_string(dir.path().join("daemon.shellenv"))
                .unwrap()
                .trim_end(),
            "1",
            "daemon was not told its environment came from the login shell"
        );
    }

    /// An account shell that passes the permission checks can still be
    /// unlaunchable — a `chsh` wrapper script whose interpreter is gone is the
    /// usual shape. Losing the account environment degrades the daemon; refusing
    /// to start it at all strands the app.
    #[test]
    fn spawn_falls_back_to_a_direct_launch_when_the_login_shell_cannot_run() {
        let dir = tempfile::tempdir().unwrap();
        let marker = dir.path().join("ran");
        let bin = stub_daemon(dir.path(), &marker);
        let log = dir.path().join("tariboyd.log");
        let broken = dir.path().join("broken-shell");
        std::fs::write(&broken, "#!/nonexistent/interpreter\n").unwrap();
        use std::os::unix::fs::PermissionsExt;
        std::fs::set_permissions(&broken, std::fs::Permissions::from_mode(0o755)).unwrap();

        let mut child = spawn_with_environment(
            &bin,
            "127.0.0.1:9993",
            dir.path(),
            &log,
            LaunchEnvironment {
                account_shell: Some(broken),
                ..Default::default()
            },
        )
        .expect("spawn must fall back rather than fail");
        let _ = child.wait();

        assert!(
            marker.exists(),
            "daemon never started after the shell failed"
        );
        assert_eq!(
            std::fs::read_to_string(dir.path().join("daemon.shellenv"))
                .unwrap()
                .trim_end(),
            "",
            "fallback launch must not claim a login-shell environment"
        );
    }

    /// Without a usable account shell there is nothing to inherit from, and the
    /// daemon must still start — directly, and without claiming a shell
    /// environment it never got.
    #[test]
    fn spawn_without_a_login_shell_execs_the_daemon_directly() {
        let dir = tempfile::tempdir().unwrap();
        let marker = dir.path().join("ran");
        let bin = stub_daemon(dir.path(), &marker);
        let log = dir.path().join("tariboyd.log");

        let mut child = spawn_with_environment(
            &bin,
            "127.0.0.1:9993",
            dir.path(),
            &log,
            LaunchEnvironment::default(),
        )
        .expect("spawn");
        let _ = child.wait();

        assert!(marker.exists(), "stub daemon never executed");
        assert_eq!(
            std::fs::read_to_string(dir.path().join("daemon.shellenv"))
                .unwrap()
                .trim_end(),
            "",
            "direct launch must not claim a login-shell environment"
        );
    }

    /// The app installs its own CLI into ~/.local/bin, and harnesses installed by
    /// their own installers land there too. A GUI launch never inherits it, so a
    /// daemon whose login-shell environment is unavailable must still find it.
    #[test]
    fn daemon_path_adds_the_account_local_bin_missing_from_gui_environment() {
        let got = daemon_path(
            Some(std::ffi::OsString::from("/usr/bin:/bin")),
            Some(std::ffi::OsString::from("/Users/tester")),
        );
        let paths = std::env::split_paths(&got).collect::<Vec<_>>();

        assert!(
            paths.contains(&PathBuf::from("/Users/tester/.local/bin")),
            "account ~/.local/bin missing from daemon PATH: {paths:?}"
        );
    }

    #[test]
    fn daemon_path_without_a_home_keeps_the_system_fallbacks() {
        let got = daemon_path(Some(std::ffi::OsString::from("/usr/bin:/bin")), None);
        let paths = std::env::split_paths(&got).collect::<Vec<_>>();

        assert!(
            paths.contains(&PathBuf::from("/opt/homebrew/bin")),
            "system fallbacks lost when HOME is unset: {paths:?}"
        );
    }

    #[test]
    fn wait_ready_returns_once_the_socket_appears() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("tariboyd.sock");
        let late = sock.clone();
        thread::spawn(move || {
            thread::sleep(Duration::from_millis(200));
            fake_daemon(late, OK_BODY);
        });

        let started = Instant::now();
        let st = wait_ready(&sock, Duration::from_secs(3), Duration::from_millis(50))
            .expect("became ready");
        assert_eq!(st.version, "1.2.3");
        assert!(started.elapsed() >= Duration::from_millis(150));
    }

    #[test]
    fn wait_ready_times_out_when_nothing_ever_listens() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("tariboyd.sock");
        let err = wait_ready(&sock, Duration::from_millis(200), Duration::from_millis(50))
            .expect_err("must time out");
        assert!(err.contains("did not become ready"), "err = {err}");
    }

    // Adoption: a daemon is already answering, so nothing is spawned. This is the
    // guard against a second daemon on a shared base dir.
    #[test]
    fn ensure_up_adopts_a_running_daemon_without_spawning() {
        let dir = tempfile::tempdir().unwrap();
        let marker = dir.path().join("ran");
        let bin = stub_daemon(dir.path(), &marker);
        fake_daemon(dir.path().join("tariboyd.sock"), OK_BODY);

        let ready = ensure_up(&cfg_for(dir.path(), bin)).expect("ensure_up");
        assert!(ready.adopted, "must adopt, not spawn");
        assert!(ready.child.is_none());
        assert_eq!(ready.status.version, "1.2.3");
        assert!(
            !marker.exists(),
            "a daemon was spawned despite one already running"
        );
    }

    // A start that never becomes ready must surface the log tail, not a bare
    // timeout: the log is where the real cause (bad exec, port clash) is.
    #[test]
    fn ensure_up_reports_the_log_tail_when_the_daemon_never_answers() {
        let dir = tempfile::tempdir().unwrap();
        let marker = dir.path().join("ran");
        let bin = stub_daemon(dir.path(), &marker);

        let err = ensure_up(&cfg_for(dir.path(), bin)).expect_err("must fail");
        assert!(marker.exists(), "the stub daemon should have been executed");
        assert!(err.contains("did not become ready"), "err = {err}");
        assert!(
            err.contains("argv: --http-addr"),
            "log tail missing from err: {err}"
        );
    }

    /// slow_stub_daemon never listens and outlives `ensure_up`'s budget, so the
    /// timeout fires while the child is still ALIVE — the exact window in which
    /// the handle used to be dropped. It records its own pid so the test can
    /// watch for the corpse.
    fn slow_stub_daemon(dir: &std::path::Path, pid_out: &std::path::Path) -> PathBuf {
        let bin = dir.join("slow-tariboyd.sh");
        std::fs::write(
            &bin,
            format!("#!/bin/sh\necho $$ > '{}'\nsleep 1\n", pid_out.display()),
        )
        .unwrap();
        use std::os::unix::fs::PermissionsExt;
        std::fs::set_permissions(&bin, std::fs::Permissions::from_mode(0o755)).unwrap();
        bin
    }

    // Regression: the error path must REAP the child it spawned. Child::drop
    // neither kills nor waits, so a daemon slower than ready_timeout used to be
    // left as a zombie — and kill(pid, 0) succeeds on a zombie, which is what
    // makes a later stop() burn its whole escalation budget and SIGKILL.
    //
    // pid_alive is the assertion on purpose: it keeps returning true for an
    // unreaped zombie and only goes false once the child has actually been
    // waited on, so without the fix this loop never terminates.
    #[test]
    fn ensure_up_reaps_the_child_when_the_start_times_out() {
        let dir = tempfile::tempdir().unwrap();
        let pid_out = dir.path().join("stub.pid");
        let mut cfg = cfg_for(dir.path(), slow_stub_daemon(dir.path(), &pid_out));
        cfg.ready_timeout = Duration::from_millis(250);
        cfg.poll_interval = Duration::from_millis(25);

        let err = ensure_up(&cfg).expect_err("the stub never listens");
        assert!(err.contains("did not become ready"), "err = {err}");

        // The stub sleeps a full second, so it is still running right here.
        let pid = read_pid_eventually(&pid_out);
        assert!(pid_alive(pid), "the stub should outlive the ready timeout");

        let deadline = Instant::now() + Duration::from_secs(10);
        while pid_alive(pid) {
            assert!(
                Instant::now() < deadline,
                "pid {pid} still exists long after the stub exited: the child was \
                 dropped instead of reaped and is now a zombie"
            );
            thread::sleep(Duration::from_millis(25));
        }
    }

    /// read_pid_eventually waits for the stub to record its pid.
    fn read_pid_eventually(path: &std::path::Path) -> i32 {
        let deadline = Instant::now() + Duration::from_secs(5);
        loop {
            if let Some(pid) = read_pid(path) {
                return pid;
            }
            assert!(Instant::now() < deadline, "stub never wrote its pid");
            thread::sleep(Duration::from_millis(10));
        }
    }

    #[test]
    fn ensure_up_fails_loudly_when_the_binary_is_missing() {
        let dir = tempfile::tempdir().unwrap();
        let err = ensure_up(&cfg_for(dir.path(), dir.path().join("no-such-binary")))
            .expect_err("must fail");
        assert!(err.contains("no-such-binary"), "err = {err}");
    }

    /// long_sleeper spawns a real child we can signal, and writes its pid where
    /// the daemon would.
    fn long_sleeper(pid_file: &std::path::Path) -> Child {
        let child = Command::new("/bin/sh")
            .arg("-c")
            .arg("sleep 30")
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .spawn()
            .expect("spawn sleeper");
        std::fs::write(pid_file, format!("{}\n", child.id())).unwrap();
        child
    }

    #[test]
    fn read_pid_parses_the_pidfile_and_rejects_junk() {
        let dir = tempfile::tempdir().unwrap();
        let p = dir.path().join("tariboyd.pid");
        std::fs::write(&p, " 1234 \n").unwrap();
        assert_eq!(read_pid(&p), Some(1234));

        std::fs::write(&p, "not-a-pid").unwrap();
        assert_eq!(read_pid(&p), None);

        std::fs::write(&p, "0").unwrap();
        assert_eq!(read_pid(&p), None);

        assert_eq!(read_pid(&dir.path().join("absent.pid")), None);
    }

    #[test]
    fn stop_terminates_the_process_and_clears_the_pidfile() {
        let dir = tempfile::tempdir().unwrap();
        let cfg = cfg_for(dir.path(), dir.path().join("unused"));
        let mut child = long_sleeper(&cfg.pid_file);
        let pid = child.id() as i32;

        stop(&cfg).expect("stop");

        assert!(!cfg.pid_file.exists(), "pidfile should be gone");
        let _ = child.wait();
        assert!(!pid_alive(pid), "process should be gone");
    }

    #[test]
    fn stop_is_a_no_op_when_nothing_is_running() {
        let dir = tempfile::tempdir().unwrap();
        let cfg = cfg_for(dir.path(), dir.path().join("unused"));
        assert!(stop(&cfg).is_ok());
    }

    // A daemon answering with no pidfile is the one case we refuse to touch:
    // guessing which process to kill on a shared base dir is how sessions get
    // reaped out from under the CLI.
    #[test]
    fn stop_refuses_when_the_daemon_answers_but_has_no_pidfile() {
        let dir = tempfile::tempdir().unwrap();
        let cfg = cfg_for(dir.path(), dir.path().join("unused"));
        fake_daemon(cfg.socket.clone(), OK_BODY);
        let err = stop(&cfg).expect_err("must refuse");
        assert!(err.contains("no pidfile"), "err = {err}");
    }

    #[test]
    fn log_tail_returns_the_last_lines_and_survives_a_missing_file() {
        let dir = tempfile::tempdir().unwrap();
        let f = dir.path().join("tariboyd.log");
        std::fs::write(&f, "a\nb\nc\nd\n").unwrap();
        assert_eq!(log_tail(&f, 2), "c\nd");
        assert_eq!(log_tail(&f, 99), "a\nb\nc\nd");
        assert_eq!(log_tail(&dir.path().join("absent.log"), 5), "");
    }

    // Regression: stop() must not spend the whole escalation budget on a corpse.
    // A child we spawned and never waited on becomes a zombie, and kill(pid, 0)
    // succeeds on zombies — so before watch_exit existed, this took exactly
    // ready_timeout and always escalated to SIGKILL.
    #[test]
    fn watch_exit_reaps_so_a_graceful_stop_is_prompt() {
        let dir = tempfile::tempdir().unwrap();
        let cfg = cfg_for(dir.path(), dir.path().join("unused"));
        let child = long_sleeper(&cfg.pid_file);
        let pid = child.id() as i32;

        let (tx, rx) = std::sync::mpsc::channel();
        watch_exit(child, move || {
            let _ = tx.send(());
        });

        let started = Instant::now();
        stop(&cfg).expect("stop");
        let elapsed = started.elapsed();

        rx.recv_timeout(Duration::from_secs(2))
            .expect("exit callback must fire when the daemon dies");
        assert!(!pid_alive(pid), "process should be reaped and gone");
        assert!(
            elapsed < cfg.ready_timeout,
            "graceful stop took {elapsed:?}, i.e. the full {:?} timeout — the \
             child was not reaped and stop escalated to SIGKILL",
            cfg.ready_timeout
        );
    }
}
