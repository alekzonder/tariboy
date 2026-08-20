use crate::{daemon, remote_health, ssh};
use std::collections::HashMap;
use std::io::Read;
use std::net::{Ipv4Addr, TcpListener};
use std::path::PathBuf;
use std::process::{Child, Command, Stdio};
use std::sync::atomic::{AtomicBool as ShutdownFlag, AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use std::thread;
use std::time::{Duration, Instant};

const STDERR_LIMIT: usize = 64 * 1024;

#[derive(Debug, Clone)]
pub struct Config {
    pub host_id: String,
    pub alias: String,
    pub remote_port: u16,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum State {
    Connecting,
    Ready,
    Retrying,
    NeedsAuth,
    Failed,
    Disconnected,
}

#[derive(Debug, Clone)]
pub struct Event {
    pub host_id: String,
    pub state: State,
    pub local_port: u16,
    pub status: Option<daemon::Status>,
    pub message: String,
}

pub type EventSink = Arc<dyn Fn(Event) + Send + Sync>;

pub trait TunnelChild: Send {
    fn try_wait(&mut self) -> std::io::Result<Option<bool>>;
    fn terminate(&mut self);
    fn stderr(&self) -> String;
}

pub trait Runner: Send + Sync {
    fn spawn(
        &self,
        alias: &str,
        local_port: u16,
        remote_port: u16,
    ) -> Result<Box<dyn TunnelChild>, String>;
}

pub struct SystemRunner {
    ssh: PathBuf,
}

impl Default for SystemRunner {
    fn default() -> Self {
        Self {
            ssh: PathBuf::from("/usr/bin/ssh"),
        }
    }
}

impl SystemRunner {
    #[cfg(test)]
    fn new(ssh: PathBuf) -> Self {
        Self { ssh }
    }
}

impl Runner for SystemRunner {
    fn spawn(
        &self,
        alias: &str,
        local_port: u16,
        remote_port: u16,
    ) -> Result<Box<dyn TunnelChild>, String> {
        validate_alias(alias)?;
        let forward = format!("127.0.0.1:{local_port}:127.0.0.1:{remote_port}");
        let mut command = Command::new(&self.ssh);
        command
            .args([
                "-o",
                "BatchMode=yes",
                "-o",
                "ExitOnForwardFailure=yes",
                "-N",
                "-L",
            ])
            .arg(&forward)
            .arg(alias)
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::piped());
        let mut child = command
            .spawn()
            .map_err(|error| format!("start SSH tunnel: {error}"))?;
        let stderr = Arc::new(Mutex::new(Vec::new()));
        let capture = stderr.clone();
        let pipe = child
            .stderr
            .take()
            .ok_or_else(|| "SSH tunnel stderr unavailable".to_string())?;
        let reader = thread::spawn(move || read_stderr(pipe, capture));
        Ok(Box::new(SystemChild {
            child: Some(child),
            stderr,
            reader: Some(reader),
        }))
    }
}

struct SystemChild {
    child: Option<Child>,
    stderr: Arc<Mutex<Vec<u8>>>,
    reader: Option<thread::JoinHandle<()>>,
}

impl SystemChild {
    fn join_reader(&mut self) {
        if let Some(reader) = self.reader.take() {
            let _ = reader.join();
        }
    }
}

impl TunnelChild for SystemChild {
    fn try_wait(&mut self) -> std::io::Result<Option<bool>> {
        let status = self
            .child
            .as_mut()
            .expect("tunnel child missing")
            .try_wait()?;
        if let Some(status) = status {
            self.join_reader();
            Ok(Some(status.success()))
        } else {
            Ok(None)
        }
    }

    fn terminate(&mut self) {
        if let Some(mut child) = self.child.take() {
            let _ = child.kill();
            let _ = child.wait();
        }
        self.join_reader();
    }

    fn stderr(&self) -> String {
        let bytes = self
            .stderr
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        String::from_utf8_lossy(&bytes).into_owned()
    }
}

impl Drop for SystemChild {
    fn drop(&mut self) {
        self.terminate();
    }
}

#[derive(Clone)]
pub struct Policy {
    pub ready_timeout: Duration,
    pub probe_timeout: Duration,
    pub health_interval: Duration,
    pub poll_interval: Duration,
    pub unhealthy_limit: usize,
    pub reconnect_delays: Vec<Duration>,
}

impl Default for Policy {
    fn default() -> Self {
        Self {
            ready_timeout: Duration::from_secs(10),
            probe_timeout: Duration::from_secs(1),
            health_interval: Duration::from_secs(2),
            poll_interval: Duration::from_millis(50),
            unhealthy_limit: 3,
            reconnect_delays: reconnect_delays().to_vec(),
        }
    }
}

struct Entry {
    generation: u64,
    cancel: ssh::CancelToken,
    worker: Option<thread::JoinHandle<()>>,
}

pub struct Supervisor {
    runner: Arc<dyn Runner>,
    health: Arc<dyn remote_health::Probe>,
    policy: Policy,
    closed: ShutdownFlag,
    next_generation: AtomicU64,
    entries: Mutex<HashMap<String, Entry>>,
}

impl Default for Supervisor {
    fn default() -> Self {
        Self::new(
            Arc::new(SystemRunner::default()),
            Arc::new(remote_health::HttpProbe),
            Policy::default(),
        )
    }
}

impl Supervisor {
    pub fn new(
        runner: Arc<dyn Runner>,
        health: Arc<dyn remote_health::Probe>,
        policy: Policy,
    ) -> Self {
        Self {
            runner,
            health,
            policy,
            closed: ShutdownFlag::new(false),
            next_generation: AtomicU64::new(1),
            entries: Mutex::new(HashMap::new()),
        }
    }

    #[cfg(test)]
    fn start(&self, config: Config, sink: EventSink) {
        self.start_if(config, sink, || true);
    }

    pub fn start_if(&self, config: Config, sink: EventSink, still_current: impl FnOnce() -> bool) {
        self.start_guarded(config, sink, still_current, || {});
    }

    fn start_guarded(
        &self,
        config: Config,
        sink: EventSink,
        still_current: impl FnOnce() -> bool,
        after_reservation: impl FnOnce(),
    ) {
        let cancel = ssh::CancelToken::default();
        let host_id = config.host_id.clone();
        let generation = self.next_generation.fetch_add(1, Ordering::Relaxed);
        let replaced = {
            let mut entries = self.entries.lock().unwrap();
            if self.closed.load(Ordering::SeqCst) || !still_current() {
                return;
            }
            entries.insert(
                host_id.clone(),
                Entry {
                    generation,
                    cancel: cancel.clone(),
                    worker: None,
                },
            )
        };
        if let Some(replaced) = replaced {
            replaced.cancel.cancel();
            if let Some(worker) = replaced.worker {
                let _ = worker.join();
            }
        }
        after_reservation();
        if cancel.is_cancelled() || self.closed.load(Ordering::SeqCst) {
            return;
        }
        let worker_cancel = cancel.clone();
        let runner = self.runner.clone();
        let health = self.health.clone();
        let policy = self.policy.clone();
        let worker =
            thread::spawn(move || supervise(config, runner, health, policy, worker_cancel, sink));
        let mut entries = self.entries.lock().unwrap();
        match entries.get_mut(&host_id) {
            Some(entry)
                if entry.generation == generation
                    && !entry.cancel.is_cancelled()
                    && !self.closed.load(Ordering::SeqCst) =>
            {
                entry.worker = Some(worker);
            }
            _ => {
                cancel.cancel();
                drop(entries);
                let _ = worker.join();
            }
        }
    }

    pub fn cancel(&self, host_id: &str) {
        let entry = self.entries.lock().unwrap().remove(host_id);
        if let Some(entry) = entry {
            entry.cancel.cancel();
            if let Some(worker) = entry.worker {
                let _ = worker.join();
            }
        }
    }

    pub fn shutdown(&self) {
        self.closed.store(true, Ordering::SeqCst);
        let entries = {
            let mut entries = self.entries.lock().unwrap();
            entries.drain().map(|(_, entry)| entry).collect::<Vec<_>>()
        };
        for entry in &entries {
            entry.cancel.cancel();
        }
        for entry in entries {
            if let Some(worker) = entry.worker {
                let _ = worker.join();
            }
        }
    }
}

fn supervise(
    config: Config,
    runner: Arc<dyn Runner>,
    health: Arc<dyn remote_health::Probe>,
    policy: Policy,
    cancel: ssh::CancelToken,
    sink: EventSink,
) {
    let mut backoff = Backoff::new(policy.reconnect_delays.clone());
    while !cancel.is_cancelled() {
        sink(event(&config.host_id, State::Connecting, 0, None, ""));
        let local_port = match free_loopback_port() {
            Ok(port) => port,
            Err(error) => {
                sink(event(&config.host_id, State::Failed, 0, None, &error));
                return;
            }
        };
        let mut child = match runner.spawn(&config.alias, local_port, config.remote_port) {
            Ok(child) => child,
            Err(error) => {
                if !retry_wait(&config, &cancel, &sink, &mut backoff, &policy, &error) {
                    return;
                }
                continue;
            }
        };
        let ready_deadline = Instant::now() + policy.ready_timeout;
        let mut healthy = false;
        let mut unhealthy = 0usize;
        let mut next_probe = Instant::now();
        let failure = loop {
            if cancel.is_cancelled() {
                child.terminate();
                sink(event(&config.host_id, State::Disconnected, 0, None, ""));
                return;
            }
            match child.try_wait() {
                Ok(Some(_)) => {
                    let message = child.stderr();
                    let classified = ssh::classify_failure(&message);
                    if classified.code == "needs_auth" {
                        sink(event(
                            &config.host_id,
                            State::NeedsAuth,
                            0,
                            None,
                            &classified.message,
                        ));
                        return;
                    }
                    if classified.code == "host_key_mismatch" {
                        sink(event(
                            &config.host_id,
                            State::Failed,
                            0,
                            None,
                            &classified.message,
                        ));
                        return;
                    }
                    break classified.message;
                }
                Err(error) => break format!("wait for SSH tunnel: {error}"),
                Ok(None) => {}
            }
            if Instant::now() >= next_probe {
                match health.status(local_port, policy.probe_timeout) {
                    Ok(status) => {
                        if !healthy {
                            healthy = true;
                            backoff.reset();
                            sink(event(
                                &config.host_id,
                                State::Ready,
                                local_port,
                                Some(status),
                                "",
                            ));
                        }
                        unhealthy = 0;
                    }
                    Err(error) => {
                        if healthy {
                            unhealthy += 1;
                            if unhealthy >= policy.unhealthy_limit {
                                child.terminate();
                                break format!("remote daemon became unhealthy: {error}");
                            }
                        } else if Instant::now() >= ready_deadline {
                            child.terminate();
                            break format!("remote daemon health timed out: {error}");
                        }
                    }
                }
                next_probe = Instant::now()
                    + if healthy {
                        policy.health_interval
                    } else {
                        policy.poll_interval
                    };
            }
            thread::sleep(policy.poll_interval);
        };
        child.terminate();
        if !retry_wait(&config, &cancel, &sink, &mut backoff, &policy, &failure) {
            return;
        }
    }
}

fn retry_wait(
    config: &Config,
    cancel: &ssh::CancelToken,
    sink: &EventSink,
    backoff: &mut Backoff,
    policy: &Policy,
    message: &str,
) -> bool {
    let delay = backoff.next();
    sink(event(
        &config.host_id,
        State::Retrying,
        0,
        None,
        &format!("{message}; retrying in {}s", delay.as_secs_f64()),
    ));
    let deadline = Instant::now() + delay;
    while !cancel.is_cancelled() && Instant::now() < deadline {
        thread::sleep(policy.poll_interval.min(Duration::from_millis(50)));
    }
    !cancel.is_cancelled()
}

fn event(
    host_id: &str,
    state: State,
    local_port: u16,
    status: Option<daemon::Status>,
    message: &str,
) -> Event {
    Event {
        host_id: host_id.into(),
        state,
        local_port,
        status,
        message: message.into(),
    }
}

fn validate_alias(alias: &str) -> Result<(), String> {
    if alias.trim().is_empty() || alias.starts_with('-') || alias.contains('\0') {
        return Err("SSH alias must be non-empty and must not start with '-'".into());
    }
    Ok(())
}

fn read_stderr(mut reader: impl Read, capture: Arc<Mutex<Vec<u8>>>) {
    let mut buffer = [0u8; 4096];
    loop {
        match reader.read(&mut buffer) {
            Ok(0) | Err(_) => return,
            Ok(n) => {
                let mut output = capture
                    .lock()
                    .unwrap_or_else(|poisoned| poisoned.into_inner());
                output.extend_from_slice(&buffer[..n]);
                if output.len() > STDERR_LIMIT {
                    let drain = output.len() - STDERR_LIMIT;
                    output.drain(..drain);
                }
            }
        }
    }
}

fn free_loopback_port() -> Result<u16, String> {
    TcpListener::bind((Ipv4Addr::LOCALHOST, 0))
        .and_then(|listener| listener.local_addr())
        .map(|address| address.port())
        .map_err(|error| format!("reserve loopback tunnel port: {error}"))
}

pub const fn reconnect_delays() -> [Duration; 6] {
    [
        Duration::from_secs(1),
        Duration::from_secs(2),
        Duration::from_secs(5),
        Duration::from_secs(10),
        Duration::from_secs(30),
        Duration::from_secs(30),
    ]
}

struct Backoff {
    delays: Vec<Duration>,
    index: usize,
}

impl Backoff {
    fn new(delays: Vec<Duration>) -> Self {
        Self { delays, index: 0 }
    }

    fn next(&mut self) -> Duration {
        let delay = self
            .delays
            .get(self.index)
            .copied()
            .or_else(|| self.delays.last().copied())
            .unwrap_or_default();
        self.index = self.index.saturating_add(1);
        delay
    }

    fn reset(&mut self) {
        self.index = 0;
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::testbin;
    use std::sync::atomic::{AtomicBool, AtomicUsize, Ordering};
    use std::sync::mpsc;

    struct FakeChild {
        alive: Arc<AtomicBool>,
        reaped: Arc<AtomicBool>,
        stderr: String,
    }

    impl TunnelChild for FakeChild {
        fn try_wait(&mut self) -> std::io::Result<Option<bool>> {
            if self.alive.load(Ordering::SeqCst) {
                Ok(None)
            } else {
                self.reaped.store(true, Ordering::SeqCst);
                Ok(Some(false))
            }
        }

        fn terminate(&mut self) {
            self.alive.store(false, Ordering::SeqCst);
            self.reaped.store(true, Ordering::SeqCst);
        }

        fn stderr(&self) -> String {
            self.stderr.clone()
        }
    }

    struct FakeRunner {
        alive: Arc<AtomicBool>,
        reaped: Arc<AtomicBool>,
        starts: Arc<AtomicUsize>,
        stderr: String,
        fail_spawn: bool,
    }

    impl Runner for FakeRunner {
        fn spawn(
            &self,
            _alias: &str,
            _local_port: u16,
            _remote_port: u16,
        ) -> Result<Box<dyn TunnelChild>, String> {
            self.starts.fetch_add(1, Ordering::SeqCst);
            if self.fail_spawn {
                return Err("spawn failed".into());
            }
            Ok(Box::new(FakeChild {
                alive: self.alive.clone(),
                reaped: self.reaped.clone(),
                stderr: self.stderr.clone(),
            }))
        }
    }

    struct Healthy;

    impl remote_health::Probe for Healthy {
        fn status(&self, _port: u16, _timeout: Duration) -> Result<daemon::Status, String> {
            Ok(daemon::Status {
                version: "1.2.3".into(),
                pid: 42,
                base_dir: "/srv/sa".into(),
                http_addr: "127.0.0.1:9990".into(),
            })
        }
    }

    fn fast_policy() -> Policy {
        Policy {
            ready_timeout: Duration::from_millis(100),
            probe_timeout: Duration::from_millis(10),
            health_interval: Duration::from_millis(10),
            poll_interval: Duration::from_millis(2),
            unhealthy_limit: 2,
            reconnect_delays: vec![Duration::from_millis(200)],
        }
    }

    fn config() -> Config {
        Config {
            host_id: "host-1".into(),
            alias: "prod".into(),
            remote_port: 9990,
        }
    }

    #[test]
    fn system_runner_uses_literal_forwarding_argv() {
        let dir = tempfile::tempdir().unwrap();
        let log = dir.path().join("ssh.log");
        let ssh = testbin::executable(dir.path(), "ssh", &testbin::argv_logger(&log, "exit 0"));
        let runner = SystemRunner::new(ssh);
        let mut child = runner.spawn("prod", 18444, 9990).unwrap();
        while child.try_wait().unwrap().is_none() {
            thread::sleep(Duration::from_millis(1));
        }
        assert_eq!(
            testbin::invocations(&log),
            vec![vec![
                "-o",
                "BatchMode=yes",
                "-o",
                "ExitOnForwardFailure=yes",
                "-N",
                "-L",
                "127.0.0.1:18444:127.0.0.1:9990",
                "prod",
            ]]
        );
    }

    #[test]
    fn retry_schedule_caps_and_resets_after_health() {
        assert_eq!(
            reconnect_delays(),
            [
                Duration::from_secs(1),
                Duration::from_secs(2),
                Duration::from_secs(5),
                Duration::from_secs(10),
                Duration::from_secs(30),
                Duration::from_secs(30),
            ]
        );
        let mut backoff = Backoff::new(reconnect_delays().to_vec());
        assert_eq!(backoff.next(), Duration::from_secs(1));
        assert_eq!(backoff.next(), Duration::from_secs(2));
        backoff.reset();
        assert_eq!(backoff.next(), Duration::from_secs(1));
    }

    #[test]
    fn free_port_is_loopback_bindable() {
        let port = free_loopback_port().unwrap();
        assert_ne!(port, 0);
    }

    #[test]
    fn auth_failure_classification_is_terminal() {
        let error = ssh::classify_failure("Permission denied (publickey).");
        assert_eq!(error.code, "needs_auth");
        let host_key = ssh::classify_failure("Host key verification failed.");
        assert_eq!(host_key.code, "host_key_mismatch");
    }

    #[test]
    fn connecting_becomes_ready_only_after_health_and_reports_ephemeral_port() {
        let alive = Arc::new(AtomicBool::new(true));
        let reaped = Arc::new(AtomicBool::new(false));
        let runner = Arc::new(FakeRunner {
            alive,
            reaped: reaped.clone(),
            starts: Arc::new(AtomicUsize::new(0)),
            stderr: String::new(),
            fail_spawn: false,
        });
        let supervisor = Supervisor::new(runner, Arc::new(Healthy), fast_policy());
        let (tx, rx) = mpsc::channel();
        supervisor.start(config(), Arc::new(move |event| tx.send(event).unwrap()));

        let connecting = rx.recv_timeout(Duration::from_secs(1)).unwrap();
        let ready = rx.recv_timeout(Duration::from_secs(1)).unwrap();

        assert_eq!(connecting.state, State::Connecting);
        assert_eq!(ready.state, State::Ready);
        assert_ne!(ready.local_port, 0);
        assert_eq!(ready.status.unwrap().version, "1.2.3");
        supervisor.cancel("host-1");
        assert!(reaped.load(Ordering::SeqCst));
    }

    #[test]
    fn auth_failure_is_terminal_and_dead_child_is_reaped() {
        let reaped = Arc::new(AtomicBool::new(false));
        let starts = Arc::new(AtomicUsize::new(0));
        let runner = Arc::new(FakeRunner {
            alive: Arc::new(AtomicBool::new(false)),
            reaped: reaped.clone(),
            starts: starts.clone(),
            stderr: "Permission denied (publickey).".into(),
            fail_spawn: false,
        });
        let supervisor = Supervisor::new(runner, Arc::new(Healthy), fast_policy());
        let (tx, rx) = mpsc::channel();
        supervisor.start(config(), Arc::new(move |event| tx.send(event).unwrap()));
        assert_eq!(
            rx.recv_timeout(Duration::from_secs(1)).unwrap().state,
            State::Connecting
        );
        assert_eq!(
            rx.recv_timeout(Duration::from_secs(1)).unwrap().state,
            State::NeedsAuth
        );
        thread::sleep(Duration::from_millis(30));
        assert_eq!(starts.load(Ordering::SeqCst), 1);
        assert!(reaped.load(Ordering::SeqCst));
        supervisor.cancel("host-1");
    }

    #[test]
    fn host_key_failure_is_terminal_without_retry() {
        let starts = Arc::new(AtomicUsize::new(0));
        let runner = Arc::new(FakeRunner {
            alive: Arc::new(AtomicBool::new(false)),
            reaped: Arc::new(AtomicBool::new(false)),
            starts: starts.clone(),
            stderr: "Host key verification failed.".into(),
            fail_spawn: false,
        });
        let supervisor = Supervisor::new(runner, Arc::new(Healthy), fast_policy());
        let (tx, rx) = mpsc::channel();
        supervisor.start(config(), Arc::new(move |event| tx.send(event).unwrap()));
        assert_eq!(
            rx.recv_timeout(Duration::from_secs(1)).unwrap().state,
            State::Connecting
        );
        assert_eq!(
            rx.recv_timeout(Duration::from_secs(1)).unwrap().state,
            State::Failed
        );
        thread::sleep(Duration::from_millis(30));
        assert_eq!(starts.load(Ordering::SeqCst), 1);
        supervisor.cancel("host-1");
    }

    #[test]
    fn cancel_during_start_reservation_prevents_worker_spawn() {
        let starts = Arc::new(AtomicUsize::new(0));
        let runner = Arc::new(FakeRunner {
            alive: Arc::new(AtomicBool::new(true)),
            reaped: Arc::new(AtomicBool::new(false)),
            starts: starts.clone(),
            stderr: String::new(),
            fail_spawn: false,
        });
        let supervisor = Arc::new(Supervisor::new(runner, Arc::new(Healthy), fast_policy()));
        let (reserved_tx, reserved_rx) = mpsc::channel();
        let (continue_tx, continue_rx) = mpsc::channel();
        let starting = supervisor.clone();
        let thread = thread::spawn(move || {
            starting.start_guarded(
                config(),
                Arc::new(|_| {}),
                || true,
                move || {
                    reserved_tx.send(()).unwrap();
                    continue_rx.recv().unwrap();
                },
            );
        });
        reserved_rx.recv_timeout(Duration::from_secs(1)).unwrap();
        supervisor.cancel("host-1");
        continue_tx.send(()).unwrap();
        thread.join().unwrap();
        assert_eq!(starts.load(Ordering::SeqCst), 0);
    }

    #[test]
    fn stale_registry_snapshot_is_rejected_after_reservation() {
        let starts = Arc::new(AtomicUsize::new(0));
        let runner = Arc::new(FakeRunner {
            alive: Arc::new(AtomicBool::new(true)),
            reaped: Arc::new(AtomicBool::new(false)),
            starts: starts.clone(),
            stderr: String::new(),
            fail_spawn: false,
        });
        let supervisor = Supervisor::new(runner, Arc::new(Healthy), fast_policy());

        supervisor.start_if(config(), Arc::new(|_| {}), || false);

        assert_eq!(starts.load(Ordering::SeqCst), 0);
        assert!(!supervisor.entries.lock().unwrap().contains_key("host-1"));
    }

    #[test]
    fn stale_candidate_does_not_replace_a_current_tunnel() {
        let starts = Arc::new(AtomicUsize::new(0));
        let alive = Arc::new(AtomicBool::new(true));
        let runner = Arc::new(FakeRunner {
            alive: alive.clone(),
            reaped: Arc::new(AtomicBool::new(false)),
            starts: starts.clone(),
            stderr: String::new(),
            fail_spawn: false,
        });
        let supervisor = Supervisor::new(runner, Arc::new(Healthy), fast_policy());
        let (tx, rx) = mpsc::channel();
        supervisor.start(config(), Arc::new(move |event| tx.send(event).unwrap()));
        assert_eq!(
            rx.recv_timeout(Duration::from_secs(1)).unwrap().state,
            State::Connecting
        );
        assert_eq!(
            rx.recv_timeout(Duration::from_secs(1)).unwrap().state,
            State::Ready
        );

        supervisor.start_if(config(), Arc::new(|_| {}), || false);

        assert_eq!(starts.load(Ordering::SeqCst), 1);
        assert!(alive.load(Ordering::SeqCst));
        supervisor.cancel("host-1");
    }

    #[test]
    fn shutdown_permanently_rejects_later_starts() {
        let starts = Arc::new(AtomicUsize::new(0));
        let runner = Arc::new(FakeRunner {
            alive: Arc::new(AtomicBool::new(true)),
            reaped: Arc::new(AtomicBool::new(false)),
            starts: starts.clone(),
            stderr: String::new(),
            fail_spawn: false,
        });
        let supervisor = Supervisor::new(runner, Arc::new(Healthy), fast_policy());

        supervisor.shutdown();
        supervisor.start(config(), Arc::new(|_| {}));

        assert_eq!(starts.load(Ordering::SeqCst), 0);
    }

    #[test]
    fn stderr_capture_truncates_multibyte_input_without_panicking() {
        let input = "ошибка".repeat(STDERR_LIMIT);
        let capture = Arc::new(Mutex::new(Vec::new()));
        read_stderr(input.as_bytes(), capture.clone());
        let captured = capture.lock().unwrap();
        assert_eq!(captured.len(), STDERR_LIMIT);
        assert!(String::from_utf8_lossy(&captured).contains("ошибка"));
    }

    #[test]
    fn cancel_interrupts_retry_and_shutdown_does_not_touch_remote_daemon() {
        let remote_daemon_alive = Arc::new(AtomicBool::new(true));
        let runner = Arc::new(FakeRunner {
            alive: Arc::new(AtomicBool::new(false)),
            reaped: Arc::new(AtomicBool::new(false)),
            starts: Arc::new(AtomicUsize::new(0)),
            stderr: String::new(),
            fail_spawn: true,
        });
        let supervisor = Supervisor::new(runner, Arc::new(Healthy), fast_policy());
        let (tx, rx) = mpsc::channel();
        supervisor.start(config(), Arc::new(move |event| tx.send(event).unwrap()));
        assert_eq!(
            rx.recv_timeout(Duration::from_secs(1)).unwrap().state,
            State::Connecting
        );
        assert_eq!(
            rx.recv_timeout(Duration::from_secs(1)).unwrap().state,
            State::Retrying
        );
        let started = Instant::now();
        supervisor.shutdown();
        assert!(started.elapsed() < Duration::from_millis(100));
        assert!(remote_daemon_alive.load(Ordering::SeqCst));
    }
}
