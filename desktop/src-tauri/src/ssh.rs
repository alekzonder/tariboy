use serde::Serialize;
use std::io::{Read, Write};
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{mpsc, Arc, Mutex};
use std::thread;
use std::time::{Duration, Instant};

use portable_pty::{native_pty_system, CommandBuilder, MasterPty, PtySize};

const POLL: Duration = Duration::from_millis(20);
const TRANSCRIPT_LIMIT: usize = 64 * 1024;
const OUTPUT_CHANNEL_CAPACITY: usize = 64;
const OUTPUT_EVENT_LIMIT: usize = 256 * 1024;
pub const OUTPUT_EVENT: &str = "host://provision-output";

#[derive(Clone)]
pub struct Binaries {
    pub ssh: PathBuf,
    #[allow(dead_code)] // Task 9 consumes the resolved system scp path for uploads.
    pub scp: PathBuf,
}

impl Binaries {
    pub fn system() -> Self {
        Self {
            ssh: PathBuf::from("/usr/bin/ssh"),
            scp: PathBuf::from("/usr/bin/scp"),
        }
    }
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
pub struct OutputEvent {
    pub operation_id: String,
    pub host_id: String,
    pub stream: String,
    pub text: String,
    pub prompt: Option<String>,
}

pub type OutputSink = Arc<dyn Fn(OutputEvent) + Send + Sync>;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SshError {
    pub code: String,
    pub message: String,
}

#[derive(Clone, Default)]
pub struct CancelToken(Arc<AtomicBool>);

impl CancelToken {
    pub fn cancel(&self) {
        self.0.store(true, Ordering::SeqCst);
    }

    pub fn is_cancelled(&self) -> bool {
        self.0.load(Ordering::SeqCst)
    }
}

pub struct Operation {
    pub id: String,
    pub host_id: String,
    pub replies: mpsc::Receiver<String>,
    pub cancel: CancelToken,
}

#[derive(Clone, Default)]
pub struct Operations {
    live: Arc<Mutex<LiveOperations>>,
}

#[derive(Clone)]
struct LiveOperation {
    host_id: String,
    replies: mpsc::SyncSender<String>,
    cancel: CancelToken,
}

#[derive(Default)]
struct LiveOperations {
    by_id: std::collections::HashMap<String, LiveOperation>,
    by_host: std::collections::HashMap<String, String>,
}

impl Operations {
    pub fn begin(&self, host_id: &str) -> Result<Operation, String> {
        let id = uuid::Uuid::new_v4().to_string();
        let (sender, replies) = mpsc::sync_channel(1);
        let cancel = CancelToken::default();
        let mut live = self
            .live
            .lock()
            .map_err(|_| "SSH operation registry lock poisoned")?;
        if let Some(operation_id) = live.by_host.get(host_id) {
            return Err(format!(
                "host {host_id} already has active SSH operation {operation_id}"
            ));
        }
        live.by_host.insert(host_id.to_string(), id.clone());
        live.by_id.insert(
            id.clone(),
            LiveOperation {
                host_id: host_id.to_string(),
                replies: sender,
                cancel: cancel.clone(),
            },
        );
        Ok(Operation {
            id,
            host_id: host_id.to_string(),
            replies,
            cancel,
        })
    }

    pub fn reply(&self, operation_id: &str, text: String) -> Result<(), String> {
        if text.len() > 4096 {
            return Err("SSH prompt reply is too long".into());
        }
        if text.contains('\r') || text.contains('\n') {
            return Err("SSH prompt reply must be a single line".into());
        }
        let live = self
            .live
            .lock()
            .map_err(|_| "SSH operation registry lock poisoned")?;
        let live = live
            .by_id
            .get(operation_id)
            .ok_or_else(|| format!("SSH operation {operation_id} is not active"))?;
        if live.cancel.is_cancelled() {
            return Err(format!("SSH operation {operation_id} is not active"));
        }
        live.replies
            .try_send(text)
            .map_err(|_| "SSH prompt already has a queued reply".to_string())
    }

    pub fn finish(&self, operation_id: &str) {
        if let Ok(mut operations) = self.live.lock() {
            if let Some(live) = operations.by_id.remove(operation_id) {
                if operations.by_host.get(&live.host_id).map(String::as_str) == Some(operation_id) {
                    operations.by_host.remove(&live.host_id);
                }
                live.cancel.cancel();
            }
        }
    }

    pub fn cancel_host(&self, host_id: &str) {
        if let Ok(operations) = self.live.lock() {
            if let Some(operation_id) = operations.by_host.get(host_id) {
                if let Some(live) = operations.by_id.get(operation_id) {
                    live.cancel.cancel();
                }
            }
        }
    }

    pub fn cancel_all(&self) {
        if let Ok(operations) = self.live.lock() {
            for operation in operations.by_id.values() {
                operation.cancel.cancel();
            }
        }
    }

    pub fn if_live<F>(&self, operation_id: &str, action: F) -> bool
    where
        F: FnOnce(),
    {
        match self.live.lock() {
            Ok(operations)
                if operations
                    .by_id
                    .get(operation_id)
                    .is_some_and(|operation| !operation.cancel.is_cancelled()) =>
            {
                action();
                true
            }
            _ => false,
        }
    }
}

pub struct Transport {
    bins: Binaries,
}

pub struct ControlCommand<'a> {
    pub alias: &'a str,
    pub socket: &'a Path,
    pub remote_args: &'a [&'a str],
    pub stdin: &'a [u8],
    pub timeout: Duration,
}

pub struct UploadCommand<'a> {
    pub alias: &'a str,
    pub socket: &'a Path,
    pub local_files: &'a [PathBuf],
    pub remote_dir: &'a str,
    pub timeout: Duration,
}

impl Transport {
    pub fn new(bins: Binaries) -> Self {
        Self { bins }
    }

    pub fn system() -> Self {
        Self::new(Binaries::system())
    }

    pub fn resolve(
        &self,
        alias: &str,
        timeout: Duration,
        cancel: &CancelToken,
    ) -> Result<String, SshError> {
        validate_alias(alias)?;
        let output = run_bounded(
            &self.bins.ssh,
            &["-G".into(), alias.into()],
            None,
            timeout,
            cancel,
            None,
        )?;
        if !output.success {
            return Err(classify_failure(&output.combined()));
        }
        Ok(output.stdout)
    }

    pub fn authenticate(
        &self,
        operation: &Operation,
        alias: &str,
        socket: &Path,
        timeout: Duration,
        sink: OutputSink,
    ) -> Result<MasterSession, SshError> {
        validate_alias(alias)?;
        if socket.exists() {
            std::fs::remove_file(socket).map_err(|error| SshError {
                code: "ssh_failed".into(),
                message: format!(
                    "remove stale SSH control socket {}: {error}",
                    socket.display()
                ),
            })?;
        }
        let parent = socket.parent().ok_or_else(|| SshError {
            code: "ssh_failed".into(),
            message: format!("SSH control socket has no parent: {}", socket.display()),
        })?;
        std::fs::create_dir_all(parent).map_err(|error| SshError {
            code: "ssh_failed".into(),
            message: format!("create SSH control directory {}: {error}", parent.display()),
        })?;

        let pty = native_pty_system();
        let pair = pty.openpty(PtySize::default()).map_err(|error| SshError {
            code: "ssh_failed".into(),
            message: format!("open SSH PTY: {error}"),
        })?;
        let mut command = CommandBuilder::new(&self.bins.ssh);
        command.args([
            "-M",
            "-N",
            "-o",
            "ControlMaster=yes",
            "-o",
            "ControlPersist=no",
            "-S",
        ]);
        command.arg(socket);
        command.arg(alias);
        let mut child = pair
            .slave
            .spawn_command(command)
            .map_err(|error| SshError {
                code: "ssh_failed".into(),
                message: format!("start SSH ControlMaster: {error}"),
            })?;
        drop(pair.slave);

        let mut reader = pair.master.try_clone_reader().map_err(|error| {
            terminate_pty(&mut child);
            SshError {
                code: "ssh_failed".into(),
                message: format!("open SSH PTY reader: {error}"),
            }
        })?;
        let mut writer = pair.master.take_writer().map_err(|error| {
            terminate_pty(&mut child);
            SshError {
                code: "ssh_failed".into(),
                message: format!("open SSH PTY writer: {error}"),
            }
        })?;
        let (output_tx, output_rx) = mpsc::sync_channel(OUTPUT_CHANNEL_CAPACITY);
        let reader_thread = thread::spawn(move || {
            let mut buf = [0u8; 4096];
            loop {
                match reader.read(&mut buf) {
                    Ok(0) | Err(_) => break,
                    Ok(n) => {
                        if output_tx
                            .send(String::from_utf8_lossy(&buf[..n]).into_owned())
                            .is_err()
                        {
                            break;
                        }
                    }
                }
            }
        });

        let started = Instant::now();
        let mut transcript = String::new();
        let mut secrets = Vec::new();
        let mut redactor = StreamRedactor::default();
        let mut output_budget = OutputBudget::default();
        let mut waiting_prompt: Option<&'static str> = None;
        let mut prompt_scan = String::new();
        loop {
            if operation.cancel.is_cancelled() {
                terminate_pty(&mut child);
                drop(pair.master);
                drop(output_rx);
                let _ = reader_thread.join();
                return Err(SshError {
                    code: "cancelled".into(),
                    message: "SSH authentication cancelled".into(),
                });
            }
            if started.elapsed() >= timeout {
                terminate_pty(&mut child);
                drop(pair.master);
                drop(output_rx);
                let _ = reader_thread.join();
                let needs_auth = waiting_prompt.is_some();
                return Err(SshError {
                    code: if needs_auth {
                        "needs_auth".into()
                    } else {
                        "connection_timeout".into()
                    },
                    message: if needs_auth {
                        "SSH authentication prompt was not answered".into()
                    } else {
                        "SSH authentication timed out".into()
                    },
                });
            }

            match output_rx.recv_timeout(POLL) {
                Ok(text) => {
                    append_bounded(&mut transcript, &text);
                    let new_prompt = if waiting_prompt.is_none() {
                        append_prompt_scan(&mut prompt_scan, &text);
                        detect_prompt(&prompt_scan)
                    } else {
                        None
                    };
                    if let Some(prompt) = new_prompt {
                        waiting_prompt = Some(prompt);
                        prompt_scan.clear();
                    }
                    let visible = redactor.push(&text, &secrets);
                    let visible = output_budget.take(&visible);
                    if visible.is_some() || new_prompt.is_some() {
                        (sink)(OutputEvent {
                            operation_id: operation.id.clone(),
                            host_id: operation.host_id.clone(),
                            stream: "pty".into(),
                            text: visible.unwrap_or_default(),
                            prompt: new_prompt.map(str::to_string),
                        });
                    }
                    if classify_failure(&transcript).code == "host_key_mismatch" {
                        terminate_pty(&mut child);
                        drop(pair.master);
                        drop(output_rx);
                        let _ = reader_thread.join();
                        return Err(classified_redacted(&transcript, &secrets));
                    }
                }
                Err(mpsc::RecvTimeoutError::Disconnected) => {}
                Err(mpsc::RecvTimeoutError::Timeout) => {}
            }

            if waiting_prompt.is_some() {
                if let Ok(reply) = operation.replies.try_recv() {
                    secrets.push(reply.clone());
                    writer
                        .write_all(reply.as_bytes())
                        .and_then(|()| writer.write_all(b"\n"))
                        .and_then(|()| writer.flush())
                        .map_err(|error| {
                            terminate_pty(&mut child);
                            SshError {
                                code: "ssh_failed".into(),
                                message: format!("write SSH prompt reply: {error}"),
                            }
                        })?;
                    waiting_prompt = None;
                    prompt_scan.clear();
                }
            }

            if socket.exists() {
                let tail = redactor.finish(&secrets);
                if let Some(tail) = output_budget.take(&tail) {
                    (sink)(OutputEvent {
                        operation_id: operation.id.clone(),
                        host_id: operation.host_id.clone(),
                        stream: "pty".into(),
                        text: tail,
                        prompt: None,
                    });
                }
                return Ok(MasterSession {
                    child: Some(child),
                    master: Some(pair.master),
                    writer: Some(writer),
                    reader_thread: Some(reader_thread),
                    socket: socket.to_path_buf(),
                });
            }
            match child.try_wait() {
                Ok(Some(_)) => {
                    drop(pair.master);
                    drop(output_rx);
                    let _ = reader_thread.join();
                    return Err(classified_redacted(&transcript, &secrets));
                }
                Ok(None) => {}
                Err(error) => {
                    terminate_pty(&mut child);
                    drop(pair.master);
                    drop(output_rx);
                    let _ = reader_thread.join();
                    return Err(SshError {
                        code: "ssh_failed".into(),
                        message: format!("wait for SSH ControlMaster: {error}"),
                    });
                }
            }
        }
    }

    pub fn run_control(
        &self,
        operation: &Operation,
        request: ControlCommand<'_>,
        sink: OutputSink,
    ) -> Result<String, SshError> {
        validate_alias(request.alias)?;
        let mut args = vec![
            "-S".to_string(),
            request.socket.display().to_string(),
            request.alias.to_string(),
        ];
        args.extend(request.remote_args.iter().map(|arg| (*arg).to_string()));
        let scoped_sink: OutputSink = {
            let operation_id = operation.id.clone();
            let host_id = operation.host_id.clone();
            Arc::new(move |mut event| {
                event.operation_id.clone_from(&operation_id);
                event.host_id.clone_from(&host_id);
                sink(event);
            })
        };
        let output = run_bounded(
            &self.bins.ssh,
            &args,
            Some(request.stdin),
            request.timeout,
            &operation.cancel,
            Some(scoped_sink),
        )?;
        if !output.success {
            return Err(classify_failure(&output.combined()));
        }
        Ok(output.stdout)
    }

    pub fn upload(
        &self,
        operation: &Operation,
        request: UploadCommand<'_>,
        sink: OutputSink,
    ) -> Result<(), SshError> {
        validate_scp_alias(request.alias)?;
        validate_remote_upload_dir(request.remote_dir)?;
        if request.local_files.is_empty() {
            return Err(SshError {
                code: "invalid_upload".into(),
                message: "SCP upload requires at least one local file".into(),
            });
        }
        for path in request.local_files {
            if !path.is_file() {
                return Err(SshError {
                    code: "invalid_upload".into(),
                    message: format!("local upload file is missing: {}", path.display()),
                });
            }
        }
        let mut args = vec![
            "-o".to_string(),
            format!("ControlPath={}", request.socket.display()),
            "--".to_string(),
        ];
        args.extend(
            request
                .local_files
                .iter()
                .map(|path| path.display().to_string()),
        );
        args.push(format!("{}:{}", request.alias, request.remote_dir));
        let scoped_sink: OutputSink = {
            let operation_id = operation.id.clone();
            let host_id = operation.host_id.clone();
            Arc::new(move |mut event| {
                event.operation_id.clone_from(&operation_id);
                event.host_id.clone_from(&host_id);
                sink(event);
            })
        };
        let output = run_bounded(
            &self.bins.scp,
            &args,
            None,
            request.timeout,
            &operation.cancel,
            Some(scoped_sink),
        )?;
        if !output.success {
            return Err(classify_failure(&output.combined()));
        }
        Ok(())
    }
}

pub struct MasterSession {
    child: Option<Box<dyn portable_pty::Child + Send + Sync>>,
    master: Option<Box<dyn MasterPty + Send>>,
    writer: Option<Box<dyn Write + Send>>,
    reader_thread: Option<thread::JoinHandle<()>>,
    socket: PathBuf,
}

impl MasterSession {
    pub fn socket(&self) -> &Path {
        &self.socket
    }
}

impl Drop for MasterSession {
    fn drop(&mut self) {
        self.writer.take();
        if let Some(child) = self.child.as_mut() {
            terminate_pty(child);
        }
        self.master.take();
        if let Some(reader) = self.reader_thread.take() {
            let _ = reader.join();
        }
        let _ = std::fs::remove_file(&self.socket);
    }
}

struct ProcessOutput {
    success: bool,
    stdout: String,
    stderr: String,
}

impl ProcessOutput {
    fn combined(&self) -> String {
        format!("{}\n{}", self.stdout, self.stderr)
    }
}

fn run_bounded(
    program: &Path,
    args: &[String],
    stdin: Option<&[u8]>,
    timeout: Duration,
    cancel: &CancelToken,
    sink: Option<OutputSink>,
) -> Result<ProcessOutput, SshError> {
    let mut command = Command::new(program);
    command
        .args(args)
        .stdin(if stdin.is_some() {
            Stdio::piped()
        } else {
            Stdio::null()
        })
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    let mut child = command.spawn().map_err(|error| SshError {
        code: "ssh_failed".into(),
        message: format!("start {}: {error}", program.display()),
    })?;
    if let Some(input) = stdin {
        if let Some(mut child_stdin) = child.stdin.take() {
            let input = input.to_vec();
            thread::spawn(move || {
                let _ = child_stdin.write_all(&input);
            });
        }
    }

    let (tx, rx) = mpsc::sync_channel(OUTPUT_CHANNEL_CAPACITY);
    let stdout_thread = spawn_reader(
        "stdout",
        child.stdout.take().ok_or_else(|| SshError {
            code: "ssh_failed".into(),
            message: "SSH stdout pipe unavailable".into(),
        })?,
        tx.clone(),
    );
    let stderr_thread = spawn_reader(
        "stderr",
        child.stderr.take().ok_or_else(|| SshError {
            code: "ssh_failed".into(),
            message: "SSH stderr pipe unavailable".into(),
        })?,
        tx,
    );
    let started = Instant::now();
    let mut stdout = String::new();
    let mut stderr = String::new();
    let mut output_budget = OutputBudget::default();
    let success = loop {
        for _ in 0..OUTPUT_CHANNEL_CAPACITY {
            match rx.try_recv() {
                Ok(item) => record_process_output(
                    item,
                    &mut stdout,
                    &mut stderr,
                    sink.as_ref(),
                    &mut output_budget,
                ),
                Err(_) => break,
            }
        }
        if cancel.is_cancelled() || started.elapsed() >= timeout {
            let _ = child.kill();
            let _ = child.wait();
            drain_process_output(
                &rx,
                &mut stdout,
                &mut stderr,
                sink.as_ref(),
                &mut output_budget,
            );
            let _ = stdout_thread.join();
            let _ = stderr_thread.join();
            return Err(SshError {
                code: if cancel.is_cancelled() {
                    "cancelled".into()
                } else {
                    "connection_timeout".into()
                },
                message: if cancel.is_cancelled() {
                    "SSH command cancelled".into()
                } else {
                    "SSH command timed out".into()
                },
            });
        }
        match child.try_wait() {
            Ok(Some(status)) => break status.success(),
            Ok(None) => thread::sleep(POLL),
            Err(error) => {
                let _ = child.kill();
                let _ = child.wait();
                drain_process_output(
                    &rx,
                    &mut stdout,
                    &mut stderr,
                    sink.as_ref(),
                    &mut output_budget,
                );
                let _ = stdout_thread.join();
                let _ = stderr_thread.join();
                return Err(SshError {
                    code: "ssh_failed".into(),
                    message: format!("wait for SSH command: {error}"),
                });
            }
        }
    };
    drain_process_output(
        &rx,
        &mut stdout,
        &mut stderr,
        sink.as_ref(),
        &mut output_budget,
    );
    let _ = stdout_thread.join();
    let _ = stderr_thread.join();
    Ok(ProcessOutput {
        success,
        stdout,
        stderr,
    })
}

fn spawn_reader<R: Read + Send + 'static>(
    stream: &'static str,
    mut reader: R,
    sender: mpsc::SyncSender<(&'static str, String)>,
) -> thread::JoinHandle<()> {
    thread::spawn(move || {
        let mut buf = [0u8; 4096];
        loop {
            match reader.read(&mut buf) {
                Ok(0) | Err(_) => return,
                Ok(n) => {
                    if sender
                        .send((stream, String::from_utf8_lossy(&buf[..n]).into_owned()))
                        .is_err()
                    {
                        return;
                    }
                }
            }
        }
    })
}

fn record_process_output(
    (stream, text): (&'static str, String),
    stdout: &mut String,
    stderr: &mut String,
    sink: Option<&OutputSink>,
    budget: &mut OutputBudget,
) {
    if stream == "stdout" {
        append_bounded(stdout, &text);
    } else {
        append_bounded(stderr, &text);
    }
    if let (Some(sink), Some(text)) = (sink, budget.take(&text)) {
        sink(OutputEvent {
            operation_id: String::new(),
            host_id: String::new(),
            stream: stream.into(),
            text,
            prompt: None,
        });
    }
}

fn drain_process_output(
    receiver: &mpsc::Receiver<(&'static str, String)>,
    stdout: &mut String,
    stderr: &mut String,
    sink: Option<&OutputSink>,
    budget: &mut OutputBudget,
) {
    while let Ok(item) = receiver.recv() {
        record_process_output(item, stdout, stderr, sink, budget);
    }
}

fn validate_alias(alias: &str) -> Result<(), SshError> {
    if alias.trim().is_empty() || alias.starts_with('-') || alias.contains('\0') {
        return Err(SshError {
            code: "invalid_alias".into(),
            message: "SSH alias must be non-empty and must not start with '-'".into(),
        });
    }
    Ok(())
}

fn validate_scp_alias(alias: &str) -> Result<(), SshError> {
    validate_alias(alias)?;
    if !alias
        .bytes()
        .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'.' | b'_' | b'-' | b'@'))
    {
        return Err(SshError {
            code: "invalid_alias".into(),
            message: "SSH alias contains characters that are unsafe for SCP".into(),
        });
    }
    Ok(())
}

fn validate_remote_upload_dir(path: &str) -> Result<(), SshError> {
    let prefix = "~/.local/lib/tariboy/.stage-";
    let suffix = path
        .strip_prefix(prefix)
        .and_then(|value| value.strip_suffix('/'));
    if suffix.map_or(true, |value| {
        value.is_empty()
            || !value
                .bytes()
                .all(|byte| byte.is_ascii_alphanumeric() || byte == b'-')
    }) {
        return Err(SshError {
            code: "invalid_upload".into(),
            message: "remote upload directory is outside the staging area".into(),
        });
    }
    Ok(())
}

fn detect_prompt(output: &str) -> Option<&'static str> {
    let lower = output.to_ascii_lowercase();
    if lower.contains("are you sure you want to continue connecting") {
        Some("host_key_confirmation")
    } else if lower.contains("password:")
        || lower.contains("passphrase")
        || lower.contains("verification code")
        || lower.contains("one-time password")
        || lower.contains("authentication code")
    {
        Some("authentication")
    } else {
        None
    }
}

fn append_prompt_scan(scan: &mut String, text: &str) {
    const PROMPT_SCAN_LIMIT: usize = 1024;
    scan.push_str(text);
    if scan.len() > PROMPT_SCAN_LIMIT {
        let split = char_boundary_at_or_after(scan, scan.len() - PROMPT_SCAN_LIMIT);
        scan.drain(..split);
    }
}

pub fn classify_failure(output: &str) -> SshError {
    let lower = output.to_ascii_lowercase();
    let (code, fallback) = if lower.contains("remote host identification has changed")
        || lower.contains("host key verification failed")
    {
        ("host_key_mismatch", "SSH host key mismatch")
    } else if lower.contains("connection timed out") || lower.contains("operation timed out") {
        ("connection_timeout", "SSH connection timed out")
    } else if lower.contains("connection refused") {
        ("connection_refused", "SSH connection refused")
    } else if lower.contains("could not resolve hostname")
        || lower.contains("name or service not known")
    {
        ("dns_failed", "SSH hostname resolution failed")
    } else if lower.contains("permission denied") || detect_prompt(output).is_some() {
        ("needs_auth", "SSH authentication is required")
    } else {
        ("ssh_failed", "SSH command failed")
    };
    let detail = output.trim();
    SshError {
        code: code.into(),
        message: if detail.is_empty() {
            fallback.into()
        } else {
            detail.to_string()
        },
    }
}

pub fn redact(text: &str, secrets: &[String]) -> String {
    secrets
        .iter()
        .filter(|secret| !secret.is_empty())
        .fold(text.to_string(), |visible, secret| {
            visible.replace(secret, "[REDACTED]")
        })
}

fn classified_redacted(output: &str, secrets: &[String]) -> SshError {
    let mut error = classify_failure(output);
    error.message = redact(&error.message, secrets);
    error
}

#[derive(Default)]
struct StreamRedactor {
    pending: String,
}

struct OutputBudget {
    remaining: usize,
    truncated: bool,
}

impl Default for OutputBudget {
    fn default() -> Self {
        Self {
            remaining: OUTPUT_EVENT_LIMIT,
            truncated: false,
        }
    }
}

impl OutputBudget {
    fn take(&mut self, text: &str) -> Option<String> {
        if text.is_empty() {
            return None;
        }
        if self.remaining == 0 {
            if self.truncated {
                return None;
            }
            self.truncated = true;
            return Some("\n[output truncated]\n".into());
        }
        let take = char_boundary_at_or_before(text, self.remaining.min(text.len()));
        self.remaining -= take;
        let mut visible = text[..take].to_string();
        if take < text.len() && !self.truncated {
            self.truncated = true;
            self.remaining = 0;
            visible.push_str("\n[output truncated]\n");
        }
        (!visible.is_empty()).then_some(visible)
    }
}

impl StreamRedactor {
    fn push(&mut self, text: &str, secrets: &[String]) -> String {
        if secrets.is_empty() {
            return text.to_string();
        }
        self.pending.push_str(text);
        let visible = redact(&self.pending, secrets);
        let keep = secrets
            .iter()
            .map(String::len)
            .max()
            .unwrap_or(0)
            .saturating_sub(1);
        if visible.len() <= keep {
            return String::new();
        }
        let split = char_boundary_at_or_before(&visible, visible.len() - keep);
        let remainder = visible.split_at(split).1.to_string();
        let ready = visible[..split].to_string();
        self.pending = remainder;
        ready
    }

    fn finish(&mut self, secrets: &[String]) -> String {
        let visible = redact(&self.pending, secrets);
        self.pending.clear();
        visible
    }
}

fn char_boundary_at_or_before(value: &str, mut index: usize) -> usize {
    while index > 0 && !value.is_char_boundary(index) {
        index -= 1;
    }
    index
}

fn char_boundary_at_or_after(value: &str, mut index: usize) -> usize {
    while index < value.len() && !value.is_char_boundary(index) {
        index += 1;
    }
    index
}

fn append_bounded(target: &mut String, text: &str) {
    target.push_str(text);
    if target.len() > TRANSCRIPT_LIMIT {
        let split = target.len() - TRANSCRIPT_LIMIT;
        let boundary = target
            .char_indices()
            .find_map(|(index, _)| (index >= split).then_some(index))
            .unwrap_or(split);
        target.drain(..boundary);
    }
}

fn terminate_pty(child: &mut Box<dyn portable_pty::Child + Send + Sync>) {
    let _ = child.kill();
    let _ = child.wait();
}

pub fn control_socket(dir: &Path, host_id: &str) -> Result<PathBuf, String> {
    use std::os::unix::ffi::OsStrExt;

    const MACOS_SUN_PATH_BYTES: usize = 104;
    const OPENSSH_TEMP_SUFFIX_BUDGET: usize = 24;
    if host_id.is_empty() || host_id.contains('/') || host_id.contains('\\') {
        return Err("invalid host id for SSH control socket".into());
    }
    let path = dir.join(host_id);
    let required = path.as_os_str().as_bytes().len()
        + 1 // "." before OpenSSH's temporary suffix
        + OPENSSH_TEMP_SUFFIX_BUDGET
        + 1; // terminating NUL
    if required > MACOS_SUN_PATH_BYTES {
        return Err(format!(
            "SSH control socket path is too long ({required} bytes, max {MACOS_SUN_PATH_BYTES} including OpenSSH suffix): {}",
            path.display()
        ));
    }
    Ok(path)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::testbin;
    use std::fs;
    use std::thread;
    use std::time::Instant;

    #[test]
    fn resolution_uses_argv_without_shell_or_insecure_policy_flags() {
        let dir = tempfile::tempdir().unwrap();
        let log = dir.path().join("argv.log");
        let ssh = testbin::executable(
            dir.path(),
            "ssh",
            &testbin::argv_logger(&log, "printf 'hostname example\\nproxyjump jump\\n'"),
        );
        let transport = Transport::new(Binaries {
            ssh,
            scp: dir.path().join("scp"),
        });
        let alias = "prod host;$(touch impossible)";

        transport
            .resolve(alias, Duration::from_secs(1), &CancelToken::default())
            .unwrap();

        let calls = testbin::invocations(&log);
        assert_eq!(calls[0], vec!["-G", alias]);
        let flat = calls.concat().join(" ");
        assert!(!flat.contains("StrictHostKeyChecking=no"));
        assert!(!flat.contains("ssh-copy-id"));
        assert!(fs::read_dir(dir.path())
            .unwrap()
            .all(|entry| entry.unwrap().file_name() != "impossible"));
    }

    #[test]
    fn production_paths_use_system_openssh() {
        let bins = Binaries::system();
        assert_eq!(bins.ssh, PathBuf::from("/usr/bin/ssh"));
        assert_eq!(bins.scp, PathBuf::from("/usr/bin/scp"));
    }

    #[test]
    fn upload_uses_system_scp_with_control_socket_and_literal_arguments() {
        let dir = tempfile::tempdir().unwrap();
        let log = dir.path().join("scp.log");
        let scp = testbin::executable(dir.path(), "scp", &testbin::argv_logger(&log, "exit 0"));
        let first = dir.path().join("tariboy");
        let second = dir.path().join("SHA256SUMS");
        fs::write(&first, b"bin").unwrap();
        fs::write(&second, b"sum").unwrap();
        let transport = Transport::new(Binaries {
            ssh: dir.path().join("ssh"),
            scp,
        });
        let operations = Operations::default();
        let operation = operations.begin("host-1").unwrap();
        let socket = dir.path().join("control.sock");

        transport
            .upload(
                &operation,
                UploadCommand {
                    alias: "prod-host",
                    socket: &socket,
                    local_files: &[first.clone(), second.clone()],
                    remote_dir: "~/.local/lib/tariboy/.stage-abc123/",
                    timeout: Duration::from_secs(1),
                },
                Arc::new(|_| {}),
            )
            .unwrap();

        assert_eq!(
            testbin::invocations(&log),
            vec![vec![
                "-o",
                &format!("ControlPath={}", socket.display()),
                "--",
                &first.display().to_string(),
                &second.display().to_string(),
                "prod-host:~/.local/lib/tariboy/.stage-abc123/",
            ]]
        );
    }

    #[test]
    fn upload_rejects_scp_metacharacters_and_non_staging_destinations() {
        let dir = tempfile::tempdir().unwrap();
        let file = dir.path().join("binary");
        fs::write(&file, b"bin").unwrap();
        let transport = Transport::new(Binaries {
            ssh: dir.path().join("ssh"),
            scp: dir.path().join("scp"),
        });
        let operations = Operations::default();
        let operation = operations.begin("host-1").unwrap();
        for (alias, remote_dir) in [
            ("host;touch-pwned", "~/.local/lib/tariboy/.stage-safe/"),
            ("host", "~/.local/bin/"),
            ("host", "~/.local/lib/tariboy/.stage-../"),
        ] {
            assert!(transport
                .upload(
                    &operation,
                    UploadCommand {
                        alias,
                        socket: &dir.path().join("sock"),
                        local_files: std::slice::from_ref(&file),
                        remote_dir,
                        timeout: Duration::from_secs(1),
                    },
                    Arc::new(|_| {}),
                )
                .is_err());
        }
    }

    #[test]
    fn timeout_terminates_and_reaps_resolution_child() {
        let dir = tempfile::tempdir().unwrap();
        let pid = dir.path().join("pid");
        let ssh = testbin::executable(
            dir.path(),
            "ssh",
            &format!("printf '%s' \"$$\" > '{}'\nexec sleep 30", pid.display()),
        );
        let transport = Transport::new(Binaries {
            ssh,
            scp: dir.path().join("scp"),
        });

        let error = transport
            .resolve("slow", Duration::from_millis(100), &CancelToken::default())
            .unwrap_err();

        assert_eq!(error.code, "connection_timeout");
        let child_pid = fs::read_to_string(pid).unwrap();
        let proc_path = PathBuf::from(format!("/proc/{child_pid}"));
        let deadline = Instant::now() + Duration::from_secs(1);
        while proc_path.exists() && Instant::now() < deadline {
            thread::sleep(Duration::from_millis(10));
        }
        assert!(!proc_path.exists(), "timed-out SSH child was not reaped");
    }

    #[test]
    fn cancellation_terminates_resolution_child() {
        let dir = tempfile::tempdir().unwrap();
        let ssh = testbin::executable(dir.path(), "ssh", "exec sleep 30");
        let transport = Transport::new(Binaries {
            ssh,
            scp: dir.path().join("scp"),
        });
        let cancel = CancelToken::default();
        cancel.cancel();

        let error = transport
            .resolve("cancelled", Duration::from_secs(5), &cancel)
            .unwrap_err();

        assert_eq!(error.code, "cancelled");
    }

    #[test]
    fn noisy_child_cannot_starve_timeout_or_grow_output_without_bound() {
        let dir = tempfile::tempdir().unwrap();
        let ssh = testbin::executable(dir.path(), "ssh", "exec yes noisy-output");
        let transport = Transport::new(Binaries {
            ssh,
            scp: dir.path().join("scp"),
        });
        let started = Instant::now();

        let error = transport
            .resolve("noisy", Duration::from_millis(100), &CancelToken::default())
            .unwrap_err();

        assert_eq!(error.code, "connection_timeout");
        assert!(started.elapsed() < Duration::from_secs(2));
    }

    #[test]
    fn operation_reply_is_bounded_and_expires() {
        let operations = Operations::default();
        let operation = operations.begin("host-1").unwrap();
        operations.reply(&operation.id, "first".into()).unwrap();
        assert!(operations.reply(&operation.id, "second".into()).is_err());
        assert_eq!(operation.replies.recv().unwrap(), "first");
        assert!(operations
            .reply(&operation.id, "two\nlines".into())
            .is_err());
        operations.finish(&operation.id);
        assert!(operations.reply(&operation.id, "late".into()).is_err());
        assert!(operation.cancel.is_cancelled());
    }

    #[test]
    fn only_one_operation_per_host_can_be_live() {
        let operations = Operations::default();
        let first = operations.begin("host-1").unwrap();
        assert!(operations.begin("host-1").is_err());
        let second = operations.begin("host-2").unwrap();
        operations.cancel_host("host-1");
        assert!(first.cancel.is_cancelled());
        operations.cancel_all();
        assert!(second.cancel.is_cancelled());
        assert!(operations.begin("host-1").is_err());
        assert!(!operations.if_live(&first.id, || panic!("cancelled operation applied state")));
        operations.finish(&first.id);
        assert!(operations.begin("host-1").is_ok());
    }

    #[test]
    fn control_socket_uses_host_id_not_alias() {
        let dir = tempfile::tempdir().unwrap();
        assert_eq!(
            control_socket(dir.path(), "8dc8-id").unwrap(),
            dir.path().join("8dc8-id")
        );
        assert!(control_socket(dir.path(), "../alias").is_err());
        assert!(
            control_socket(
                Path::new("/a-control-directory-that-is-deliberately-too-long-for-macos"),
                "f94d49b9-33cf-4802-8727-789d9ff61eed"
            )
            .unwrap_err()
            .contains("path is too long")
        );
    }

    #[test]
    fn prompt_reply_is_one_shot_and_never_appears_in_output() {
        let dir = tempfile::tempdir().unwrap();
        let ssh = testbin::executable(
            dir.path(),
            "ssh",
            r#"socket=
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-S" ]; then shift; socket="$1"; fi
  shift || true
done
printf 'Password:'
IFS= read -r reply
printf '\naccepted:top-'
sleep 0.05
printf 'secret\n'
sleep 0.05
printf 'Verification code:'
IFS= read -r second_reply
printf '\nverified\n'
: > "$socket"
exec sleep 30"#,
        );
        let transport = Transport::new(Binaries {
            ssh,
            scp: dir.path().join("scp"),
        });
        let operations = Operations::default();
        let operation = operations.begin("host-1").unwrap();
        let operation_id = operation.id.clone();
        let events = Arc::new(Mutex::new(Vec::<OutputEvent>::new()));
        let seen = events.clone();
        let replies = operations.clone();
        let sink: OutputSink = Arc::new(move |event| {
            if event.prompt.is_some() {
                let _ = replies.reply(&operation_id, "top-secret".into());
            }
            seen.lock().unwrap().push(event);
        });
        let socket = control_socket(dir.path(), "host-1").unwrap();

        let master = transport
            .authenticate(
                &operation,
                "prod alias",
                &socket,
                Duration::from_secs(2),
                sink,
            )
            .unwrap();
        assert_eq!(master.socket(), socket);
        drop(master);
        operations.finish(&operation.id);

        let events = events.lock().unwrap();
        assert!(events
            .iter()
            .any(|event| event.prompt.as_deref() == Some("authentication")));
        assert_eq!(
            events
                .iter()
                .filter(|event| event.prompt.as_deref() == Some("authentication"))
                .count(),
            2
        );
        let visible = events
            .iter()
            .map(|event| event.text.as_str())
            .collect::<String>();
        assert!(!visible.contains("top-secret"));
        assert!(visible.contains("[REDACTED]"));
    }

    #[test]
    fn unanswered_auth_prompt_is_needs_auth_not_network_timeout() {
        let dir = tempfile::tempdir().unwrap();
        let ssh = testbin::executable(dir.path(), "ssh", "printf 'Password:'\nexec sleep 30");
        let transport = Transport::new(Binaries {
            ssh,
            scp: dir.path().join("scp"),
        });
        let operations = Operations::default();
        let operation = operations.begin("host-1").unwrap();
        let sink: OutputSink = Arc::new(|_| {});

        let error = match transport.authenticate(
            &operation,
            "prod",
            &dir.path().join("host-1.sock"),
            Duration::from_millis(100),
            sink,
        ) {
            Ok(_) => panic!("authentication unexpectedly succeeded"),
            Err(error) => error,
        };

        assert_eq!(error.code, "needs_auth");
        operations.finish(&operation.id);
    }

    #[test]
    fn failures_are_classified_without_insecure_bypass() {
        assert_eq!(
            detect_prompt("Are you sure you want to continue connecting (yes/no/[fingerprint])?"),
            Some("host_key_confirmation")
        );
        assert_eq!(detect_prompt("Verification code:"), Some("authentication"));
        assert_eq!(
            classify_failure("@@@ REMOTE HOST IDENTIFICATION HAS CHANGED! @@@").code,
            "host_key_mismatch"
        );
        assert_eq!(
            classify_failure("ssh: connect to host x port 22: Connection refused").code,
            "connection_refused"
        );
        assert_eq!(
            classify_failure("ssh: Could not resolve hostname x").code,
            "dns_failed"
        );
        assert_eq!(
            classify_failure("Permission denied (publickey,password).").code,
            "needs_auth"
        );
        assert_eq!(classify_failure("unfamiliar failure").code, "ssh_failed");
    }

    #[test]
    fn redaction_removes_every_prompt_reply() {
        assert_eq!(
            redact("password=one code=two", &["one".into(), "two".into()]),
            "password=[REDACTED] code=[REDACTED]"
        );
    }

    #[test]
    fn event_output_budget_emits_one_truncation_marker() {
        let mut budget = OutputBudget::default();
        let large = "x".repeat(OUTPUT_EVENT_LIMIT + 100);
        let first = budget.take(&large).unwrap();
        assert!(first.ends_with("[output truncated]\n"));
        assert!(budget.take("more").is_none());
    }
}
