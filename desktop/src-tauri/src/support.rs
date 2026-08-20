//! Privacy-safe diagnostics export.
//!
//! Support bundles are an allowlist, never a copy of application state. The
//! collector deliberately cannot see credentials, prompts, audit records,
//! agent files, SSH configuration, or provisioning replies.

use crate::hosts::HostState;
use serde::Serialize;
use std::collections::{BTreeMap, BTreeSet};
use std::fs::{self, File, OpenOptions};
use std::io::{Cursor, Read, Seek, SeekFrom, Write};
use std::path::{Path, PathBuf};
use std::sync::{Mutex, OnceLock};
use std::time::{SystemTime, UNIX_EPOCH};
use uuid::Uuid;

const POLICY_VERSION: u32 = 1;
const MAX_LOG_LINES: usize = 200;
const MAX_LOG_BYTES: usize = 64 * 1024;
const MAX_LABEL_CHARS: usize = 200;
const MAX_PREREQUISITES: usize = 32;
const MAX_DESKTOP_LOG_BYTES: u64 = 1024 * 1024;
const MAX_DAEMON_ARCHIVE_BYTES: u64 = 160 * 1024 * 1024;
static DESKTOP_LOG_LOCK: OnceLock<Mutex<()>> = OnceLock::new();

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "lowercase")]
pub(crate) enum SelectedTransport {
    Local,
    Ssh,
    Https,
}

#[derive(Debug, Clone, Serialize)]
pub(crate) struct SelectedHost {
    pub id: String,
    pub label: String,
    pub transport: SelectedTransport,
    pub state: String,
    pub phase: String,
    pub platform: String,
    pub arch: String,
    pub prerequisites: Vec<String>,
    pub daemon_version: String,
    pub error_code: Option<String>,
}

pub(crate) enum HostCollection {
    Archive(Vec<u8>),
    Partial(&'static str),
}

pub(crate) struct HostScopedSnapshot {
    pub generated_at: String,
    pub app_version: String,
    pub selected_host: SelectedHost,
    pub desktop_log: PathBuf,
    pub include_agent_data: bool,
    pub collection: HostCollection,
}

#[derive(Serialize)]
struct AppDiagnostic {
    app_version: String,
    platform: &'static str,
    arch: &'static str,
}

#[derive(Serialize)]
struct CollectionError {
    code: &'static str,
}

#[derive(Serialize)]
struct HostScopedManifest {
    redaction_policy_version: u32,
    generated_at: String,
    app_version: String,
    selected_host_id: String,
    selected_host_label: String,
    selected_host_transport: SelectedTransport,
    host_reachable: bool,
    collection_error_code: Option<&'static str>,
    agent_data_requested: bool,
    agent_data_effective: bool,
    iteration_limit: usize,
    files: Vec<String>,
    excluded: Vec<&'static str>,
    max_agent_source_bytes: usize,
    max_daemon_response_bytes: u64,
}

struct BundleFiles(BTreeMap<String, Vec<u8>>);

fn host_state(state: &HostState) -> &'static str {
    match state {
        HostState::Disconnected => "disconnected",
        HostState::Connecting => "connecting",
        HostState::Provisioning => "provisioning",
        HostState::Ready => "ready",
        HostState::Degraded => "degraded",
        HostState::NeedsAuth => "needs_auth",
        HostState::Failed => "failed",
    }
}

fn safe_identifier(value: &str) -> String {
    let value = value.trim();
    if !value.is_empty()
        && value.len() <= 80
        && value
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || b"._-".contains(&byte))
    {
        value.to_string()
    } else if value.is_empty() {
        String::new()
    } else {
        "redacted".into()
    }
}

fn safe_label(value: &str) -> String {
    value
        .chars()
        .filter(|character| !character.is_control())
        .take(MAX_LABEL_CHARS)
        .collect()
}

fn safe_version(value: &str) -> String {
    let value = value.trim();
    let (core, suffix) = value.split_once('-').unwrap_or((value, ""));
    let mut parts = core.split('.');
    let valid_core = (0..3).all(|_| {
        parts
            .next()
            .is_some_and(|part| !part.is_empty() && part.bytes().all(|byte| byte.is_ascii_digit()))
    }) && parts.next().is_none();
    let valid_suffix = suffix.is_empty()
        || (suffix.len() <= 40
            && suffix
                .bytes()
                .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'.' | b'-')));
    if valid_core && valid_suffix && value.len() <= 80 {
        value.to_string()
    } else {
        String::new()
    }
}

fn error_code(message: &str) -> Option<String> {
    let candidate = message.split_once(':')?.0.trim();
    safe_error_code(candidate).map(str::to_string)
}

fn safe_error_code(value: &str) -> Option<&str> {
    const CODES: &[&str] = &[
        "authentication_failed",
        "bind_failed",
        "cancelled",
        "connection_refused",
        "connection_timeout",
        "dns_failed",
        "host_key_mismatch",
        "host_not_ready",
        "host_registry_failed",
        "host_unreachable",
        "invalid_alias",
        "invalid_upload",
        "invalid_version",
        "legacy_active_work",
        "needs_auth",
        "preflight_failed",
        "release_conflict",
        "remote_activate_invalid",
        "remote_stage_invalid",
        "remote_status_invalid",
        "response_too_large",
        "ssh_failed",
        "unsupported_daemon",
        "unsupported_host",
    ];
    CODES.contains(&value).then_some(value)
}

fn safe_lifecycle(value: &str) -> Option<&str> {
    const VALUES: [&str; 28] = [
        "adopted",
        "authenticate",
        "cancelled",
        "connect",
        "connected",
        "connecting",
        "degraded",
        "disconnected",
        "down",
        "exited",
        "failed",
        "health",
        "needs_auth",
        "preflight",
        "provisioning",
        "ready",
        "resolve",
        "retrying",
        "started",
        "starting",
        "status",
        "stopped",
        "tunnel",
        "unavailable",
        "upload",
        "validate",
        "verified",
        "verifying",
    ];
    VALUES.contains(&value).then_some(value)
}

fn safe_log(path: &Path) -> Result<String, String> {
    let mut file = match File::open(path) {
        Ok(file) => file,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(String::new()),
        Err(error) => return Err(format!("read support log {}: {error}", path.display())),
    };
    let len = file.metadata().map_err(|error| error.to_string())?.len();
    let read_limit = (MAX_LOG_BYTES * 4) as u64;
    if len > read_limit {
        file.seek(SeekFrom::Start(len - read_limit))
            .map_err(|error| error.to_string())?;
    }
    let mut raw = Vec::new();
    file.take(read_limit)
        .read_to_end(&mut raw)
        .map_err(|error| format!("read support log {}: {error}", path.display()))?;
    let raw = String::from_utf8_lossy(&raw);

    let mut kept = Vec::new();
    let mut bytes = 0usize;
    for line in raw.lines().rev() {
        let Some(line) = safe_log_line(line) else {
            continue;
        };
        let next = line.len() + usize::from(!kept.is_empty());
        if kept.len() >= MAX_LOG_LINES || bytes + next > MAX_LOG_BYTES {
            break;
        }
        bytes += next;
        kept.push(line);
    }
    kept.reverse();
    Ok(kept.join("\n"))
}

fn safe_log_line(line: &str) -> Option<String> {
    let lower = line.to_ascii_lowercase();
    const SENSITIVE: [&str; 18] = [
        "authorization",
        "bearer",
        "token",
        "secret",
        "password",
        "prompt",
        "transcript",
        "context",
        "content",
        "message=",
        "env ",
        "identityfile",
        "private key",
        "ssh",
        "alias",
        "reply",
        "keychain",
        "credential",
    ];
    if SENSITIVE.iter().any(|needle| lower.contains(needle)) {
        return None;
    }
    const ALLOWED: [&str; 19] = [
        "started",
        "stopped",
        "ready",
        "failed",
        "error_code=",
        "code=",
        "connecting",
        "connected",
        "disconnected",
        "provisioning",
        "tunnel",
        "version",
        "health",
        "adopted",
        "exited",
        "starting",
        "down",
        "needs_auth",
        "degraded",
    ];
    if !ALLOWED.iter().any(|needle| lower.contains(needle)) {
        return None;
    }
    let redacted = redact_home_paths(line);
    let mut safe = Vec::new();
    let mut has_lifecycle = false;
    for (index, token) in redacted.split_whitespace().enumerate() {
        let plain =
            token.trim_matches(|ch: char| !ch.is_ascii_alphanumeric() && ch != '_' && ch != '-');
        let lower = plain.to_ascii_lowercase();
        if index == 0 {
            if let Some(timestamp) = safe_rfc3339_timestamp(token) {
                safe.push(timestamp);
            } else if let Some(value) = token.strip_prefix("timestamp=") {
                if value.len() <= 20 && value.bytes().all(|byte| byte.is_ascii_digit()) {
                    safe.push(format!("timestamp={value}"));
                }
            }
        } else if matches!(lower.as_str(), "daemon" | "desktop" | "tunnel" | "host") {
            safe.push(lower);
        } else if ALLOWED.iter().any(|allowed| *allowed == lower) {
            has_lifecycle = true;
            safe.push(lower);
        } else if let Some((key, value)) = token.split_once('=') {
            if matches!(key, "code" | "error_code") {
                if let Some(value) = safe_error_code(value) {
                    safe.push(format!("{key}={value}"));
                }
            } else if matches!(key, "state" | "phase") {
                if let Some(value) = safe_lifecycle(value) {
                    safe.push(format!("{key}={value}"));
                }
            } else if key == "timestamp"
                && value.len() <= 20
                && value.bytes().all(|byte| byte.is_ascii_digit())
            {
                safe.push(format!("timestamp={value}"));
            } else if key == "scope" && !safe_identifier(value).is_empty() {
                safe.push(format!("scope={}", safe_identifier(value)));
            } else if matches!(key, "base" | "path") && value.starts_with("$HOME/") {
                safe.push(format!("{key}={}", safe_home_path(value)));
            }
        }
    }
    has_lifecycle.then(|| safe.join(" "))
}

fn safe_rfc3339_timestamp(value: &str) -> Option<String> {
    let bytes = value.as_bytes();
    if !(20..=35).contains(&bytes.len())
        || bytes.get(4) != Some(&b'-')
        || bytes.get(7) != Some(&b'-')
        || bytes.get(10) != Some(&b'T')
        || bytes.get(13) != Some(&b':')
        || bytes.get(16) != Some(&b':')
    {
        return None;
    }
    for index in [0, 1, 2, 3, 5, 6, 8, 9, 11, 12, 14, 15, 17, 18] {
        if !bytes[index].is_ascii_digit() {
            return None;
        }
    }
    let number = |range: std::ops::Range<usize>| {
        value[range]
            .parse::<u32>()
            .expect("timestamp digits checked above")
    };
    let year = number(0..4);
    let month = number(5..7);
    let day = number(8..10);
    let hour = number(11..13);
    let minute = number(14..16);
    let second = number(17..19);
    let leap = year % 4 == 0 && (year % 100 != 0 || year % 400 == 0);
    let max_day = match month {
        1 | 3 | 5 | 7 | 8 | 10 | 12 => 31,
        4 | 6 | 9 | 11 => 30,
        2 if leap => 29,
        2 => 28,
        _ => return None,
    };
    if day == 0 || day > max_day || hour > 23 || minute > 59 || second > 60 {
        return None;
    }
    let suffix = &value[19..];
    let valid_zone = |zone: &str| {
        let bytes = zone.as_bytes();
        bytes.len() == 6
            && matches!(bytes[0], b'+' | b'-')
            && bytes[1].is_ascii_digit()
            && bytes[2].is_ascii_digit()
            && bytes[3] == b':'
            && bytes[4].is_ascii_digit()
            && bytes[5].is_ascii_digit()
            && zone[1..3].parse::<u32>().is_ok_and(|hour| hour <= 23)
            && zone[4..6].parse::<u32>().is_ok_and(|minute| minute <= 59)
    };
    let valid = suffix == "Z"
        || valid_zone(suffix)
        || suffix.strip_prefix('.').is_some_and(|fraction| {
            if let Some(digits) = fraction.strip_suffix('Z') {
                (1..=9).contains(&digits.len()) && digits.bytes().all(|byte| byte.is_ascii_digit())
            } else if fraction.len() >= 7 {
                let split = fraction.len() - 6;
                let (digits, zone) = fraction.split_at(split);
                (1..=9).contains(&digits.len())
                    && digits.bytes().all(|byte| byte.is_ascii_digit())
                    && valid_zone(zone)
            } else {
                false
            }
        });
    valid
        .then(|| format!("{year:04}-{month:02}-{day:02}T{hour:02}:{minute:02}:{second:02}{suffix}"))
}

fn safe_home_path(value: &str) -> &'static str {
    if value.starts_with("$HOME/.tariboyd") {
        "$HOME/.tariboyd"
    } else if value.starts_with("$HOME/.tariboy") {
        "$HOME/.tariboy"
    } else {
        "$HOME/<redacted>"
    }
}

fn redact_home_paths(line: &str) -> String {
    let mut output = line.to_string();
    for prefix in ["/Users/", "/home/"] {
        while let Some(start) = output.find(prefix) {
            let user_start = start + prefix.len();
            let Some(rest_offset) = output[user_start..].find('/') else {
                output.replace_range(start.., "$HOME");
                break;
            };
            let rest = user_start + rest_offset;
            output.replace_range(start..rest, "$HOME");
        }
    }
    output
}

pub(crate) fn diagnostic_error_code(message: &str) -> Option<String> {
    error_code(message)
}

pub(crate) fn diagnostic_host_state(state: &HostState) -> &'static str {
    host_state(state)
}

/// Append one structured desktop lifecycle event. Arbitrary messages never
/// enter this log: callers provide enum-derived state and allowlisted codes.
pub(crate) fn append_desktop_event(
    path: &Path,
    component: &str,
    scope: &str,
    state: &str,
    code: Option<&str>,
) {
    let component = match component {
        "daemon" => "daemon",
        "host" => "host",
        _ => return,
    };
    let Some(state) = safe_lifecycle(state) else {
        return;
    };
    let scope = safe_identifier(scope);
    if scope.is_empty() {
        return;
    }
    let code = code.and_then(safe_error_code);
    let lock = DESKTOP_LOG_LOCK.get_or_init(|| Mutex::new(()));
    let _guard = lock.lock().unwrap();
    if fs::metadata(path).is_ok_and(|metadata| metadata.len() > MAX_DESKTOP_LOG_BYTES) {
        let _ = fs::write(path, []);
    }
    let timestamp = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|duration| duration.as_secs())
        .unwrap_or_default();
    let mut line =
        format!("timestamp={timestamp} desktop {component} {state} scope={scope} state={state}");
    if let Some(code) = code {
        line.push_str(&format!(" code={code}"));
    }
    line.push('\n');
    if let Some(parent) = path.parent() {
        let _ = fs::create_dir_all(parent);
    }
    if let Ok(mut file) = OpenOptions::new().create(true).append(true).open(path) {
        let _ = file.write_all(line.as_bytes());
    }
}

pub(crate) fn suggested_name(host_id: &str, label: &str, unix_seconds: u64) -> String {
    let mut slug = host_slug(label);
    if slug.is_empty() {
        slug = host_slug(host_id);
    }
    if slug.is_empty() {
        slug = "host".into();
    }
    format!(
        "tariboy-support-{slug}-{}.zip",
        utc_timestamp(unix_seconds)
    )
}

fn host_slug(value: &str) -> String {
    let mut output = String::new();
    let mut separator = false;
    for character in value.chars() {
        if character.is_ascii_alphanumeric() {
            if output.len() < 48 {
                output.push(character.to_ascii_lowercase());
            }
            separator = false;
        } else if !output.is_empty() && !separator && output.len() < 48 {
            output.push('-');
            separator = true;
        }
    }
    output.truncate(48);
    output.trim_matches('-').to_string()
}

fn utc_timestamp(unix_seconds: u64) -> String {
    let days = (unix_seconds / 86_400) as i64;
    let seconds = unix_seconds % 86_400;
    let hour = seconds / 3_600;
    let minute = (seconds % 3_600) / 60;
    let second = seconds % 60;
    let (year, month, day) = civil_from_days(days);
    format!("{year:04}{month:02}{day:02}-{hour:02}{minute:02}{second:02}")
}

// Howard Hinnant's civil-from-days algorithm. `days` is counted from the Unix
// epoch and the result is Gregorian UTC, including leap years.
fn civil_from_days(days: i64) -> (i64, u64, u64) {
    let z = days + 719_468;
    let era = if z >= 0 { z } else { z - 146_096 } / 146_097;
    let doe = z - era * 146_097;
    let yoe = (doe - doe / 1_460 + doe / 36_524 - doe / 146_096) / 365;
    let mut year = yoe + era * 400;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let day = doy - (153 * mp + 2) / 5 + 1;
    let month = mp + if mp < 10 { 3 } else { -9 };
    year += i64::from(month <= 2);
    (year, month as u64, day as u64)
}

fn safe_scoped_log(path: &Path, selected: &SelectedHost) -> Result<Vec<u8>, String> {
    let log = safe_log(path)?;
    let scope = format!("scope={}", safe_identifier(&selected.id));
    let component = match selected.transport {
        SelectedTransport::Local => " daemon ",
        SelectedTransport::Ssh | SelectedTransport::Https => " host ",
    };
    let mut output = String::new();
    for line in log.lines() {
        if line.contains(component) && line.split_whitespace().any(|field| field == scope) {
            output.push_str(line);
            output.push('\n');
        }
    }
    Ok(output.into_bytes())
}

fn sanitized_selected_host(host: &SelectedHost) -> SelectedHost {
    SelectedHost {
        id: safe_identifier(&host.id),
        label: safe_label(&host.label),
        transport: host.transport,
        state: safe_identifier(&host.state),
        phase: safe_identifier(&host.phase),
        platform: safe_identifier(&host.platform),
        arch: safe_identifier(&host.arch),
        prerequisites: host
            .prerequisites
            .iter()
            .take(MAX_PREREQUISITES)
            .map(|value| safe_identifier(value))
            .filter(|value| !value.is_empty())
            .collect(),
        daemon_version: safe_version(&host.daemon_version),
        error_code: host
            .error_code
            .as_deref()
            .and_then(safe_error_code)
            .map(str::to_string),
    }
}

fn merge_daemon_archive(
    files: &mut BTreeMap<String, Vec<u8>>,
    archive: &[u8],
) -> Result<(), String> {
    if archive.len() as u64 > MAX_DAEMON_ARCHIVE_BYTES {
        return Err("daemon support archive exceeds response limit".into());
    }
    let mut input = zip::ZipArchive::new(Cursor::new(archive))
        .map_err(|_| "daemon support response is not a valid ZIP".to_string())?;
    let mut seen = BTreeSet::new();
    let mut uncompressed = 0u64;
    for index in 0..input.len() {
        let mut file = input
            .by_index(index)
            .map_err(|_| "read daemon support archive entry".to_string())?;
        let name = file.name().to_string();
        let symlink = file
            .unix_mode()
            .is_some_and(|mode| mode & 0o170000 == 0o120000);
        if name.is_empty()
            || file.is_dir()
            || file.encrypted()
            || file.enclosed_name().is_none()
            || name.starts_with('/')
            || name.contains('\\')
            || symlink
            || !seen.insert(name.clone())
        {
            return Err("daemon support archive contains an unsafe entry".into());
        }
        uncompressed = uncompressed
            .checked_add(file.size())
            .ok_or_else(|| "daemon support archive exceeds response limit".to_string())?;
        if uncompressed > MAX_DAEMON_ARCHIVE_BYTES {
            return Err("daemon support archive exceeds response limit".into());
        }
        let mut body = Vec::with_capacity(file.size().min(1024 * 1024) as usize);
        file.read_to_end(&mut body)
            .map_err(|_| "read daemon support archive entry".to_string())?;
        let destination = format!("host/{name}");
        if files.insert(destination, body).is_some() {
            return Err("daemon support archive contains a duplicate entry".into());
        }
    }
    Ok(())
}

fn collect_host_scoped(snapshot: &HostScopedSnapshot) -> Result<BundleFiles, String> {
    let selected = sanitized_selected_host(&snapshot.selected_host);
    if selected.id.is_empty() {
        return Err("selected support host has an invalid id".into());
    }
    let mut files = BTreeMap::new();
    files.insert(
        "desktop/app.json".into(),
        serde_json::to_vec_pretty(&AppDiagnostic {
            app_version: safe_version(&snapshot.app_version),
            platform: std::env::consts::OS,
            arch: std::env::consts::ARCH,
        })
        .map_err(|error| error.to_string())?,
    );
    files.insert(
        "desktop/selected-host.json".into(),
        serde_json::to_vec_pretty(&selected).map_err(|error| error.to_string())?,
    );
    files.insert(
        "desktop/lifecycle.log".into(),
        safe_scoped_log(&snapshot.desktop_log, &selected)?,
    );

    let (reachable, error_code, effective) = match &snapshot.collection {
        HostCollection::Archive(archive) => {
            merge_daemon_archive(&mut files, archive)?;
            (true, None, snapshot.include_agent_data)
        }
        HostCollection::Partial(code) => {
            let code = safe_error_code(code).unwrap_or("host_unreachable");
            files.insert(
                "host/collection-error.json".into(),
                serde_json::to_vec_pretty(&CollectionError { code })
                    .map_err(|error| error.to_string())?,
            );
            (false, Some(code), false)
        }
    };
    let mut names = vec!["manifest.json".to_string()];
    names.extend(files.keys().cloned());
    let manifest = HostScopedManifest {
        redaction_policy_version: POLICY_VERSION + 1,
        generated_at: safe_label(&snapshot.generated_at),
        app_version: safe_version(&snapshot.app_version),
        selected_host_id: selected.id,
        selected_host_label: selected.label,
        selected_host_transport: selected.transport,
        host_reachable: reachable,
        collection_error_code: error_code,
        agent_data_requested: snapshot.include_agent_data,
        agent_data_effective: effective,
        iteration_limit: 10,
        files: names,
        excluded: vec![
            "PROMPT.md and all prompts",
            "model and proxy transcripts",
            "audit.jsonl",
            "credentials, secrets and environment values",
            "CONTEXT.md, workdirs and user files",
            "SSH aliases and configuration",
            "image, plugin and provisioning data",
        ],
        max_agent_source_bytes: 128 * 1024 * 1024,
        max_daemon_response_bytes: MAX_DAEMON_ARCHIVE_BYTES,
    };
    files.insert(
        "manifest.json".into(),
        serde_json::to_vec_pretty(&manifest).map_err(|error| error.to_string())?,
    );
    Ok(BundleFiles(files))
}

pub(crate) fn export_host_scoped(
    snapshot: &HostScopedSnapshot,
    destination: &Path,
) -> Result<PathBuf, String> {
    let files = collect_host_scoped(snapshot)?;
    write_bundle_files(files, destination)
}

fn write_bundle_files(files: BundleFiles, destination: &Path) -> Result<PathBuf, String> {
    let parent = destination
        .parent()
        .filter(|path| !path.as_os_str().is_empty())
        .unwrap_or_else(|| Path::new("."));
    fs::create_dir_all(parent).map_err(|error| {
        format!(
            "create support bundle directory {}: {error}",
            parent.display()
        )
    })?;
    let name = destination
        .file_name()
        .and_then(|name| name.to_str())
        .unwrap_or("support.zip");
    let temporary = parent.join(format!(".{name}.{}.tmp", Uuid::new_v4()));
    let result = (|| -> Result<(), String> {
        let mut options = OpenOptions::new();
        options.write(true).create_new(true);
        #[cfg(unix)]
        {
            use std::os::unix::fs::OpenOptionsExt;
            options.mode(0o600);
        }
        let file = options
            .open(&temporary)
            .map_err(|error| format!("create temporary support bundle: {error}"))?;
        let mut archive = zip::ZipWriter::new(file);
        let options = zip::write::SimpleFileOptions::default()
            .compression_method(zip::CompressionMethod::Deflated)
            .unix_permissions(0o600);
        for (path, body) in files.0 {
            archive
                .start_file(path, options)
                .map_err(|error| format!("write support bundle: {error}"))?;
            archive
                .write_all(&body)
                .map_err(|error| format!("write support bundle: {error}"))?;
        }
        let file = archive
            .finish()
            .map_err(|error| format!("finish support bundle: {error}"))?;
        file.sync_all()
            .map_err(|error| format!("sync support bundle: {error}"))?;
        fs::rename(&temporary, destination)
            .map_err(|error| format!("publish support bundle: {error}"))?;
        Ok(())
    })();
    if result.is_err() {
        let _ = fs::remove_file(&temporary);
    }
    result.map(|()| destination.to_path_buf())
}

#[cfg(any())] // retired v1 exporter; kept out of the compiled product
pub fn export(snapshot: &Snapshot, destination: &Path) -> Result<PathBuf, String> {
    write_bundle_files(collect(snapshot)?, destination)
}

#[cfg(any())] // retired v1 tests
mod tests {
    use super::*;
    use crate::hosts::{HostKind, HostState};
    use crate::state::DaemonView;
    use std::fs;

    fn host() -> HostView {
        HostView {
            id: "host-1".into(),
            label: "Build box".into(),
            kind: HostKind::Ssh,
            ssh_alias: "alice-prod-secret-alias".into(),
            remote_install_dir: "/home/alice/private/install".into(),
            remote_port: 9990,
            https_base_url: "https://token.example/private".into(),
            last_daemon_version: "0.11.0".into(),
            state: HostState::Ready,
            base_url: "http://127.0.0.1:18444".into(),
            local_port: 18444,
            phase: "tunnel".into(),
            platform: "linux".into(),
            arch: "x86_64".into(),
            prerequisites: vec!["bash".into(), "tmux".into()],
            message: "needs_auth: password hunter2".into(),
        }
    }

    #[test]
    fn bundle_is_an_allowlist_and_redacts_logs() {
        let dir = tempfile::tempdir().unwrap();
        let daemon_log = dir.path().join("daemon.log");
        let desktop_log = dir.path().join("desktop.log");
        fs::write(
            &daemon_log,
            concat!(
                "2026-07-29T10:00:00Z daemon started version=0.11.0\n",
                "2026-07-29T10:00:01Z Authorization: Bearer top-secret-token\n",
                "2026-07-29T10:00:02Z prompt=implement private merger\n",
                "2026-07-29T10:00:03Z transcript user said secret words\n",
                "2026-07-29T10:00:04Z env AWS_SECRET_ACCESS_KEY=abc\n",
                "2026-07-29T10:00:05Z daemon ready base=/home/alice/.tariboy\n",
                "2026-07-29T10:00:06Z daemon failed endpoint=private.customer.example\n",
                "2026-07-29T10:00:07Z daemon failed api_key=sk-live-bypass\n",
                "2026-07-29T10:00:08Z daemon failed access_key=access-bypass\n",
                "2026-07-29T10:00:09Z daemon failed url=https://private.invalid/?key=query-bypass\n",
                "2026-07-29T10:00:10Z daemon failed err=customer-name-is-private\n",
                "2026-07-29T10:00:11Z daemon failed path=/srv/private/customer/file\n",
                "2026-07-29T10:00:00+api_key=timestamp-bypass daemon failed\n",
                "2026-07-29T10:00:12Z daemon failed state=ghp_1234567890abcdefghijklmnopqrstuv\n",
                "2026-07-29T10:00:13Z daemon failed phase=private_customer_rollout\n",
                "2026-07-29T10:00:14Z daemon failed version=ghp_abcdefghijklmnopqrstuvwxyz123456\n",
                "2026-99-99T99:99:99Z daemon failed state=failed\n",
            ),
        )
        .unwrap();
        fs::write(
            &desktop_log,
            concat!(
                "2026-07-29T10:01:00Z tunnel connected host=host-1\n",
                "Host prod\n  IdentityFile ~/.ssh/id_ed25519\n",
                "2026-07-29T10:01:01Z provisioning reply=654321\n",
                "2026-07-29T10:01:02Z code=host_unreachable failed\n",
            ),
        )
        .unwrap();
        let snapshot = Snapshot {
            generated_at: "2026-07-29T10:02:00Z".into(),
            app_version: "0.11.0".into(),
            daemon: DaemonView::failed(
                "bind_failed: /Users/alice/private socket token=hidden",
                "0.11.0",
            ),
            hosts: vec![host()],
            daemon_log,
            desktop_log,
        };

        let files = collect(&snapshot).unwrap().0;
        let all = files
            .iter()
            .map(|(name, body)| format!("{name}\n{}", String::from_utf8_lossy(body)))
            .collect::<Vec<_>>()
            .join("\n");

        assert!(all.contains("0.11.0"));
        assert!(all.contains("host-1"));
        assert!(all.contains("Build box"));
        assert!(all.contains("linux"));
        assert!(all.contains("x86_64"));
        assert!(all.contains("daemon started"));
        assert!(all.contains("tunnel connected"));
        assert!(all.contains("host_unreachable"));
        assert!(all.contains("$HOME/.tariboy"));
        for forbidden in [
            "top-secret-token",
            "private merger",
            "secret words",
            "AWS_SECRET_ACCESS_KEY",
            "alice-prod-secret-alias",
            "IdentityFile",
            "id_ed25519",
            "654321",
            "hunter2",
            "/home/alice",
            "/Users/alice",
            "https://token.example",
            "private.customer.example",
            "sk-live-bypass",
            "access-bypass",
            "query-bypass",
            "customer-name-is-private",
            "/srv/private/customer/file",
            "timestamp-bypass",
            "ghp_1234567890abcdefghijklmnopqrstuv",
            "private_customer_rollout",
            "ghp_abcdefghijklmnopqrstuvwxyz123456",
            "2026-99-99T99:99:99Z",
        ] {
            assert!(!all.contains(forbidden), "bundle leaked {forbidden}: {all}");
        }
        let manifest = String::from_utf8(files["manifest.json"].clone()).unwrap();
        assert!(manifest.contains(&format!("\"redaction_policy_version\": {POLICY_VERSION}")));
    }

    #[test]
    fn log_output_is_bounded_by_lines_and_bytes() {
        let dir = tempfile::tempdir().unwrap();
        let daemon_log = dir.path().join("daemon.log");
        let desktop_log = dir.path().join("desktop.log");
        let lines = (0..5_000)
            .map(|index| {
                format!(
                    "2026-07-29T10:00:00Z daemon ready {index} {}\n",
                    "x".repeat(500)
                )
            })
            .collect::<String>();
        fs::write(&daemon_log, lines).unwrap();
        fs::write(&desktop_log, "").unwrap();
        let files = collect(&Snapshot {
            generated_at: "2026-07-29T10:02:00Z".into(),
            app_version: "v".into(),
            daemon: DaemonView::starting("v"),
            hosts: vec![],
            daemon_log,
            desktop_log,
        })
        .unwrap()
        .0;
        let log = &files["logs/daemon.log"];
        assert!(log.len() <= MAX_LOG_BYTES);
        assert!(String::from_utf8_lossy(log).lines().count() <= MAX_LOG_LINES);
    }

    #[test]
    fn export_is_a_deterministic_atomic_zip() {
        let dir = tempfile::tempdir().unwrap();
        let daemon_log = dir.path().join("daemon.log");
        let desktop_log = dir.path().join("desktop.log");
        fs::write(&daemon_log, "2026-07-29T10:00:00Z daemon ready\n").unwrap();
        fs::write(&desktop_log, "2026-07-29T10:00:01Z tunnel connected\n").unwrap();
        let snapshot = Snapshot {
            generated_at: "2026-07-29T10:02:00Z".into(),
            app_version: "0.11.0".into(),
            daemon: DaemonView::starting("0.11.0"),
            hosts: vec![host()],
            daemon_log,
            desktop_log,
        };
        let first = dir.path().join("first.zip");
        let second = dir.path().join("second.zip");
        export(&snapshot, &first).unwrap();
        export(&snapshot, &second).unwrap();
        assert_eq!(fs::read(&first).unwrap(), fs::read(&second).unwrap());
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            assert_eq!(
                fs::metadata(&first).unwrap().permissions().mode() & 0o777,
                0o600
            );
            assert_eq!(
                fs::metadata(&second).unwrap().permissions().mode() & 0o777,
                0o600
            );
        }
        assert!(!fs::read_dir(dir.path()).unwrap().any(|entry| entry
            .unwrap()
            .file_name()
            .to_string_lossy()
            .ends_with(".tmp")));

        let mut archive = zip::ZipArchive::new(File::open(first).unwrap()).unwrap();
        let mut manifest = String::new();
        archive
            .by_name("manifest.json")
            .unwrap()
            .read_to_string(&mut manifest)
            .unwrap();
        assert!(manifest.contains("\"logs/daemon.log\""));
        assert!(archive.by_name("diagnostics/hosts.json").is_ok());
    }

    #[test]
    fn failed_publish_leaves_no_partial_archive() {
        let dir = tempfile::tempdir().unwrap();
        let log = dir.path().join("log");
        fs::write(&log, "daemon ready").unwrap();
        let destination = dir.path().join("existing-directory");
        fs::create_dir(&destination).unwrap();
        let result = export(
            &Snapshot {
                generated_at: "now".into(),
                app_version: "v".into(),
                daemon: DaemonView::starting("v"),
                hosts: vec![],
                daemon_log: log.clone(),
                desktop_log: log,
            },
            &destination,
        );
        assert!(result.is_err());
        assert!(destination.is_dir());
        assert!(!fs::read_dir(dir.path()).unwrap().any(|entry| entry
            .unwrap()
            .file_name()
            .to_string_lossy()
            .ends_with(".tmp")));
    }

    #[test]
    fn host_metadata_is_bounded_and_reports_truncation() {
        let dir = tempfile::tempdir().unwrap();
        let log = dir.path().join("log");
        fs::write(&log, "").unwrap();
        let mut oversized = host();
        oversized.label = "x".repeat(10_000);
        oversized.prerequisites = (0..100).map(|index| format!("tool-{index}")).collect();
        let files = collect(&Snapshot {
            generated_at: "now".into(),
            app_version: "v".into(),
            daemon: DaemonView::starting("v"),
            hosts: vec![oversized; 150],
            daemon_log: log.clone(),
            desktop_log: log,
        })
        .unwrap()
        .0;
        let hosts: serde_json::Value =
            serde_json::from_slice(&files["diagnostics/hosts.json"]).unwrap();
        assert_eq!(hosts.as_array().unwrap().len(), MAX_HOSTS);
        assert!(hosts[0]["label"].as_str().unwrap().chars().count() <= MAX_LABEL_CHARS);
        assert!(hosts[0]["prerequisites"].as_object().unwrap().len() <= MAX_PREREQUISITES);
        let manifest = String::from_utf8(files["manifest.json"].clone()).unwrap();
        assert!(manifest.contains("\"hosts_total\": 150"));
        assert!(manifest.contains("\"hosts_included\": 100"));
        assert!(files.values().map(Vec::len).sum::<usize>() < 256 * 1024);
    }

    #[test]
    fn desktop_logger_writes_only_structured_allowlisted_fields() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("desktop.log");
        append_desktop_event(&path, "host", "host-1", "failed", Some("needs_auth"));
        append_desktop_event(&path, "host", "host-1", "failed", Some("sk_live_secret"));
        append_desktop_event(
            &path,
            "host",
            "host-1",
            "ghp_1234567890abcdefghijklmnopqrstuv",
            None,
        );
        append_desktop_event(&path, "unknown", "host-1", "failed", Some("needs_auth"));
        let raw = fs::read_to_string(&path).unwrap();
        assert!(raw.contains("desktop host failed scope=host-1 state=failed code=needs_auth"));
        assert!(!raw.contains("sk_live_secret"));
        assert_eq!(raw.lines().count(), 2);
        let safe = safe_log(&path).unwrap();
        assert!(safe.contains("desktop host failed scope=host-1 state=failed code=needs_auth"));
        assert!(!safe.contains("sk_live_secret"));
    }
}

#[cfg(test)]
mod host_scoped_tests {
    use super::*;
    use std::io::Cursor;

    #[test]
    fn linear_update_error_code_is_preserved_for_diagnostics() {
        assert_eq!(
            safe_error_code("legacy_active_work"),
            Some("legacy_active_work")
        );
        assert_eq!(safe_error_code("active_work_check_failed"), None);
    }

    fn daemon_archive(entries: &[(&str, &str)]) -> Vec<u8> {
        let mut output = Cursor::new(Vec::new());
        {
            let mut archive = zip::ZipWriter::new(&mut output);
            let options = zip::write::SimpleFileOptions::default()
                .compression_method(zip::CompressionMethod::Deflated)
                .unix_permissions(0o600);
            for (name, body) in entries {
                archive.start_file(*name, options).unwrap();
                archive.write_all(body.as_bytes()).unwrap();
            }
            archive.finish().unwrap();
        }
        output.into_inner()
    }

    fn selected(id: &str, label: &str, transport: SelectedTransport) -> SelectedHost {
        SelectedHost {
            id: id.into(),
            label: label.into(),
            transport,
            state: "ready".into(),
            phase: "connect".into(),
            platform: "linux".into(),
            arch: "x86_64".into(),
            prerequisites: vec!["tmux".into()],
            daemon_version: "0.11.0".into(),
            error_code: None,
        }
    }

    #[test]
    fn suggested_name_contains_only_safe_host_slug_and_utc_timestamp() {
        assert_eq!(
            suggested_name("host-1", "Build Box / Primary", 1_785_321_486),
            "tariboy-support-build-box-primary-20260729-103806.zip"
        );
        assert_eq!(
            suggested_name("local", "Local", 1_785_321_486),
            "tariboy-support-local-20260729-103806.zip"
        );
        let fallback = suggested_name("f94d49b9-33cf-4802-8727", "\n💣", 1_785_321_486);
        assert!(fallback.starts_with("tariboy-support-f94d49b9-33cf-4802-8727-"));
        assert!(!fallback.contains(".."));
        assert!(!fallback.contains('/'));
        assert!(!fallback.contains('\\'));
    }

    #[test]
    fn export_contains_only_selected_host_and_scoped_lifecycle_rows() {
        let dir = tempfile::tempdir().unwrap();
        let desktop_log = dir.path().join("desktop.log");
        fs::write(
            &desktop_log,
            concat!(
                "timestamp=1 desktop host ready scope=host-1 state=ready\n",
                "timestamp=2 desktop host failed scope=host-2 state=failed code=host_unreachable\n",
                "timestamp=3 desktop daemon ready scope=local state=ready\n",
                "timestamp=4 desktop host failed state=failed code=needs_auth\n",
            ),
        )
        .unwrap();
        let destination = dir.path().join("selected.zip");
        export_host_scoped(
            &HostScopedSnapshot {
                generated_at: "2026-07-29T10:02:00Z".into(),
                app_version: "0.11.0".into(),
                selected_host: selected("host-1", "Build box", SelectedTransport::Ssh),
                desktop_log,
                include_agent_data: false,
                collection: HostCollection::Archive(daemon_archive(&[
                    ("diagnostics.json", r#"{"daemon_version":"0.11.0"}"#),
                    ("logs/tariboyd.log", "daemon ready"),
                ])),
            },
            &destination,
        )
        .unwrap();

        let mut archive = zip::ZipArchive::new(File::open(destination).unwrap()).unwrap();
        let mut all = String::new();
        for index in 0..archive.len() {
            let mut file = archive.by_index(index).unwrap();
            all.push_str(file.name());
            all.push('\n');
            file.read_to_string(&mut all).unwrap();
            all.push('\n');
        }
        assert!(all.contains("host-1"));
        assert!(all.contains("Build box"));
        assert!(all.contains("scope=host-1"));
        assert!(all.contains("host/diagnostics.json"));
        for forbidden in ["host-2", "scope=local", "scope=host-2", "timestamp=4"] {
            assert!(
                !all.contains(forbidden),
                "archive leaked {forbidden}: {all}"
            );
        }
    }

    #[test]
    fn partial_collection_still_exports_structured_error() {
        let dir = tempfile::tempdir().unwrap();
        let desktop_log = dir.path().join("desktop.log");
        fs::write(&desktop_log, "").unwrap();
        let destination = dir.path().join("partial.zip");
        export_host_scoped(
            &HostScopedSnapshot {
                generated_at: "2026-07-29T10:02:00Z".into(),
                app_version: "0.11.0".into(),
                selected_host: selected("host-1", "Build box", SelectedTransport::Https),
                desktop_log,
                include_agent_data: true,
                collection: HostCollection::Partial("unsupported_daemon"),
            },
            &destination,
        )
        .unwrap();
        let mut archive = zip::ZipArchive::new(File::open(destination).unwrap()).unwrap();
        let mut error = String::new();
        archive
            .by_name("host/collection-error.json")
            .unwrap()
            .read_to_string(&mut error)
            .unwrap();
        assert!(error.contains("unsupported_daemon"));
        let mut manifest = String::new();
        archive
            .by_name("manifest.json")
            .unwrap()
            .read_to_string(&mut manifest)
            .unwrap();
        assert!(manifest.contains("\"agent_data_effective\": false"));
    }

    #[test]
    fn unsafe_daemon_zip_entries_are_rejected() {
        let dir = tempfile::tempdir().unwrap();
        let desktop_log = dir.path().join("desktop.log");
        fs::write(&desktop_log, "").unwrap();
        for name in ["../escape", "/absolute", "a\\b"] {
            let destination = dir.path().join(format!("{}.zip", name.len()));
            let result = export_host_scoped(
                &HostScopedSnapshot {
                    generated_at: "2026-07-29T10:02:00Z".into(),
                    app_version: "0.11.0".into(),
                    selected_host: selected("host-1", "Build box", SelectedTransport::Ssh),
                    desktop_log: desktop_log.clone(),
                    include_agent_data: false,
                    collection: HostCollection::Archive(daemon_archive(&[(name, "bad")])),
                },
                &destination,
            );
            assert!(result.is_err(), "unsafe entry {name:?} was accepted");
            assert!(!destination.exists());
        }
    }
}
