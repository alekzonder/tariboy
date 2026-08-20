//! Persistent, non-secret remote-host metadata.
//!
//! The registry deliberately owns no credentials. Manual HTTPS bearer tokens
//! live behind `keychain::TokenStore`; SSH authentication stays entirely with
//! the user's system OpenSSH configuration and agent.

use serde::{Deserialize, Serialize};
use std::fs::{self, OpenOptions};
use std::io::Write;
use std::path::PathBuf;
use std::sync::Mutex;
use std::time::{SystemTime, UNIX_EPOCH};
use uuid::Uuid;

const SCHEMA_VERSION: u32 = 1;
const DEFAULT_REMOTE_INSTALL_DIR: &str = "~/.local/lib/tariboy";
const DEFAULT_REMOTE_PORT: u16 = 9990;
pub const STATE_EVENT: &str = "host://state";

#[derive(Debug, Clone, Copy, Deserialize, PartialEq, Eq, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum HostKind {
    Ssh,
    Https,
}

#[derive(Debug, Clone, Deserialize, PartialEq, Eq, Serialize)]
#[serde(deny_unknown_fields)]
pub struct HostRecord {
    pub id: String,
    pub label: String,
    pub kind: HostKind,
    pub ssh_alias: String,
    pub remote_install_dir: String,
    pub remote_port: u16,
    pub https_base_url: String,
    pub last_daemon_version: String,
}

#[derive(Debug, Clone, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct SaveSshInput {
    #[serde(default)]
    pub id: Option<String>,
    pub label: String,
    pub ssh_alias: String,
    #[serde(default)]
    pub remote_install_dir: String,
    #[serde(default)]
    pub remote_port: u16,
}

#[derive(Debug, Clone, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct SaveHttpsInput {
    #[serde(default)]
    pub id: Option<String>,
    pub label: String,
    pub https_base_url: String,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
#[serde(rename_all = "snake_case")]
#[allow(dead_code)] // Task 8/10 transition through the non-static runtime states.
pub enum HostState {
    Disconnected,
    Connecting,
    Provisioning,
    Ready,
    Degraded,
    NeedsAuth,
    Failed,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
pub struct HostView {
    pub id: String,
    pub label: String,
    pub kind: HostKind,
    pub ssh_alias: String,
    pub remote_install_dir: String,
    pub remote_port: u16,
    pub https_base_url: String,
    pub last_daemon_version: String,
    pub state: HostState,
    pub base_url: String,
    pub local_port: u16,
    pub phase: String,
    pub platform: String,
    pub arch: String,
    pub prerequisites: Vec<String>,
    pub message: String,
}

impl From<HostRecord> for HostView {
    fn from(record: HostRecord) -> Self {
        let (state, base_url) = match record.kind {
            HostKind::Https => (HostState::Ready, record.https_base_url.clone()),
            HostKind::Ssh => (HostState::Disconnected, String::new()),
        };
        Self {
            id: record.id,
            label: record.label,
            kind: record.kind,
            ssh_alias: record.ssh_alias,
            remote_install_dir: record.remote_install_dir,
            remote_port: record.remote_port,
            https_base_url: record.https_base_url,
            last_daemon_version: record.last_daemon_version,
            state,
            base_url,
            local_port: 0,
            phase: String::new(),
            platform: String::new(),
            arch: String::new(),
            prerequisites: Vec::new(),
            message: String::new(),
        }
    }
}

#[derive(Debug, Clone)]
struct HostRuntime {
    state: HostState,
    base_url: String,
    local_port: u16,
    phase: String,
    platform: String,
    arch: String,
    prerequisites: Vec<String>,
    message: String,
    daemon_version: String,
}

impl Default for HostRuntime {
    fn default() -> Self {
        Self {
            state: HostState::Disconnected,
            base_url: String::new(),
            local_port: 0,
            phase: String::new(),
            platform: String::new(),
            arch: String::new(),
            prerequisites: Vec::new(),
            message: String::new(),
            daemon_version: String::new(),
        }
    }
}

#[derive(Default)]
pub struct RuntimeHosts {
    hosts: Mutex<std::collections::HashMap<String, HostRuntime>>,
}

impl RuntimeHosts {
    pub fn view(&self, record: HostRecord) -> HostView {
        let mut view = HostView::from(record);
        if let Some(runtime) = self.hosts.lock().unwrap().get(&view.id).cloned() {
            view.state = runtime.state;
            view.base_url = runtime.base_url;
            view.local_port = runtime.local_port;
            view.phase = runtime.phase;
            view.platform = runtime.platform;
            view.arch = runtime.arch;
            view.prerequisites = runtime.prerequisites;
            view.message = runtime.message;
            if !runtime.daemon_version.is_empty() {
                view.last_daemon_version = runtime.daemon_version;
            }
        }
        view
    }

    pub fn healthy_tunnel(&self, id: &str) -> Option<(u16, String)> {
        self.hosts
            .lock()
            .unwrap()
            .get(id)
            .filter(|runtime| runtime.state == HostState::Ready && runtime.local_port != 0)
            .map(|runtime| (runtime.local_port, runtime.daemon_version.clone()))
    }

    pub fn set_phase(&self, id: &str, phase: &str) {
        let mut hosts = self.hosts.lock().unwrap();
        let runtime = hosts.entry(id.to_string()).or_default();
        runtime.state = HostState::Provisioning;
        runtime.phase = phase.to_string();
        runtime.message.clear();
    }

    pub fn set_preflight(
        &self,
        id: &str,
        platform: String,
        arch: String,
        prerequisites: Vec<String>,
        install_supported: bool,
    ) {
        let mut hosts = self.hosts.lock().unwrap();
        let runtime = hosts.entry(id.to_string()).or_default();
        runtime.state = if install_supported {
            HostState::Disconnected
        } else {
            HostState::Degraded
        };
        runtime.phase = "preflight".into();
        runtime.platform = platform;
        runtime.arch = arch;
        runtime.prerequisites = prerequisites;
        runtime.message.clear();
    }

    pub fn set_error(&self, id: &str, code: &str, message: &str) {
        let mut hosts = self.hosts.lock().unwrap();
        let runtime = hosts.entry(id.to_string()).or_default();
        runtime.state = if code == "needs_auth" {
            HostState::NeedsAuth
        } else {
            HostState::Failed
        };
        runtime.message = format!("{code}: {message}");
    }

    pub fn set_provisioned(
        &self,
        id: &str,
        daemon_version: String,
        degraded: bool,
        message: String,
    ) {
        let mut hosts = self.hosts.lock().unwrap();
        let runtime = hosts.entry(id.to_string()).or_default();
        runtime.state = if degraded {
            HostState::Degraded
        } else {
            HostState::Disconnected
        };
        runtime.phase = "status".into();
        runtime.daemon_version = daemon_version;
        runtime.message = message;
    }

    pub fn set_tunnel(&self, event: &crate::tunnel::Event) {
        let mut hosts = self.hosts.lock().unwrap();
        let runtime = hosts.entry(event.host_id.clone()).or_default();
        runtime.local_port = event.local_port;
        runtime.base_url = if event.local_port == 0 {
            String::new()
        } else {
            format!("http://127.0.0.1:{}", event.local_port)
        };
        runtime.phase = "connect".into();
        runtime.message.clone_from(&event.message);
        runtime.state = match event.state {
            crate::tunnel::State::Connecting | crate::tunnel::State::Retrying => {
                HostState::Connecting
            }
            crate::tunnel::State::Ready => HostState::Ready,
            crate::tunnel::State::NeedsAuth => HostState::NeedsAuth,
            crate::tunnel::State::Failed => HostState::Failed,
            crate::tunnel::State::Disconnected => HostState::Disconnected,
        };
        if let Some(status) = &event.status {
            runtime.daemon_version.clone_from(&status.version);
        }
    }

    pub fn remove(&self, id: &str) {
        self.hosts.lock().unwrap().remove(id);
    }

    #[cfg(debug_assertions)]
    pub(crate) fn set_image_transfer_test_target(
        &self,
        id: &str,
        base_url: &str,
        daemon_version: &str,
    ) {
        self.hosts.lock().unwrap().insert(
            id.to_string(),
            HostRuntime {
                state: HostState::Ready,
                base_url: base_url.to_string(),
                local_port: 0,
                phase: "desktop-e2e".into(),
                platform: "linux".into(),
                arch: "x86_64".into(),
                prerequisites: Vec::new(),
                message: String::new(),
                daemon_version: daemon_version.to_string(),
            },
        );
    }
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
struct RegistryFile {
    schema_version: u32,
    hosts: Vec<HostRecord>,
}

pub struct Registry {
    path: PathBuf,
    lock: Mutex<()>,
}

impl Registry {
    pub fn new(path: PathBuf) -> Self {
        Self {
            path,
            lock: Mutex::new(()),
        }
    }

    #[cfg(test)]
    pub fn path(&self) -> &std::path::Path {
        &self.path
    }

    pub fn list(&self) -> Result<Vec<HostRecord>, String> {
        let _guard = self
            .lock
            .lock()
            .map_err(|_| "host registry lock poisoned")?;
        self.load_unlocked()
    }

    pub fn get(&self, id: &str) -> Result<HostRecord, String> {
        self.list()?
            .into_iter()
            .find(|host| host.id == id)
            .ok_or_else(|| format!("host {id:?} not found"))
    }

    pub fn save_ssh(&self, input: SaveSshInput) -> Result<HostRecord, String> {
        let label = required("label", &input.label)?;
        let ssh_alias = required("ssh_alias", &input.ssh_alias)?;
        let install_dir = if input.remote_install_dir.trim().is_empty() {
            DEFAULT_REMOTE_INSTALL_DIR.to_string()
        } else {
            input.remote_install_dir.trim().to_string()
        };
        let remote_port = if input.remote_port == 0 {
            DEFAULT_REMOTE_PORT
        } else {
            input.remote_port
        };
        self.upsert(input.id, move |id, previous| HostRecord {
            id,
            label,
            kind: HostKind::Ssh,
            ssh_alias,
            remote_install_dir: install_dir,
            remote_port,
            https_base_url: String::new(),
            last_daemon_version: previous
                .map(|host| host.last_daemon_version)
                .unwrap_or_default(),
        })
    }

    pub fn save_https(&self, input: SaveHttpsInput) -> Result<HostRecord, String> {
        let label = required("label", &input.label)?;
        let https_base_url = normalise_https_url(&input.https_base_url)?;
        self.upsert(input.id, move |id, previous| HostRecord {
            id,
            label,
            kind: HostKind::Https,
            ssh_alias: String::new(),
            remote_install_dir: String::new(),
            remote_port: 0,
            https_base_url,
            last_daemon_version: previous
                .map(|host| host.last_daemon_version)
                .unwrap_or_default(),
        })
    }

    pub fn remove(&self, id: &str) -> Result<(), String> {
        let _guard = self
            .lock
            .lock()
            .map_err(|_| "host registry lock poisoned")?;
        let mut hosts = self.load_unlocked()?;
        let before = hosts.len();
        hosts.retain(|host| host.id != id);
        if hosts.len() == before {
            return Err(format!("host {id:?} not found"));
        }
        self.write_unlocked(&hosts)
    }

    pub fn set_last_daemon_version(&self, id: &str, version: &str) -> Result<(), String> {
        let _guard = self
            .lock
            .lock()
            .map_err(|_| "host registry lock poisoned")?;
        let mut hosts = self.load_unlocked()?;
        let host = hosts
            .iter_mut()
            .find(|host| host.id == id)
            .ok_or_else(|| format!("host {id:?} not found"))?;
        host.last_daemon_version = version.to_string();
        self.write_unlocked(&hosts)
    }

    pub(crate) fn restore(&self, record: HostRecord) -> Result<(), String> {
        let _guard = self
            .lock
            .lock()
            .map_err(|_| "host registry lock poisoned")?;
        let mut hosts = self.load_unlocked()?;
        match hosts.iter().position(|host| host.id == record.id) {
            Some(index) => hosts[index] = record,
            None => hosts.push(record),
        }
        self.write_unlocked(&hosts)
    }

    fn upsert<F>(&self, id: Option<String>, build: F) -> Result<HostRecord, String>
    where
        F: FnOnce(String, Option<HostRecord>) -> HostRecord,
    {
        let _guard = self
            .lock
            .lock()
            .map_err(|_| "host registry lock poisoned")?;
        let mut hosts = self.load_unlocked()?;
        let (id, index, previous) = match id {
            Some(id) => {
                let index = hosts
                    .iter()
                    .position(|host| host.id == id)
                    .ok_or_else(|| format!("host {id:?} not found"))?;
                (id, Some(index), Some(hosts[index].clone()))
            }
            None => (Uuid::new_v4().to_string(), None, None),
        };
        let record = build(id, previous);
        match index {
            Some(index) => hosts[index] = record.clone(),
            None => hosts.push(record.clone()),
        }
        self.write_unlocked(&hosts)?;
        Ok(record)
    }

    fn load_unlocked(&self) -> Result<Vec<HostRecord>, String> {
        let bytes = match fs::read(&self.path) {
            Ok(bytes) => bytes,
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(Vec::new()),
            Err(error) => return Err(format!("read {}: {error}", self.path.display())),
        };
        let file: RegistryFile = match serde_json::from_slice(&bytes) {
            Ok(file) => file,
            Err(error) => {
                let backup = self
                    .existing_corrupt_backup(&bytes)
                    .unwrap_or_else(|| self.corrupt_backup_path());
                if !backup.exists() {
                    fs::copy(&self.path, &backup).map_err(|copy_error| {
                        format!(
                            "host registry is corrupt ({error}); preserve as {}: {copy_error}",
                            backup.display()
                        )
                    })?;
                }
                return Err(format!(
                    "host registry is corrupt ({error}); preserved as {}",
                    backup.display()
                ));
            }
        };
        if file.schema_version != SCHEMA_VERSION {
            return Err(format!(
                "unsupported host registry schema {}, expected {}",
                file.schema_version, SCHEMA_VERSION
            ));
        }
        Ok(file.hosts)
    }

    fn write_unlocked(&self, hosts: &[HostRecord]) -> Result<(), String> {
        let parent = self
            .path
            .parent()
            .ok_or_else(|| format!("host registry has no parent: {}", self.path.display()))?;
        fs::create_dir_all(parent)
            .map_err(|error| format!("create {}: {error}", parent.display()))?;
        let temp = parent.join(format!(
            ".{}.tmp-{}",
            self.path
                .file_name()
                .and_then(|name| name.to_str())
                .unwrap_or("hosts.json"),
            Uuid::new_v4()
        ));
        let bytes = serde_json::to_vec_pretty(&RegistryFile {
            schema_version: SCHEMA_VERSION,
            hosts: hosts.to_vec(),
        })
        .map_err(|error| format!("encode host registry: {error}"))?;

        let mut options = OpenOptions::new();
        options.write(true).create_new(true);
        #[cfg(unix)]
        {
            use std::os::unix::fs::OpenOptionsExt;
            options.mode(0o600);
        }
        let write_result = (|| -> Result<(), String> {
            let mut file = options
                .open(&temp)
                .map_err(|error| format!("create {}: {error}", temp.display()))?;
            file.write_all(&bytes)
                .map_err(|error| format!("write {}: {error}", temp.display()))?;
            file.sync_all()
                .map_err(|error| format!("sync {}: {error}", temp.display()))?;
            fs::rename(&temp, &self.path).map_err(|error| {
                format!(
                    "replace {} with {}: {error}",
                    self.path.display(),
                    temp.display()
                )
            })?;
            Ok(())
        })();
        if write_result.is_err() {
            let _ = fs::remove_file(&temp);
        }
        write_result
    }

    fn corrupt_backup_path(&self) -> PathBuf {
        let millis = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_millis();
        let name = self
            .path
            .file_name()
            .and_then(|name| name.to_str())
            .unwrap_or("hosts.json");
        self.path
            .with_file_name(format!("{name}.corrupt-{millis}-{}", Uuid::new_v4()))
    }

    fn existing_corrupt_backup(&self, corrupt_bytes: &[u8]) -> Option<PathBuf> {
        let parent = self.path.parent()?;
        let name = self.path.file_name()?.to_string_lossy();
        let prefix = format!("{name}.corrupt-");
        fs::read_dir(parent)
            .ok()?
            .filter_map(Result::ok)
            .map(|entry| entry.path())
            .find(|path| {
                path.file_name()
                    .is_some_and(|file| file.to_string_lossy().starts_with(&prefix))
                    && fs::read(path).is_ok_and(|bytes| bytes == corrupt_bytes)
            })
    }
}

fn required(field: &str, value: &str) -> Result<String, String> {
    let value = value.trim();
    if value.is_empty() {
        return Err(format!("{field} is required"));
    }
    Ok(value.to_string())
}

pub(crate) fn normalise_https_url(value: &str) -> Result<String, String> {
    let value = value.trim().trim_end_matches('/');
    if !value.starts_with("https://") || value.len() == "https://".len() {
        return Err("https_base_url must be a non-empty HTTPS URL beginning with https://".into());
    }
    Ok(value.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;

    fn registry(dir: &tempfile::TempDir) -> Registry {
        Registry::new(dir.path().join("hosts.json"))
    }

    fn https(label: &str, id: Option<String>) -> SaveHttpsInput {
        SaveHttpsInput {
            id,
            label: label.into(),
            https_base_url: "https://example.internal:8765/".into(),
        }
    }

    #[test]
    fn missing_file_yields_empty_registry() {
        let dir = tempfile::tempdir().unwrap();
        assert_eq!(registry(&dir).list().unwrap(), Vec::<HostRecord>::new());
    }

    #[test]
    fn create_update_list_remove_round_trip_keeps_uuid() {
        let dir = tempfile::tempdir().unwrap();
        let store = registry(&dir);

        let created = store.save_https(https("Production", None)).unwrap();
        uuid::Uuid::parse_str(&created.id).expect("new ids are UUIDs");
        assert_eq!(created.https_base_url, "https://example.internal:8765");

        let updated = store
            .save_https(https("Production renamed", Some(created.id.clone())))
            .unwrap();
        assert_eq!(updated.id, created.id);
        assert_eq!(store.list().unwrap(), vec![updated.clone()]);

        store.remove(&created.id).unwrap();
        assert!(store.list().unwrap().is_empty());
    }

    #[test]
    fn manual_https_hosts_reject_cleartext_http() {
        let dir = tempfile::tempdir().unwrap();
        let store = registry(&dir);
        let error = store
            .save_https(SaveHttpsInput {
                id: None,
                label: "Unsafe".into(),
                https_base_url: "http://remote.internal:9990".into(),
            })
            .unwrap_err();
        assert!(error.contains("https://"), "{error}");
        assert!(store.list().unwrap().is_empty());
    }

    #[test]
    fn daemon_version_update_is_persisted_without_changing_transport() {
        let dir = tempfile::tempdir().unwrap();
        let registry = Registry::new(dir.path().join("hosts.json"));
        let host = registry
            .save_ssh(SaveSshInput {
                id: None,
                label: "prod".into(),
                ssh_alias: "prod".into(),
                remote_install_dir: String::new(),
                remote_port: 0,
            })
            .unwrap();

        registry
            .set_last_daemon_version(&host.id, "0.9.0-dev")
            .unwrap();

        let updated = registry.get(&host.id).unwrap();
        assert_eq!(updated.last_daemon_version, "0.9.0-dev");
        assert_eq!(updated.ssh_alias, "prod");
    }

    #[test]
    fn replacement_is_atomic_and_owner_only() {
        use std::os::unix::fs::PermissionsExt;

        let dir = tempfile::tempdir().unwrap();
        let store = registry(&dir);
        store.save_https(https("one", None)).unwrap();
        store.save_https(https("two", None)).unwrap();

        let mode = fs::metadata(store.path()).unwrap().permissions().mode() & 0o777;
        assert_eq!(mode, 0o600);
        let leftovers = fs::read_dir(dir.path())
            .unwrap()
            .filter_map(Result::ok)
            .filter(|entry| entry.file_name().to_string_lossy().contains(".tmp-"))
            .count();
        assert_eq!(leftovers, 0, "atomic replacement left temporary files");
        assert_eq!(store.list().unwrap().len(), 2);
    }

    #[test]
    fn corrupt_json_is_preserved_and_reported() {
        let dir = tempfile::tempdir().unwrap();
        let store = registry(&dir);
        fs::write(store.path(), b"{ definitely not json").unwrap();

        let err = store.list().unwrap_err();
        assert!(err.contains("corrupt"), "{err}");
        assert_eq!(fs::read(store.path()).unwrap(), b"{ definitely not json");
        let second = store.list().unwrap_err();
        assert!(second.contains("corrupt"), "{second}");
        let backups: Vec<_> = fs::read_dir(dir.path())
            .unwrap()
            .filter_map(Result::ok)
            .filter(|entry| {
                entry
                    .file_name()
                    .to_string_lossy()
                    .starts_with("hosts.json.corrupt-")
            })
            .collect();
        assert_eq!(backups.len(), 1);
        assert_eq!(
            fs::read(backups[0].path()).unwrap(),
            b"{ definitely not json"
        );

        fs::write(store.path(), b"[ a different corruption").unwrap();
        assert!(store.list().unwrap_err().contains("corrupt"));
        let backups: Vec<_> = fs::read_dir(dir.path())
            .unwrap()
            .filter_map(Result::ok)
            .filter(|entry| {
                entry
                    .file_name()
                    .to_string_lossy()
                    .starts_with("hosts.json.corrupt-")
            })
            .collect();
        assert_eq!(backups.len(), 2);
        assert!(backups
            .iter()
            .any(|entry| fs::read(entry.path()).unwrap() == b"[ a different corruption"));
    }

    #[test]
    fn persisted_json_has_no_secret_fields_or_values() {
        let dir = tempfile::tempdir().unwrap();
        let store = registry(&dir);
        store.save_https(https("safe", None)).unwrap();

        let raw = fs::read_to_string(store.path()).unwrap();
        for forbidden in ["token", "password", "passphrase", "super-secret"] {
            assert!(!raw.to_lowercase().contains(forbidden), "{raw}");
        }
    }

    #[test]
    fn runtime_preflight_and_auth_failure_are_attached_to_host_view() {
        let dir = tempfile::tempdir().unwrap();
        let store = registry(&dir);
        let record = store
            .save_ssh(SaveSshInput {
                id: None,
                label: "prod".into(),
                ssh_alias: "prod".into(),
                remote_install_dir: String::new(),
                remote_port: 0,
            })
            .unwrap();
        let runtime = RuntimeHosts::default();

        runtime.set_phase(&record.id, "authenticate");
        let connecting = runtime.view(record.clone());
        assert_eq!(connecting.state, HostState::Provisioning);
        assert_eq!(connecting.phase, "authenticate");

        runtime.set_preflight(
            &record.id,
            "Linux".into(),
            "x86_64".into(),
            vec!["codex".into()],
            true,
        );
        let checked = runtime.view(record.clone());
        assert_eq!(checked.state, HostState::Disconnected);
        assert_eq!(checked.platform, "Linux");
        assert_eq!(checked.arch, "x86_64");
        assert_eq!(checked.prerequisites, vec!["codex"]);

        runtime.set_tunnel(&crate::tunnel::Event {
            host_id: record.id.clone(),
            state: crate::tunnel::State::Ready,
            local_port: 18444,
            status: Some(crate::daemon::Status {
                version: "0.9.0-dev".into(),
                pid: 42,
                base_dir: "/home/u/.tariboy".into(),
                http_addr: "127.0.0.1:9990".into(),
            }),
            message: String::new(),
        });
        let ready = runtime.view(record.clone());
        assert_eq!(ready.state, HostState::Ready);
        assert_eq!(ready.base_url, "http://127.0.0.1:18444");
        assert_eq!(ready.local_port, 18444);
        assert_eq!(
            runtime.healthy_tunnel(&record.id),
            Some((18444, "0.9.0-dev".into()))
        );

        runtime.set_error(&record.id, "needs_auth", "Verification code required");
        let failed = runtime.view(record);
        assert_eq!(failed.state, HostState::NeedsAuth);
        assert!(failed.message.contains("needs_auth"));
    }
}
