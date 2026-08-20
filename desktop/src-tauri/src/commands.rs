//! Tauri IPC. Marshalling only — every decision lives in daemon.rs / state.rs /
//! cli_install.rs so it stays unit-testable without a running webview.

use crate::{
    bundle, cli_install, daemon, hosts, keychain, notifications, paths, preflight, provision,
    remote_health, ssh,
    state::{AppState, DaemonView, Phase},
    support, support_client, tunnel,
};
use serde::Serialize;
use std::sync::Arc;
use std::time::Duration;
use tauri::Emitter;
use tauri::{AppHandle, Manager, State};

#[derive(Debug, Serialize)]
pub struct InstallResult {
    pub outcome: String,
    pub link: String,
    pub target: String,
    pub existing: String,
}

#[derive(Debug, Serialize)]
pub struct SessionCredentials {
    pub base_url: String,
    pub token: String,
}

#[derive(Debug, Serialize)]
pub struct OperationResult {
    pub operation_id: String,
}

#[derive(Debug, Serialize)]
pub struct SupportBundleResult {
    pub path: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct ActiveWork {
    pub agent: String,
    pub state: String,
    pub running_iterations: Vec<String>,
}

pub fn outcome_name(o: &cli_install::Outcome) -> &'static str {
    match o {
        cli_install::Outcome::Created => "created",
        cli_install::Outcome::AlreadyInstalled => "already-installed",
        cli_install::Outcome::Occupied { .. } => "occupied",
    }
}

/// bring_up is shared by the command, the tray and setup: probe-or-start, then
/// publish whatever happened. It never returns without having set a state.
pub fn bring_up(app: &AppHandle) -> DaemonView {
    let st = app.state::<AppState>();
    st.set(app, DaemonView::starting(st.bundle.version()));
    match daemon::ensure_up(&st.cfg) {
        Ok(ready) => {
            if let Some(child) = ready.child {
                // A new child supersedes any watcher still registered for an
                // older one, so claim a fresh generation for it.
                let generation = st.next_generation();
                crate::state::watch_daemon(app.clone(), child, generation);
            }
            let view = DaemonView::ready(&ready.status, ready.adopted, st.bundle.version());
            st.set(app, view.clone());
            view
        }
        Err(e) => {
            let view = DaemonView::failed(&e, st.bundle.version());
            st.set(app, view.clone());
            view
        }
    }
}

/// restart_daemon is the shared stop-then-start. The command and the tray item
/// both call it so the publish sequence exists in exactly one place.
pub fn restart_daemon(app: &AppHandle) -> DaemonView {
    let st = app.state::<AppState>();
    // Retire the watcher for the child we are about to kill: its exit is the
    // point of this call, and this function publishes the outcome itself. Without
    // this, the dying child's watcher could land a `down` AFTER the new daemon's
    // `ready`.
    st.next_generation();
    st.set(app, DaemonView::starting(st.bundle.version()));
    match daemon::restart(&st.cfg) {
        Ok(ready) => {
            if let Some(child) = ready.child {
                let generation = st.next_generation();
                crate::state::watch_daemon(app.clone(), child, generation);
            }
            let view = DaemonView::ready(&ready.status, ready.adopted, st.bundle.version());
            st.set(app, view.clone());
            view
        }
        Err(e) => {
            let view = DaemonView::failed(&e, st.bundle.version());
            st.set(app, view.clone());
            view
        }
    }
}

/// stop_daemon shuts the daemon down and publishes the outcome.
pub fn stop_daemon(app: &AppHandle) -> DaemonView {
    let st = app.state::<AppState>();
    // Same as restart: we are killing our own child on purpose and publishing the
    // result below, so its watcher must not race us with a second `down`.
    st.next_generation();
    let view = match daemon::stop(&st.cfg) {
        Ok(()) => DaemonView::down(st.bundle.version()),
        Err(e) => DaemonView::failed(&e, st.bundle.version()),
    };
    st.set(app, view.clone());
    view
}

/// off_main runs a blocking lifecycle step somewhere other than the macOS UI
/// thread.
///
/// This is not a nicety. macOS delivers the webview's IPC callback on the MAIN
/// thread, and tauri-macros expands a NON-async `#[tauri::command]` through
/// `body_blocking`, i.e. it runs INLINE on that thread. `ensure_up` can block for
/// the whole READY_TIMEOUT and a restart for stop+start, so a banner button would
/// beachball the app for up to 20 seconds.
///
/// It can also hard-deadlock. `MenuItem::set_text` is `run_item_main_thread!`:
/// from a background thread it blocks until the event loop pumps. With the event
/// loop parked inside a command, the exit watcher would block there while the
/// command waits on the state it is holding — force-quit territory.
///
/// `async` hands the command to tauri's async runtime, and `spawn_blocking` puts
/// the multi-second wait on tokio's blocking pool rather than starving a worker.
async fn off_main<F>(app: AppHandle, step: F) -> DaemonView
where
    F: FnOnce(&AppHandle) -> DaemonView + Send + 'static,
{
    let h = app.clone();
    match tauri::async_runtime::spawn_blocking(move || step(&h)).await {
        Ok(view) => view,
        // The step never completed (panic, or the runtime going away). Say so in
        // the banner instead of leaving the UI stuck on "starting…".
        Err(e) => {
            let st = app.state::<AppState>();
            let view = DaemonView::failed(
                &format!("daemon task did not run: {e}"),
                st.bundle.version(),
            );
            st.set(&app, view.clone());
            view
        }
    }
}

// Each #[tauri::command] below is reachable only because main.rs lists it in
// generate_handler!; the names there are what `ui/src/lib/desktop.ts` invokes.
// The `async` ones take only injected arguments, so the JS-visible payload is
// unchanged — the frontend already awaits every one of them.
#[tauri::command]
pub fn daemon_status(state: State<AppState>) -> DaemonView {
    // No reaping here: watch_daemon already pushes `down` the moment a daemon we
    // started exits, so this is a pure read of the current view. Cheap enough to
    // stay on the UI thread.
    state.view()
}

#[tauri::command]
pub async fn daemon_start(app: AppHandle) -> DaemonView {
    off_main(app, bring_up).await
}

#[tauri::command]
pub async fn daemon_restart(app: AppHandle) -> DaemonView {
    off_main(app, restart_daemon).await
}

#[tauri::command]
pub async fn daemon_stop(app: AppHandle) -> DaemonView {
    off_main(app, stop_daemon).await
}

#[tauri::command]
pub async fn task_notification_show(
    app: AppHandle,
    input: notifications::TaskNotificationInput,
) -> Result<notifications::TaskNotificationResult, String> {
    notifications::show(app, input).await
}

#[tauri::command]
pub fn task_notification_activate_test(
    app: AppHandle,
    input: notifications::TaskNotificationActivation,
) -> Result<(), String> {
    notifications::activate_test(&app, input)
}

#[tauri::command]
pub fn daemon_log(state: State<AppState>, lines: Option<usize>) -> String {
    daemon::log_tail(&state.cfg.log_file, lines.unwrap_or(40))
}

/// open_daemon_log hands the log to the system default viewer (Console.app /
/// the user's editor), which is more useful than a tail inside a banner.
///
/// Shared with the tray's "Open daemon log" so there is one implementation.
#[tauri::command]
pub fn open_daemon_log(state: State<AppState>) -> Result<(), String> {
    match std::process::Command::new("/usr/bin/open")
        .arg(&state.cfg.log_file)
        .spawn()
    {
        // `open` exits as soon as it has handed the file to the viewer, but
        // nobody waits on it — so every click used to leave a zombie for the
        // app's lifetime. A detached reaper costs one short-lived thread.
        Ok(child) => {
            daemon::watch_exit(child, || {});
            Ok(())
        }
        Err(e) => Err(format!("open {}: {e}", state.cfg.log_file.display())),
    }
}

/// The two errors the external-link boundary is allowed to report. They are
/// fixed strings on purpose: the UI shows them and the log keeps them, so
/// neither may carry the terminal text, its host or its query.
const INVALID_EXTERNAL_URL: &str = "invalid external web link";
const EXTERNAL_OPEN_FAILED: &str = "cannot open external web link";

/// Test-only seam. The isolated Desktop E2E fixture sets it to an owner-only
/// file below its own temp root; production leaves it absent.
const EXTERNAL_OPEN_LOG_ENV: &str = "TARIBOY_DESKTOP_EXTERNAL_OPEN_LOG";

/// checked_web_url is the whole authorization decision: an absolute, parseable
/// URL whose scheme is exactly http or https. Relative text, bare hosts and
/// every non-web scheme fail here, so neither the xterm matcher nor the WebView
/// is trusted to have filtered anything.
fn checked_web_url(raw: &str) -> Result<url::Url, String> {
    let url = url::Url::parse(raw).map_err(|_| INVALID_EXTERNAL_URL.to_string())?;
    match url.scheme() {
        "http" | "https" => Ok(url),
        _ => Err(INVALID_EXTERNAL_URL.to_string()),
    }
}

/// record_external_open appends one accepted URL to the E2E observation file.
/// Owner-only, and only ever reached after validation.
fn record_external_open(path: &std::path::Path, url: &url::Url) -> Result<(), String> {
    use std::io::Write;
    use std::os::unix::fs::OpenOptionsExt;

    let mut file = std::fs::OpenOptions::new()
        .create(true)
        .append(true)
        .mode(0o600)
        .open(path)
        .map_err(|_| EXTERNAL_OPEN_FAILED.to_string())?;
    writeln!(file, "{url}").map_err(|_| EXTERNAL_OPEN_FAILED.to_string())
}

/// open_external_url_with is the testable core: validate, then either record the
/// open for the E2E observer or hand the serialized URL to the platform opener.
/// The observation path REPLACES the opener so an E2E run never launches a real
/// browser, and every opener failure collapses to one fixed message.
fn open_external_url_with<F>(
    opener: F,
    raw: &str,
    observation_path: Option<&std::path::Path>,
) -> Result<(), String>
where
    F: FnOnce(&url::Url) -> Result<(), String>,
{
    let url = checked_web_url(raw)?;
    match observation_path {
        Some(path) => record_external_open(path, &url),
        None => opener(&url).map_err(|_| EXTERNAL_OPEN_FAILED.to_string()),
    }
}

/// launch_platform_opener passes the serialized URL as a single argv entry —
/// never a shell string — to the platform's default handler.
fn launch_platform_opener(url: &url::Url) -> Result<(), String> {
    #[cfg(target_os = "macos")]
    const OPENER: &str = "/usr/bin/open";
    #[cfg(not(target_os = "macos"))]
    const OPENER: &str = "xdg-open";

    match std::process::Command::new(OPENER).arg(url.as_str()).spawn() {
        // Same detached reaper as open_daemon_log: nobody waits on the opener,
        // so without it every click would leave a zombie for the app's lifetime.
        Ok(child) => {
            daemon::watch_exit(child, || {});
            Ok(())
        }
        Err(error) => Err(format!("spawn {OPENER}: {error}")),
    }
}

/// open_external_url opens a Command-clicked terminal link in the operator's
/// browser. The UI has already parsed the text; this re-validates it because the
/// WebView is not an authorization boundary.
#[tauri::command]
pub fn open_external_url(url: String) -> Result<(), String> {
    let observation = std::env::var_os(EXTERNAL_OPEN_LOG_ENV);
    open_external_url_with(
        launch_platform_opener,
        &url,
        observation.as_ref().map(std::path::Path::new),
    )
}

fn encode_uri_component(value: &str, preserve_slashes: bool) -> String {
    const HEX: &[u8; 16] = b"0123456789ABCDEF";
    let mut encoded = String::with_capacity(value.len());
    for byte in value.bytes() {
        if byte.is_ascii_alphanumeric()
            || matches!(byte, b'-' | b'.' | b'_' | b'~')
            || (preserve_slashes && byte == b'/')
        {
            encoded.push(char::from(byte));
        } else {
            encoded.push('%');
            encoded.push(char::from(HEX[usize::from(byte >> 4)]));
            encoded.push(char::from(HEX[usize::from(byte & 0x0f)]));
        }
    }
    encoded
}

fn vscode_folder_uri(
    registry: &hosts::Registry,
    host_id: &str,
    path: &str,
) -> Result<String, String> {
    if !std::path::Path::new(path).is_absolute() {
        return Err("folder path must be absolute".into());
    }
    let path = encode_uri_component(path, true);
    if host_id.is_empty() {
        return Ok(format!("vscode://file{path}"));
    }

    let host = registry.get(host_id)?;
    if host.kind != hosts::HostKind::Ssh {
        return Err("VS Code folder opening requires a local or SSH host".into());
    }
    let alias = encode_uri_component(&host.ssh_alias, false);
    Ok(format!("vscode://vscode-remote/ssh-remote+{alias}{path}"))
}

#[tauri::command]
pub fn open_host_path_in_vscode(
    state: State<AppState>,
    host_id: String,
    path: String,
) -> Result<(), String> {
    let uri = vscode_folder_uri(&state.hosts, &host_id, &path)?;
    match std::process::Command::new("/usr/bin/open")
        .arg(&uri)
        .spawn()
    {
        Ok(child) => {
            daemon::watch_exit(child, || {});
            Ok(())
        }
        Err(error) => Err(format!("open folder in VS Code: {error}")),
    }
}

#[tauri::command]
pub async fn support_bundle_export(
    app: AppHandle,
    state: State<'_, AppState>,
    host_id: String,
    include_agent_data: bool,
) -> Result<Option<SupportBundleResult>, String> {
    use std::time::{SystemTime, UNIX_EPOCH};
    use tauri_plugin_dialog::DialogExt;

    let target = support_client::resolve_target(
        &host_id,
        &state.view(),
        &state.hosts,
        &state.host_runtime,
        state.tokens.as_ref(),
    )
    .map_err(str::to_string)?;
    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_err(|error| format!("read system time: {error}"))?
        .as_secs();
    let suggested = support::suggested_name(&target.id, &target.label, now);
    let chosen = app
        .dialog()
        .file()
        .set_file_name(&suggested)
        .add_filter("ZIP archive", &["zip"])
        .blocking_save_file();
    let Some(chosen) = chosen else {
        return Ok(None);
    };
    let destination = chosen
        .into_path()
        .map_err(|error| format!("resolve support bundle destination: {error}"))?;
    let app_version = state.bundle.version().to_string();
    let desktop_log = state.paths.app_data.join("desktop.log");
    let selected_host = target.diagnostic.clone();
    let path = tauri::async_runtime::spawn_blocking(move || {
        let collection = match support_client::fetch(&target, include_agent_data) {
            support_client::FetchOutcome::Archive(body) => support::HostCollection::Archive(body),
            support_client::FetchOutcome::Partial(code) => support::HostCollection::Partial(code),
        };
        support::export_host_scoped(
            &support::HostScopedSnapshot {
                generated_at: now.to_string(),
                app_version,
                selected_host,
                desktop_log,
                include_agent_data,
                collection,
            },
            &destination,
        )
    })
    .await
    .map_err(|error| format!("support bundle task did not run: {error}"))??;
    Ok(Some(SupportBundleResult {
        path: path.display().to_string(),
    }))
}

fn install_then_restart(
    install: impl FnOnce() -> std::io::Result<cli_install::Outcome>,
    restart: impl FnOnce() -> Result<(), String>,
) -> Result<cli_install::Outcome, String> {
    let outcome = install().map_err(|error| format!("install CLI binaries: {error}"))?;
    if matches!(outcome, cli_install::Outcome::Occupied { .. }) {
        return Ok(outcome);
    }
    restart().map_err(|error| {
        format!("CLI binaries are installed, but daemon restart failed: {error}")
    })?;
    Ok(outcome)
}

fn handoff_capable_version(version: &str) -> bool {
    let normalized = version.strip_prefix('v').unwrap_or(version);
    let core = normalized
        .split_once('-')
        .map_or(normalized, |(core, _)| core);
    let mut parts = core.split('.');
    let parsed = (
        parts.next().and_then(|part| part.parse::<u64>().ok()),
        parts.next().and_then(|part| part.parse::<u64>().ok()),
        parts.next().and_then(|part| part.parse::<u64>().ok()),
        parts.next(),
    );
    matches!(
        parsed,
        (Some(major), Some(minor), Some(patch), None)
            if (major, minor, patch) >= (0, 10, 1)
    )
}

fn loopback_http_port(base_url: &str) -> Result<u16, String> {
    let authority = base_url
        .strip_prefix("http://")
        .ok_or_else(|| format!("daemon has an invalid loopback HTTP address: {base_url}"))?;
    let address = authority
        .parse::<std::net::SocketAddr>()
        .map_err(|_| format!("daemon has an invalid loopback HTTP address: {base_url}"))?;
    if !address.ip().is_loopback() {
        return Err(format!(
            "daemon HTTP address is not loopback and cannot be probed safely: {base_url}"
        ));
    }
    Ok(address.port())
}

fn restart_handoff_preflight(
    view: &DaemonView,
    active_work: impl FnOnce(u16) -> Result<Vec<ActiveWork>, String>,
) -> Result<(), String> {
    match view.state {
        Phase::Down | Phase::Failed => return Ok(()),
        Phase::Starting => {
            return Err(
                "daemon is still starting; wait for it to become ready, then retry the update"
                    .into(),
            );
        }
        Phase::Ready if handoff_capable_version(&view.daemon_version) => return Ok(()),
        Phase::Ready => {}
    }

    let port = loopback_http_port(&view.base_url)?;
    let active = active_work(port).map_err(|error| {
        format!(
            "cannot safely restart daemon {} because active work could not be checked: {error}",
            if view.daemon_version.is_empty() {
                "with unknown version"
            } else {
                &view.daemon_version
            }
        )
    })?;
    if active.is_empty() {
        return Ok(());
    }

    let details = active
        .iter()
        .map(|work| {
            if work.running_iterations.is_empty() {
                format!("{} ({})", work.agent, work.state)
            } else {
                format!(
                    "{} ({}; iterations: {})",
                    work.agent,
                    work.state,
                    work.running_iterations.join(", ")
                )
            }
        })
        .collect::<Vec<_>>()
        .join("; ");
    Err(format!(
        "daemon {} predates restart-safe handoff and still has active work: {details}. \
         Let that work finish or stop it, then retry Install/Update CLI",
        if view.daemon_version.is_empty() {
            "with unknown version"
        } else {
            &view.daemon_version
        }
    ))
}

fn remote_restart_handoff_preflight(
    running_version: &str,
    staged_previous: &str,
    active_work: impl FnOnce() -> Result<Vec<ActiveWork>, String>,
) -> Result<(), String> {
    if handoff_capable_version(running_version) {
        return Ok(());
    }

    let daemon = if running_version.is_empty() {
        "with unknown version".to_string()
    } else {
        running_version.to_string()
    };
    let links = if staged_previous.is_empty() {
        "managed release links are absent".to_string()
    } else if staged_previous == running_version {
        format!("managed release links match {staged_previous}")
    } else {
        format!("managed release links point to {staged_previous}")
    };
    let active = active_work().map_err(|error| {
        format!(
            "cannot safely restart legacy daemon {daemon} ({links}) because active work could \
             not be checked: {error}"
        )
    })?;
    if active.is_empty() {
        return Ok(());
    }

    let details = active
        .iter()
        .map(|work| {
            if work.running_iterations.is_empty() {
                format!("{} ({})", work.agent, work.state)
            } else {
                format!(
                    "{} ({}; iterations: {})",
                    work.agent,
                    work.state,
                    work.running_iterations.join(", ")
                )
            }
        })
        .collect::<Vec<_>>()
        .join("; ");
    Err(format!(
        "daemon {daemon} predates restart-safe handoff ({links}) and still has active work: \
         {details}. Let that work finish or stop it, then retry the update"
    ))
}

pub fn install_update_cli(app: &AppHandle) -> Result<InstallResult, String> {
    let state = app.state::<AppState>();
    let observed = match daemon::probe(&state.cfg.socket, Duration::from_secs(2)) {
        daemon::Probe::Down => DaemonView::down(state.bundle.version()),
        daemon::Probe::Up(Some(status)) => DaemonView::ready(&status, true, state.bundle.version()),
        daemon::Probe::Up(None) => {
            return Err(
                "cannot safely restart the running daemon because its status is unreadable".into(),
            );
        }
    };
    restart_handoff_preflight(&observed, active_remote_work)?;
    let link_dir = cli_install::default_bin_dir(&paths::env_getter)?;
    let source = state.bundle.local_bundle()?;
    let outcome = install_then_restart(
        || cli_install::install_all(&link_dir, &source.dir, &bundle::BINARIES),
        || {
            let view = restart_daemon(app);
            if view.state == Phase::Ready {
                Ok(())
            } else if view.message.is_empty() {
                Err(format!("daemon ended in state {:?}", view.state))
            } else {
                Err(view.message)
            }
        },
    )?;
    let link = link_dir.join("tariboy");
    let target = source.file("tariboy");
    let existing = match &outcome {
        cli_install::Outcome::Occupied { existing } => existing.clone(),
        _ => String::new(),
    };
    Ok(InstallResult {
        outcome: outcome_name(&outcome).to_string(),
        link: link.display().to_string(),
        target: target.display().to_string(),
        existing,
    })
}

#[tauri::command]
pub async fn install_cli(app: AppHandle) -> Result<InstallResult, String> {
    let handle = app.clone();
    tauri::async_runtime::spawn_blocking(move || install_update_cli(&handle))
        .await
        .map_err(|error| format!("Install/Update CLI task did not run: {error}"))?
}

fn list_host_views(
    registry: &hosts::Registry,
    runtime: &hosts::RuntimeHosts,
) -> Result<Vec<hosts::HostView>, String> {
    Ok(registry
        .list()?
        .into_iter()
        .map(|record| runtime.view(record))
        .collect())
}

fn save_https_host(
    registry: &hosts::Registry,
    tokens: &dyn keychain::TokenStore,
    input: hosts::SaveHttpsInput,
    token: Option<String>,
) -> Result<hosts::HostView, String> {
    let previous = input.id.as_deref().map(|id| registry.get(id)).transpose()?;
    let record = registry.save_https(input)?;
    let token_result = if let Some(token) = token {
        if token.is_empty() {
            tokens.delete(&record.id)
        } else {
            tokens.set(&record.id, &token)
        }
    } else {
        Ok(())
    };
    if let Err(error) = token_result {
        let rollback = match previous {
            Some(previous) => registry.restore(previous),
            None => registry.remove(&record.id),
        };
        return match rollback {
            Ok(()) => Err(error),
            Err(rollback_error) => Err(format!(
                "{error}; additionally failed to roll back host metadata: {rollback_error}"
            )),
        };
    }
    Ok(record.into())
}

fn session_credentials(
    registry: &hosts::Registry,
    tokens: &dyn keychain::TokenStore,
    id: &str,
) -> Result<SessionCredentials, String> {
    let record = registry.get(id)?;
    if record.kind != hosts::HostKind::Https {
        return Err("SSH host has no direct session credentials until its tunnel is ready".into());
    }
    let base_url = hosts::normalise_https_url(&record.https_base_url).map_err(|_| {
        "refusing to return a Keychain token for a host without an HTTPS base URL".to_string()
    })?;
    Ok(SessionCredentials {
        base_url,
        token: tokens.get(id)?.unwrap_or_default(),
    })
}

fn has_host_token(
    registry: &hosts::Registry,
    tokens: &dyn keychain::TokenStore,
    id: &str,
) -> Result<bool, String> {
    registry.get(id)?;
    Ok(tokens.get(id)?.is_some_and(|token| !token.is_empty()))
}

fn remove_host(
    registry: &hosts::Registry,
    tokens: &dyn keychain::TokenStore,
    id: &str,
) -> Result<(), String> {
    registry.get(id)?;
    let previous_token = tokens.get(id)?;
    tokens.delete(id)?;
    if let Err(error) = registry.remove(id) {
        let rollback = match previous_token {
            Some(token) => tokens.set(id, &token),
            None => Ok(()),
        };
        return match rollback {
            Ok(()) => Err(error),
            Err(rollback_error) => Err(format!(
                "{error}; additionally failed to restore host token: {rollback_error}"
            )),
        };
    }
    Ok(())
}

/// Headless seam used only by the destructive alpha vertical after the Desktop
/// process has exited. It exercises the same registry/keychain transaction as
/// `host_remove` without inventing a second JSON writer.
///
/// The path and id are intentionally pinned to the mktemp layout created by
/// product-alpha-e2e.sh, so this hidden helper cannot be pointed at a user's
/// real Desktop registry.
fn alpha_smoke_hosts_file(app_data: &std::path::Path) -> Result<std::path::PathBuf, String> {
    #[cfg(not(unix))]
    {
        let _ = app_data;
        return Err("alpha smoke host removal is supported only on Unix".into());
    }
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;

        let root = app_data
            .parent()
            .ok_or_else(|| "alpha smoke app-data has no root".to_string())?;
        let sentinel = root.join(".tariboy-alpha-smoke-root");
        let canonical_root = std::fs::canonicalize(root)
            .map_err(|error| format!("resolve alpha smoke root: {error}"))?;
        let canonical_app_data = std::fs::canonicalize(app_data)
            .map_err(|error| format!("resolve alpha smoke app-data: {error}"))?;
        let canonical_hosts = std::fs::canonicalize(app_data.join("hosts.json"))
            .map_err(|error| format!("resolve alpha smoke hosts: {error}"))?;
        let root_metadata = std::fs::symlink_metadata(root)
            .map_err(|error| format!("inspect alpha smoke root: {error}"))?;
        let app_metadata = std::fs::symlink_metadata(app_data)
            .map_err(|error| format!("inspect alpha smoke app-data: {error}"))?;
        let hosts_metadata = std::fs::symlink_metadata(&canonical_hosts)
            .map_err(|error| format!("inspect alpha smoke hosts: {error}"))?;
        let sentinel_metadata = std::fs::symlink_metadata(&sentinel)
            .map_err(|error| format!("inspect alpha smoke sentinel: {error}"))?;
        let sentinel_value = std::fs::read_to_string(&sentinel)
            .map_err(|error| format!("read alpha smoke sentinel: {error}"))?;

        let valid = app_data.is_absolute()
            && canonical_root
                .file_name()
                .and_then(|value| value.to_str())
                .is_some_and(|value| value.starts_with("tariboy-product-alpha."))
            && canonical_app_data == canonical_root.join("app-data")
            && canonical_hosts == canonical_app_data.join("hosts.json")
            && root_metadata.is_dir()
            && !root_metadata.file_type().is_symlink()
            && app_metadata.is_dir()
            && !app_metadata.file_type().is_symlink()
            && hosts_metadata.is_file()
            && !hosts_metadata.file_type().is_symlink()
            && sentinel_metadata.is_file()
            && !sentinel_metadata.file_type().is_symlink()
            && root_metadata.permissions().mode() & 0o777 == 0o700
            && app_metadata.permissions().mode() & 0o777 == 0o700
            && hosts_metadata.permissions().mode() & 0o777 == 0o600
            && sentinel_metadata.permissions().mode() & 0o777 == 0o600
            && sentinel_value.trim() == "TARIBOY_ALPHA_SMOKE_ROOT";
        if !valid {
            return Err(format!(
                "alpha smoke host removal requires its isolated app-data path \
                 (root={}, app_data={}, root_mode={:o}, app_mode={:o}, hosts_mode={:o}, sentinel_mode={:o})",
                canonical_root.display(),
                canonical_app_data.display(),
                root_metadata.permissions().mode() & 0o777,
                app_metadata.permissions().mode() & 0o777,
                hosts_metadata.permissions().mode() & 0o777,
                sentinel_metadata.permissions().mode() & 0o777,
            ));
        }
        Ok(canonical_hosts)
    }
}

pub(crate) fn alpha_smoke_remove_host(app_data: &std::path::Path, id: &str) -> Result<(), String> {
    if id != "alpha-e2e" {
        return Err("alpha smoke host removal requires its isolated id".into());
    }
    let hosts_file = alpha_smoke_hosts_file(app_data)?;
    remove_host(
        &hosts::Registry::new(hosts_file),
        keychain::system().as_ref(),
        id,
    )
}

#[tauri::command]
pub fn hosts_list(state: State<AppState>) -> Result<Vec<hosts::HostView>, String> {
    list_host_views(&state.hosts, &state.host_runtime)
}

#[tauri::command]
pub fn host_save_ssh(
    state: State<AppState>,
    input: hosts::SaveSshInput,
) -> Result<hosts::HostView, String> {
    if let Some(id) = input.id.as_deref() {
        state.tunnels.cancel(id);
        state.ssh_operations.cancel_host(id);
        state.host_runtime.remove(id);
    }
    let record = state.hosts.save_ssh(input)?;
    state.tunnels.cancel(&record.id);
    state.host_runtime.remove(&record.id);
    Ok(state.host_runtime.view(record))
}

#[tauri::command]
pub fn host_save_https(
    state: State<AppState>,
    input: hosts::SaveHttpsInput,
    token: Option<String>,
) -> Result<hosts::HostView, String> {
    if let Some(id) = input.id.as_deref() {
        state.tunnels.cancel(id);
        state.ssh_operations.cancel_host(id);
        state.host_runtime.remove(id);
    }
    let view = save_https_host(&state.hosts, state.tokens.as_ref(), input, token)?;
    state.tunnels.cancel(&view.id);
    state.host_runtime.remove(&view.id);
    Ok(view)
}

#[tauri::command]
pub fn host_session_credentials(
    state: State<AppState>,
    id: String,
) -> Result<SessionCredentials, String> {
    session_credentials(&state.hosts, state.tokens.as_ref(), &id)
}

#[tauri::command]
pub fn host_has_token(state: State<AppState>, id: String) -> Result<bool, String> {
    has_host_token(&state.hosts, state.tokens.as_ref(), &id)
}

#[tauri::command]
pub fn host_remove(state: State<AppState>, id: String) -> Result<(), String> {
    state.tunnels.cancel(&id);
    state.ssh_operations.cancel_host(&id);
    state.host_runtime.remove(&id);
    remove_host(&state.hosts, state.tokens.as_ref(), &id)?;
    state.tunnels.cancel(&id);
    Ok(())
}

fn start_host_tunnel(app: &AppHandle, host: hosts::HostRecord) {
    let host_id = host.id.clone();
    let expected_alias = host.ssh_alias.clone();
    let expected_port = host.remote_port;
    let handle = app.clone();
    let sink: tunnel::EventSink = Arc::new(move |event| {
        let state = handle.state::<AppState>();
        if let Some(status) = &event.status {
            if let Err(error) = state
                .hosts
                .set_last_daemon_version(&event.host_id, &status.version)
            {
                state
                    .host_runtime
                    .set_error(&event.host_id, "host_registry_failed", &error);
                emit_host_state(&handle, &event.host_id);
                return;
            }
        }
        state.host_runtime.set_tunnel(&event);
        emit_host_state(&handle, &event.host_id);
    });
    let registry_handle = app.clone();
    let expected_id = host_id.clone();
    let state = app.state::<AppState>();
    state.tunnels.start_if(
        tunnel::Config {
            host_id,
            alias: host.ssh_alias,
            remote_port: host.remote_port,
        },
        sink,
        move || {
            registry_handle
                .state::<AppState>()
                .hosts
                .get(&expected_id)
                .is_ok_and(|current| {
                    current.kind == hosts::HostKind::Ssh
                        && current.ssh_alias == expected_alias
                        && current.remote_port == expected_port
                })
        },
    );
}

#[tauri::command]
pub fn host_connect(app: AppHandle, id: String) -> Result<(), String> {
    let state = app.state::<AppState>();
    let host = state.hosts.get(&id)?;
    if host.kind != hosts::HostKind::Ssh {
        return Err("only SSH hosts use native tunnels".into());
    }
    start_host_tunnel(&app, host);
    Ok(())
}

fn active_remote_work(port: u16) -> Result<Vec<ActiveWork>, String> {
    let result = remote_health::get_result(port, "/api/agents", Duration::from_secs(2))?;
    let agents = result
        .get("agents")
        .and_then(serde_json::Value::as_array)
        .ok_or_else(|| "remote /api/agents response has no agents array".to_string())?;
    let mut active = Vec::new();
    for agent in agents {
        let name = agent
            .get("name")
            .and_then(serde_json::Value::as_str)
            .unwrap_or("");
        if matches!(name, "" | "." | "..")
            || !name
                .bytes()
                .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'_' | b'-' | b'.'))
        {
            return Err("remote agent response contains an unsafe name".into());
        }
        let state = agent
            .get("state")
            .and_then(serde_json::Value::as_str)
            .unwrap_or("unknown");
        let iterations = remote_health::get_result(
            port,
            &format!("/api/agents/{name}/iterations"),
            Duration::from_secs(2),
        )?;
        let running_iterations = iterations
            .get("iterations")
            .and_then(serde_json::Value::as_array)
            .into_iter()
            .flatten()
            .filter(|iteration| {
                iteration.get("status").and_then(serde_json::Value::as_str) == Some("running")
            })
            .filter_map(|iteration| {
                iteration
                    .get("id")
                    .and_then(serde_json::Value::as_str)
                    .map(str::to_string)
            })
            .collect::<Vec<_>>();
        if state != "stopped" || !running_iterations.is_empty() {
            active.push(ActiveWork {
                agent: name.into(),
                state: state.into(),
                running_iterations,
            });
        }
    }
    Ok(active)
}

#[tauri::command]
pub fn host_update(app: AppHandle, id: String) -> Result<OperationResult, String> {
    let state = app.state::<AppState>();
    let host = state.hosts.get(&id)?;
    if host.kind != hosts::HostKind::Ssh {
        return Err("only SSH hosts can be updated".into());
    }
    let (local_port, running_version) = state
        .host_runtime
        .healthy_tunnel(&id)
        .ok_or_else(|| "host must have a healthy tunnel before update".to_string())?;
    let linux_bundle = state
        .bundle
        .platform(bundle::Platform::LinuxX86_64)
        .map_err(|error| format!("remote bundle is not ready: {error}"))?;
    let operation = state.ssh_operations.begin(&id)?;
    let operation_id = operation.id.clone();
    let operations = state.ssh_operations.clone();
    let control_dir = state.paths.ssh_control_dir();
    let handle = app.clone();
    std::thread::spawn(move || {
        let sink = host_operation_sink(&handle, &operations, &operation);
        let result = (|| {
            let (preflight, master) = establish_host_preflight(
                &ssh::Transport::system(),
                &operation,
                &host,
                &control_dir,
                sink.clone(),
            )?;
            if !preflight.install_supported {
                return Err(ssh::SshError {
                    code: "unsupported_host".into(),
                    message: "remote host no longer satisfies install prerequisites".into(),
                });
            }
            let staged = provision::stage(
                &ssh::Transport::system(),
                &operation,
                &host.ssh_alias,
                master.socket(),
                &linux_bundle,
                sink.clone(),
            )?;
            remote_restart_handoff_preflight(&running_version, &staged.previous, || {
                active_remote_work(local_port)
            })
            .map_err(|message| ssh::SshError {
                code: "legacy_active_work".into(),
                message,
            })?;
            handle.state::<AppState>().tunnels.cancel(&host.id);
            let installed = provision::activate_and_restart(
                &ssh::Transport::system(),
                &operation,
                &host.ssh_alias,
                master.socket(),
                provision::VersionSwitch {
                    next: &linux_bundle.version,
                    previous: &staged.previous,
                },
                host.remote_port,
                sink,
            )?;
            Ok(installed)
        })();
        match result {
            Ok(installed) => {
                let applied = operations.if_live(&operation.id, || {
                    let state = handle.state::<AppState>();
                    state.host_runtime.set_provisioned(
                        &operation.host_id,
                        installed.version,
                        false,
                        String::new(),
                    );
                });
                if applied {
                    emit_host_state(&handle, &operation.host_id);
                    start_host_tunnel(&handle, host.clone());
                }
            }
            Err(error) => {
                let applied = operations.if_live(&operation.id, || {
                    handle.state::<AppState>().host_runtime.set_error(
                        &operation.host_id,
                        &error.code,
                        &error.message,
                    );
                });
                if applied {
                    emit_host_state(&handle, &operation.host_id);
                    emit_host_output(
                        &handle,
                        ssh::OutputEvent {
                            operation_id: operation.id.clone(),
                            host_id: operation.host_id.clone(),
                            stream: "error".into(),
                            text: serde_json::json!({
                                "code": error.code,
                                "message": error.message,
                            })
                            .to_string(),
                            prompt: None,
                        },
                    );
                    start_host_tunnel(&handle, host.clone());
                }
            }
        }
        operations.finish(&operation.id);
    });
    Ok(OperationResult { operation_id })
}

pub fn connect_saved_hosts(app: &AppHandle) {
    let hosts = {
        let state = app.state::<AppState>();
        state.hosts.list().unwrap_or_default()
    };
    for host in hosts {
        if host.kind == hosts::HostKind::Ssh {
            start_host_tunnel(app, host);
        }
    }
}

fn emit_host_output(app: &AppHandle, event: ssh::OutputEvent) {
    let _ = app.emit(ssh::OUTPUT_EVENT, event);
}

fn emit_host_state(app: &AppHandle, host_id: &str) {
    let state = app.state::<AppState>();
    if let Ok(record) = state.hosts.get(host_id) {
        let view = state.host_runtime.view(record);
        let code = support::diagnostic_error_code(&view.message);
        support::append_desktop_event(
            &state.paths.app_data.join("desktop.log"),
            "host",
            host_id,
            support::diagnostic_host_state(&view.state),
            code.as_deref(),
        );
        let _ = app.emit(hosts::STATE_EVENT, view);
    }
}

fn host_operation_sink(
    handle: &AppHandle,
    operations: &ssh::Operations,
    operation: &ssh::Operation,
) -> ssh::OutputSink {
    let handle = handle.clone();
    let operations = operations.clone();
    let operation_id = operation.id.clone();
    Arc::new(move |event| {
        let applied = operations.if_live(&operation_id, || {
            if event.stream == "phase" {
                let state = handle.state::<AppState>();
                state.host_runtime.set_phase(&event.host_id, &event.text);
            } else if event.prompt.is_some() {
                let state = handle.state::<AppState>();
                state.host_runtime.set_error(
                    &event.host_id,
                    "needs_auth",
                    "SSH authentication reply required",
                );
            }
        });
        if !applied {
            return;
        }
        if event.stream == "phase" || event.prompt.is_some() {
            emit_host_state(&handle, &event.host_id);
        }
        emit_host_output(&handle, event);
    })
}

#[cfg(test)]
fn run_host_preflight(
    transport: &ssh::Transport,
    operation: &ssh::Operation,
    host: &hosts::HostRecord,
    control_dir: &std::path::Path,
    sink: ssh::OutputSink,
) -> Result<preflight::Result, ssh::SshError> {
    let (result, _master) =
        establish_host_preflight(transport, operation, host, control_dir, sink)?;
    Ok(result)
}

fn establish_host_preflight(
    transport: &ssh::Transport,
    operation: &ssh::Operation,
    host: &hosts::HostRecord,
    control_dir: &std::path::Path,
    sink: ssh::OutputSink,
) -> Result<(preflight::Result, ssh::MasterSession), ssh::SshError> {
    let phase = |name: &str| {
        sink(ssh::OutputEvent {
            operation_id: operation.id.clone(),
            host_id: operation.host_id.clone(),
            stream: "phase".into(),
            text: name.to_string(),
            prompt: None,
        });
    };
    phase("resolve");
    transport.resolve(&host.ssh_alias, Duration::from_secs(10), &operation.cancel)?;
    phase("authenticate");
    let socket = ssh::control_socket(control_dir, &host.id).map_err(|message| ssh::SshError {
        code: "ssh_failed".into(),
        message,
    })?;
    let master = transport.authenticate(
        operation,
        &host.ssh_alias,
        &socket,
        Duration::from_secs(120),
        sink.clone(),
    )?;
    phase("preflight");
    let result = preflight::run(
        transport,
        operation,
        &host.ssh_alias,
        master.socket(),
        Duration::from_secs(20),
        sink.clone(),
    )?;
    sink(ssh::OutputEvent {
        operation_id: operation.id.clone(),
        host_id: operation.host_id.clone(),
        stream: "stdout".into(),
        text: serde_json::to_string(&result).unwrap_or_default(),
        prompt: None,
    });
    Ok((result, master))
}

fn run_host_install(
    transport: &ssh::Transport,
    operation: &ssh::Operation,
    host: &hosts::HostRecord,
    control_dir: &std::path::Path,
    bundle: &bundle::PlatformBundle,
    sink: ssh::OutputSink,
) -> Result<(preflight::Result, provision::Result), ssh::SshError> {
    let (preflight, master) =
        establish_host_preflight(transport, operation, host, control_dir, sink.clone())?;
    if !preflight.install_supported {
        return Err(ssh::SshError {
            code: "unsupported_host".into(),
            message: format!(
                "remote installation requires Linux x86_64 and writable ~/.local (found {}/{})",
                preflight.platform, preflight.arch
            ),
        });
    }
    let installed = provision::run(
        transport,
        operation,
        &host.ssh_alias,
        master.socket(),
        bundle,
        host.remote_port,
        sink,
    )?;
    Ok((preflight, installed))
}

#[tauri::command]
pub fn host_provision(app: AppHandle, id: String) -> Result<OperationResult, String> {
    let state = app.state::<AppState>();
    let host = state.hosts.get(&id)?;
    if host.kind != hosts::HostKind::Ssh {
        return Err("only SSH hosts can be provisioned".into());
    }
    let linux_bundle = state
        .bundle
        .platform(bundle::Platform::LinuxX86_64)
        .map_err(|error| format!("remote bundle is not ready: {error}"))?;
    let operation = state.ssh_operations.begin(&id)?;
    let operation_id = operation.id.clone();
    let operations = state.ssh_operations.clone();
    let control_dir = state.paths.ssh_control_dir();
    let handle = app.clone();
    std::thread::spawn(move || {
        let sink = host_operation_sink(&handle, &operations, &operation);
        match run_host_install(
            &ssh::Transport::system(),
            &operation,
            &host,
            &control_dir,
            &linux_bundle,
            sink,
        ) {
            Ok((preflight, installed)) => {
                let should_connect = installed.state == "ready";
                let applied = operations.if_live(&operation.id, || {
                    let state = handle.state::<AppState>();
                    state.host_runtime.set_preflight(
                        &operation.host_id,
                        preflight.platform,
                        preflight.arch,
                        preflight.prerequisites,
                        preflight.install_supported,
                    );
                    state.host_runtime.set_provisioned(
                        &operation.host_id,
                        installed.version,
                        installed.state == "degraded",
                        installed.message,
                    );
                });
                if applied {
                    emit_host_state(&handle, &operation.host_id);
                    if should_connect {
                        start_host_tunnel(&handle, host.clone());
                    }
                }
            }
            Err(error) => {
                let applied = operations.if_live(&operation.id, || {
                    let state = handle.state::<AppState>();
                    state
                        .host_runtime
                        .set_error(&operation.host_id, &error.code, &error.message);
                });
                if applied {
                    emit_host_state(&handle, &operation.host_id);
                    emit_host_output(
                        &handle,
                        ssh::OutputEvent {
                            operation_id: operation.id.clone(),
                            host_id: operation.host_id.clone(),
                            stream: "stderr".into(),
                            text: format!("{}: {}", error.code, error.message),
                            prompt: None,
                        },
                    );
                }
            }
        }
        operations.finish(&operation.id);
    });
    Ok(OperationResult { operation_id })
}

#[tauri::command]
pub fn host_prompt_reply(
    state: State<AppState>,
    operation_id: String,
    text: String,
) -> Result<(), String> {
    state.ssh_operations.reply(&operation_id, text)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::keychain::TokenStore;
    use std::io::{Read, Write};
    use std::net::{Ipv4Addr, TcpListener};

    struct FailingKeychain;

    #[test]
    fn vscode_local_uri_encodes_the_absolute_folder_path() {
        let dir = tempfile::tempdir().unwrap();
        let registry = hosts::Registry::new(dir.path().join("hosts.json"));

        assert_eq!(
            vscode_folder_uri(&registry, "", "/Users/alice/Agent #1").unwrap(),
            "vscode://file/Users/alice/Agent%20%231"
        );
    }

    #[test]
    fn vscode_ssh_uri_uses_the_saved_alias_and_encodes_uri_components() {
        let dir = tempfile::tempdir().unwrap();
        let registry = hosts::Registry::new(dir.path().join("hosts.json"));
        let host = registry
            .save_ssh(hosts::SaveSshInput {
                id: None,
                label: "GPU".into(),
                ssh_alias: "gpu box+prod".into(),
                remote_install_dir: String::new(),
                remote_port: 0,
            })
            .unwrap();

        assert_eq!(
            vscode_folder_uri(&registry, &host.id, "/srv/Agent #1").unwrap(),
            "vscode://vscode-remote/ssh-remote+gpu%20box%2Bprod/srv/Agent%20%231"
        );
    }

    #[test]
    fn vscode_uri_rejects_https_unknown_and_non_absolute_targets() {
        let dir = tempfile::tempdir().unwrap();
        let registry = hosts::Registry::new(dir.path().join("hosts.json"));
        let https = registry
            .save_https(hosts::SaveHttpsInput {
                id: None,
                label: "API".into(),
                https_base_url: "https://agents.example".into(),
            })
            .unwrap();

        assert!(vscode_folder_uri(&registry, &https.id, "/srv/agent")
            .unwrap_err()
            .contains("SSH"));
        assert!(vscode_folder_uri(&registry, "missing", "/srv/agent").is_err());
        assert!(vscode_folder_uri(&registry, "", "relative/path")
            .unwrap_err()
            .contains("absolute"));
    }

    /// Every terminal text the boundary must refuse. `//example.test`, `/tmp/a`,
    /// `./a` and `example.test` have no scheme at all; the rest carry a scheme
    /// that is not web. `%zz` is the malformed percent input.
    const REJECTED_EXTERNAL_URLS: &[&str] = &[
        "file:///tmp/a",
        "javascript:alert(1)",
        "data:text/plain,x",
        "custom:value",
        "//example.test",
        "/tmp/a",
        "./a",
        "example.test",
        "%zz",
        "http://%zz/",
    ];

    #[test]
    fn external_url_allows_only_http_and_https() {
        for accepted in [
            "http://example.test/a",
            "https://example.test/a?token=secret",
        ] {
            let opened = std::cell::RefCell::new(Vec::new());
            open_external_url_with(
                |url| {
                    opened.borrow_mut().push(url.to_string());
                    Ok(())
                },
                accepted,
                None,
            )
            .unwrap();
            assert_eq!(&*opened.borrow(), &[accepted.to_string()]);
        }

        // Collected, not asserted row by row: a bare assert! would unwind on the
        // first offender and hide every input after it.
        let mut leaked = Vec::new();
        for rejected in REJECTED_EXTERNAL_URLS {
            let opened = std::cell::RefCell::new(Vec::new());
            let result = open_external_url_with(
                |url| {
                    opened.borrow_mut().push(url.to_string());
                    Ok(())
                },
                rejected,
                None,
            );
            if result.is_ok() {
                leaked.push(format!("{rejected}: returned Ok"));
            }
            if !opened.borrow().is_empty() {
                leaked.push(format!("{rejected}: reached the opener"));
            }
        }
        assert!(leaked.is_empty(), "not rejected: {leaked:?}");
    }

    #[test]
    fn external_url_rejection_never_echoes_the_input_or_its_query() {
        let mut leaked = Vec::new();
        for rejected in REJECTED_EXTERNAL_URLS {
            let message = match open_external_url_with(|_| Ok(()), rejected, None) {
                Ok(()) => continue,
                Err(message) => message,
            };
            if message != "invalid external web link" {
                leaked.push(format!("{rejected}: {message}"));
            }
        }
        let message = open_external_url_with(
            |_| Err("boom".to_string()),
            "https://example.test/a?token=secret",
            None,
        )
        .expect_err("a failing opener must surface an error");
        if message != "cannot open external web link" {
            leaked.push(format!("opener failure: {message}"));
        }
        assert!(leaked.is_empty(), "leaky error text: {leaked:?}");
    }

    #[test]
    fn external_url_open_failure_hides_the_url_the_opener_reported() {
        let message = open_external_url_with(
            |url| Err(format!("spawn open {url}: No such file or directory")),
            "https://example.test/a?token=secret",
            None,
        )
        .expect_err("a failing opener must surface an error");

        assert_eq!(message, "cannot open external web link");
        assert!(!message.contains("example.test"), "leaked host: {message}");
        assert!(!message.contains("token=secret"), "leaked query: {message}");
    }

    #[test]
    fn external_url_records_accepted_opens_for_the_isolated_e2e_observer() {
        use std::os::unix::fs::PermissionsExt;

        let dir = tempfile::tempdir().unwrap();
        let log = dir.path().join("external-open.log");
        let opened = std::cell::RefCell::new(Vec::new());
        let record = |raw: &str| {
            open_external_url_with(
                |url| {
                    opened.borrow_mut().push(url.to_string());
                    Ok(())
                },
                raw,
                Some(log.as_path()),
            )
        };

        record("https://example.test/a?token=secret").unwrap();
        record("http://example.test/a").unwrap();
        assert!(record("file:///tmp/a").is_err());

        assert_eq!(
            std::fs::read_to_string(&log).unwrap(),
            "https://example.test/a?token=secret\nhttp://example.test/a\n"
        );
        // The observation path must replace the system opener, never precede it:
        // an E2E run may not launch a real browser.
        assert!(opened.borrow().is_empty(), "opened: {:?}", opened.borrow());
        let mode = std::fs::metadata(&log).unwrap().permissions().mode() & 0o777;
        assert_eq!(mode, 0o600, "observation log mode {mode:o}");
    }

    impl TokenStore for FailingKeychain {
        fn set(&self, _account: &str, _token: &str) -> Result<(), String> {
            Err("Keychain locked".into())
        }

        fn get(&self, _account: &str) -> Result<Option<String>, String> {
            Ok(None)
        }

        fn delete(&self, _account: &str) -> Result<(), String> {
            Err("Keychain locked".into())
        }
    }

    struct FailingDeleteKeychain {
        token: String,
    }

    impl TokenStore for FailingDeleteKeychain {
        fn set(&self, _account: &str, _token: &str) -> Result<(), String> {
            Ok(())
        }

        fn get(&self, _account: &str) -> Result<Option<String>, String> {
            Ok(Some(self.token.clone()))
        }

        fn delete(&self, _account: &str) -> Result<(), String> {
            Err("Keychain locked".into())
        }
    }

    #[test]
    fn install_result_serialises_for_the_ui() {
        let r = InstallResult {
            outcome: "occupied".into(),
            link: "/home/u/.local/bin/tariboy".into(),
            target: "/App/tariboy".into(),
            existing: "regular file at /home/u/.local/bin/tariboy".into(),
        };
        let j = serde_json::to_value(&r).unwrap();
        assert_eq!(j["outcome"], "occupied");
        assert_eq!(j["link"], "/home/u/.local/bin/tariboy");
        assert_eq!(
            j["existing"],
            "regular file at /home/u/.local/bin/tariboy"
        );
    }

    #[test]
    fn outcomes_map_to_stable_ui_strings() {
        assert_eq!(
            outcome_name(&crate::cli_install::Outcome::Created),
            "created"
        );
        assert_eq!(
            outcome_name(&crate::cli_install::Outcome::AlreadyInstalled),
            "already-installed"
        );
        assert_eq!(
            outcome_name(&crate::cli_install::Outcome::Occupied {
                existing: String::new()
            }),
            "occupied"
        );
    }

    #[test]
    fn install_update_restarts_for_created_and_already_installed() {
        for outcome in [
            crate::cli_install::Outcome::Created,
            crate::cli_install::Outcome::AlreadyInstalled,
        ] {
            let restarts = std::cell::Cell::new(0);
            let got = install_then_restart(
                || Ok(outcome),
                || {
                    restarts.set(restarts.get() + 1);
                    Ok(())
                },
            )
            .unwrap();
            assert!(!matches!(got, crate::cli_install::Outcome::Occupied { .. }));
            assert_eq!(restarts.get(), 1);
        }
    }

    #[test]
    fn install_update_does_not_restart_for_occupied_or_install_error() {
        let restarts = std::cell::Cell::new(0);
        let got = install_then_restart(
            || {
                Ok(crate::cli_install::Outcome::Occupied {
                    existing: "regular file at /tmp/tariboy".into(),
                })
            },
            || {
                restarts.set(restarts.get() + 1);
                Ok(())
            },
        )
        .unwrap();
        assert!(matches!(got, crate::cli_install::Outcome::Occupied { .. }));
        assert_eq!(restarts.get(), 0);

        let error = install_then_restart(
            || Err(std::io::Error::other("disk full")),
            || {
                restarts.set(restarts.get() + 1);
                Ok(())
            },
        )
        .unwrap_err();
        assert!(error.contains("disk full"));
        assert_eq!(restarts.get(), 0);
    }

    #[test]
    fn install_update_distinguishes_restart_failure_after_link_success() {
        let error = install_then_restart(
            || Ok(crate::cli_install::Outcome::Created),
            || Err("new daemon did not become ready".into()),
        )
        .unwrap_err();
        assert!(error.contains("CLI binaries are installed"));
        assert!(error.contains("new daemon did not become ready"));
    }

    fn ready_daemon(version: &str) -> DaemonView {
        DaemonView {
            state: Phase::Ready,
            base_url: "http://127.0.0.1:43123".into(),
            daemon_version: version.into(),
            app_version: "0.11.0".into(),
            base_dir: "/tmp/tariboy".into(),
            pid: 123,
            adopted: true,
            message: String::new(),
        }
    }

    #[test]
    fn legacy_restart_preflight_blocks_active_work() {
        let probes = std::cell::Cell::new(0);
        let error = restart_handoff_preflight(&ready_daemon("0.10.0"), |port| {
            probes.set(probes.get() + 1);
            assert_eq!(port, 43123);
            Ok(vec![ActiveWork {
                agent: "researcher".into(),
                state: "running".into(),
                running_iterations: vec!["iter-7".into()],
            }])
        })
        .unwrap_err();

        assert_eq!(probes.get(), 1);
        assert!(error.contains("0.10.0"));
        assert!(error.contains("researcher"));
        assert!(error.contains("iter-7"));
        assert!(error.contains("retry"));
    }

    #[test]
    fn legacy_restart_preflight_allows_idle_daemon() {
        let probes = std::cell::Cell::new(0);
        restart_handoff_preflight(&ready_daemon("0.10.0"), |port| {
            probes.set(probes.get() + 1);
            assert_eq!(port, 43123);
            Ok(Vec::new())
        })
        .unwrap();
        assert_eq!(probes.get(), 1);
    }

    #[test]
    fn restart_preflight_skips_probe_for_handoff_capable_daemon() {
        for version in ["0.10.1", "0.11.0", "1.0.0", "v1.2.3"] {
            restart_handoff_preflight(&ready_daemon(version), |_| {
                panic!("handoff-capable daemon must not need an active-work probe")
            })
            .unwrap();
        }
    }

    #[test]
    fn legacy_restart_preflight_fails_closed_when_work_cannot_be_checked() {
        let error = restart_handoff_preflight(&ready_daemon("not-a-version"), |_| {
            Err("connection reset".into())
        })
        .unwrap_err();
        assert!(error.contains("cannot safely restart"));
        assert!(error.contains("connection reset"));

        let mut view = ready_daemon("0.10.0");
        view.base_url = "http://localhost:not-a-port".into();
        let error = restart_handoff_preflight(&view, |_| Ok(Vec::new())).unwrap_err();
        assert!(error.contains("loopback HTTP address"));
    }

    #[test]
    fn remote_restart_handoff_preflight_skips_probe_for_safe_versions() {
        for version in ["0.10.1", "0.16.1", "v1.2.3"] {
            remote_restart_handoff_preflight(version, "0.10.0", || {
                panic!("new or handoff-capable remote daemon must not need an active-work probe")
            })
            .unwrap();
        }
    }

    #[test]
    fn remote_restart_handoff_preflight_allows_idle_legacy_daemon() {
        let probes = std::cell::Cell::new(0);
        remote_restart_handoff_preflight("0.10.0", "0.19.0", || {
            probes.set(probes.get() + 1);
            Ok(Vec::new())
        })
        .unwrap();
        assert_eq!(probes.get(), 1);
    }

    #[test]
    fn remote_restart_handoff_preflight_blocks_active_legacy_daemon() {
        let error = remote_restart_handoff_preflight("0.10.0", "0.19.0", || {
            Ok(vec![ActiveWork {
                agent: "researcher".into(),
                state: "running".into(),
                running_iterations: vec!["iter-7".into()],
            }])
        })
        .unwrap_err();

        assert!(error.contains("0.10.0"));
        assert!(error.contains("researcher"));
        assert!(error.contains("iter-7"));
        assert!(error.contains("retry"));
    }

    #[test]
    fn remote_restart_handoff_preflight_uses_running_version_when_links_are_absent() {
        let error = remote_restart_handoff_preflight("0.10.0", "", || {
            Ok(vec![ActiveWork {
                agent: "researcher".into(),
                state: "running".into(),
                running_iterations: Vec::new(),
            }])
        })
        .unwrap_err();

        assert!(error.contains("0.10.0"));
        assert!(error.contains("researcher"));
    }

    #[test]
    fn remote_restart_handoff_preflight_uses_running_version_when_links_are_legacy() {
        remote_restart_handoff_preflight("0.10.1", "0.10.0", || {
            panic!("handoff-capable running daemon must not be classified by older links")
        })
        .unwrap();
    }

    #[test]
    fn remote_restart_handoff_preflight_fails_closed_when_legacy_probe_fails() {
        let error = remote_restart_handoff_preflight("0.10.0", "0.19.0", || {
            Err("remote API timed out".into())
        })
        .unwrap_err();

        assert!(error.contains("0.10.0"));
        assert!(error.contains("could not be checked"));
        assert!(error.contains("remote API timed out"));
    }

    #[test]
    fn https_host_credentials_never_enter_registry_json_or_views() {
        let dir = tempfile::tempdir().unwrap();
        let registry = hosts::Registry::new(dir.path().join("hosts.json"));
        let tokens = keychain::MemoryKeychain::default();
        let view = save_https_host(
            &registry,
            &tokens,
            hosts::SaveHttpsInput {
                id: None,
                label: "prod".into(),
                https_base_url: "https://prod.internal/".into(),
            },
            Some("super-secret".into()),
        )
        .unwrap();

        let json = std::fs::read_to_string(registry.path()).unwrap();
        assert!(!json.contains("super-secret"));
        assert!(!serde_json::to_string(&view)
            .unwrap()
            .contains("super-secret"));
        let credentials = session_credentials(&registry, &tokens, &view.id).unwrap();
        assert_eq!(credentials.base_url, "https://prod.internal");
        assert_eq!(credentials.token, "super-secret");
        assert!(has_host_token(&registry, &tokens, &view.id).unwrap());
    }

    #[test]
    fn legacy_cleartext_host_never_receives_keychain_token() {
        let dir = tempfile::tempdir().unwrap();
        let registry = hosts::Registry::new(dir.path().join("hosts.json"));
        let tokens = keychain::MemoryKeychain::default();
        let view = save_https_host(
            &registry,
            &tokens,
            hosts::SaveHttpsInput {
                id: None,
                label: "legacy".into(),
                https_base_url: "https://legacy.internal".into(),
            },
            Some("must-not-leak".into()),
        )
        .unwrap();
        let json = std::fs::read_to_string(registry.path())
            .unwrap()
            .replace("https://legacy.internal", "http://legacy.internal");
        std::fs::write(registry.path(), json).unwrap();

        let error = session_credentials(&registry, &tokens, &view.id).unwrap_err();
        assert!(error.contains("HTTPS"), "{error}");
        assert_eq!(
            tokens.get(&view.id).unwrap().as_deref(),
            Some("must-not-leak")
        );
    }

    #[test]
    fn removing_host_removes_its_keychain_token() {
        let dir = tempfile::tempdir().unwrap();
        let registry = hosts::Registry::new(dir.path().join("hosts.json"));
        let tokens = keychain::MemoryKeychain::default();
        let view = save_https_host(
            &registry,
            &tokens,
            hosts::SaveHttpsInput {
                id: None,
                label: "prod".into(),
                https_base_url: "https://prod.internal".into(),
            },
            Some("secret".into()),
        )
        .unwrap();

        remove_host(&registry, &tokens, &view.id).unwrap();
        assert!(registry.list().unwrap().is_empty());
        assert_eq!(tokens.get(&view.id).unwrap(), None);
    }

    #[test]
    fn alpha_smoke_host_removal_rejects_nonisolated_targets() {
        for path in [
            std::path::Path::new("/Users/alice/Library/Application Support/app.tariboy.desktop"),
            std::path::Path::new("tariboy-product-alpha.fake/app-data"),
            std::path::Path::new("/tmp/not-alpha/app-data"),
        ] {
            assert!(alpha_smoke_hosts_file(path).is_err(), "{path:?}");
        }
    }

    #[cfg(unix)]
    #[test]
    fn alpha_smoke_host_removal_accepts_only_sentinel_owner_only_tree() {
        use std::os::unix::fs::PermissionsExt;

        let root = tempfile::Builder::new()
            .prefix("tariboy-product-alpha.")
            .tempdir()
            .unwrap();
        std::fs::set_permissions(root.path(), std::fs::Permissions::from_mode(0o700)).unwrap();
        let app_data = root.path().join("app-data");
        std::fs::create_dir(&app_data).unwrap();
        std::fs::set_permissions(&app_data, std::fs::Permissions::from_mode(0o700)).unwrap();
        std::fs::write(
            app_data.join("hosts.json"),
            b"{\"schema_version\":1,\"hosts\":[]}",
        )
        .unwrap();
        std::fs::set_permissions(
            app_data.join("hosts.json"),
            std::fs::Permissions::from_mode(0o600),
        )
        .unwrap();
        std::fs::write(
            root.path().join(".tariboy-alpha-smoke-root"),
            b"TARIBOY_ALPHA_SMOKE_ROOT\n",
        )
        .unwrap();
        std::fs::set_permissions(
            root.path().join(".tariboy-alpha-smoke-root"),
            std::fs::Permissions::from_mode(0o600),
        )
        .unwrap();

        assert_eq!(
            alpha_smoke_hosts_file(&app_data).unwrap(),
            std::fs::canonicalize(app_data.join("hosts.json")).unwrap()
        );
        assert!(alpha_smoke_remove_host(&app_data, "another-host").is_err());
    }

    #[test]
    fn failed_keychain_write_rolls_back_new_host_metadata() {
        let dir = tempfile::tempdir().unwrap();
        let registry = hosts::Registry::new(dir.path().join("hosts.json"));
        let error = save_https_host(
            &registry,
            &FailingKeychain,
            hosts::SaveHttpsInput {
                id: None,
                label: "prod".into(),
                https_base_url: "https://prod.internal".into(),
            },
            Some("secret".into()),
        )
        .unwrap_err();

        assert!(error.contains("Keychain locked"), "{error}");
        assert!(registry.list().unwrap().is_empty());
    }

    #[test]
    fn failed_keychain_delete_leaves_host_metadata_intact() {
        let dir = tempfile::tempdir().unwrap();
        let registry = hosts::Registry::new(dir.path().join("hosts.json"));
        let record = registry
            .save_https(hosts::SaveHttpsInput {
                id: None,
                label: "prod".into(),
                https_base_url: "https://prod.internal".into(),
            })
            .unwrap();
        let tokens = FailingDeleteKeychain {
            token: "secret".into(),
        };

        let error = remove_host(&registry, &tokens, &record.id).unwrap_err();

        assert!(error.contains("Keychain locked"), "{error}");
        assert_eq!(registry.get(&record.id).unwrap(), record);
    }

    #[test]
    fn host_preflight_resolves_authenticates_then_runs_fixed_probe() {
        let dir = tempfile::tempdir().unwrap();
        let log = dir.path().join("ssh-argv.log");
        let preflight_stdin = dir.path().join("preflight.sh");
        let json = r#"{"platform":"Linux","arch":"x86_64","home":"/home/u","free_disk_kb":1000,"writable_local":true,"tmux":{"available":true,"version":"tmux 3"},"flock":{"available":true,"version":"flock 2"},"claude":{"available":true,"version":"1"},"codex":{"available":false,"version":""},"opencode":{"available":false,"version":""}}"#;
        let behavior = format!(
            r#"if [ "${{1:-}}" = "-G" ]; then
  printf 'hostname prod\nproxyjump jump\nidentityfile ~/.ssh/id_ed25519\n'
  exit 0
fi
for arg in "$@"; do
  if [ "$arg" = "-M" ]; then
    socket=
    while [ "$#" -gt 0 ]; do
      if [ "$1" = "-S" ]; then shift; socket="$1"; fi
      shift || true
    done
    : > "$socket"
    exec sleep 30
  fi
done
cat > '{preflight_stdin}'
printf '%s\n' '{json}'"#,
            preflight_stdin = preflight_stdin.display(),
        );
        let ssh_path = crate::testbin::executable(
            dir.path(),
            "ssh",
            &crate::testbin::argv_logger(&log, &behavior),
        );
        let transport = ssh::Transport::new(ssh::Binaries {
            ssh: ssh_path,
            scp: dir.path().join("scp"),
        });
        let operations = ssh::Operations::default();
        let operation = operations.begin("host-id").unwrap();
        let events = Arc::new(std::sync::Mutex::new(Vec::new()));
        let capture = events.clone();
        let sink: ssh::OutputSink = Arc::new(move |event| capture.lock().unwrap().push(event));
        let host = hosts::HostRecord {
            id: "host-id".into(),
            label: "prod".into(),
            kind: hosts::HostKind::Ssh,
            ssh_alias: "prod alias".into(),
            remote_install_dir: "~/.local/lib/tariboy".into(),
            remote_port: 9990,
            https_base_url: String::new(),
            last_daemon_version: String::new(),
        };

        let result = run_host_preflight(&transport, &operation, &host, dir.path(), sink).unwrap();

        assert!(result.install_supported);
        let calls = crate::testbin::invocations(&log);
        assert_eq!(calls[0], vec!["-G", "prod alias"]);
        assert!(calls[1].iter().any(|arg| arg == "-M"));
        assert_eq!(
            calls[2],
            vec![
                "-S",
                dir.path().join("host-id").to_str().unwrap(),
                "prod alias",
                "sh",
                "-s"
            ]
        );
        assert_eq!(
            std::fs::read_to_string(preflight_stdin).unwrap(),
            preflight::SCRIPT
        );
        let argv = calls.concat().join(" ");
        assert!(!argv.contains("StrictHostKeyChecking"));
        assert!(!argv.contains("IdentityFile"));
        assert!(!argv.contains("ProxyJump"));
        let phases: Vec<_> = events
            .lock()
            .unwrap()
            .iter()
            .filter(|event| event.stream == "phase")
            .map(|event| event.text.clone())
            .collect();
        assert_eq!(phases, vec!["resolve", "authenticate", "preflight"]);
    }

    #[test]
    fn active_work_guard_returns_agents_and_running_iterations() {
        let listener = TcpListener::bind((Ipv4Addr::LOCALHOST, 0)).unwrap();
        let port = listener.local_addr().unwrap().port();
        let server = std::thread::spawn(move || {
            for _ in 0..3 {
                let (mut stream, _) = listener.accept().unwrap();
                let mut request = [0u8; 4096];
                let n = stream.read(&mut request).unwrap();
                let request = String::from_utf8_lossy(&request[..n]);
                let body = if request.starts_with("GET /api/agents HTTP/1.0") {
                    r#"{"ok":true,"result":{"agents":[{"name":"worker","state":"running"},{"name":"idle","state":"stopped"}]}}"#
                } else if request.starts_with("GET /api/agents/worker/iterations HTTP/1.0") {
                    r#"{"ok":true,"result":{"iterations":[{"id":"it-1","status":"running"},{"id":"it-0","status":"done"}]}}"#
                } else {
                    r#"{"ok":true,"result":{"iterations":[]}}"#
                };
                write!(
                    stream,
                    "HTTP/1.0 200 OK\r\nContent-Length: {}\r\n\r\n{}",
                    body.len(),
                    body
                )
                .unwrap();
            }
        });

        let active = active_remote_work(port).unwrap();

        assert_eq!(active.len(), 1);
        assert_eq!(active[0].agent, "worker");
        assert_eq!(active[0].running_iterations, ["it-1"]);
        server.join().unwrap();
    }
}
