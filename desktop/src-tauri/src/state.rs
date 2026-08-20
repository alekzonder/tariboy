//! The lifecycle state the webview renders, and the app-wide handle that owns it.
//!
//! Every failure mode the app can hit is a VALUE here rather than a silent
//! no-op: the banner is the only place a user learns why nothing is happening.

use crate::{bundle, daemon, hosts, keychain, paths, ssh, support, tunnel};
use serde::Serialize;
use std::path::Path;
use std::process::Child;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Duration;
use tauri::menu::MenuItem;
use tauri::Wry;
use tauri::{AppHandle, Emitter, Manager};

/// The event the webview subscribes to (`ui/src/lib/desktop.ts`).
pub const STATE_EVENT: &str = "daemon://state";

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum Phase {
    /// Spawned (or about to be) and not answering yet.
    Starting,
    /// Answering on the control socket.
    Ready,
    /// A start attempt failed; `message` says how.
    Failed,
    /// It was up and is not any more.
    Down,
}

#[derive(Debug, Clone, Serialize)]
pub struct DaemonView {
    pub state: Phase,
    /// http://host:port of the daemon, or "" when it has no TCP listener.
    pub base_url: String,
    /// Version of the RUNNING daemon ("" when unknown).
    pub daemon_version: String,
    /// Version of the binaries this app bundles.
    pub app_version: String,
    pub base_dir: String,
    pub pid: i64,
    pub adopted: bool,
    pub message: String,
}

impl DaemonView {
    pub fn starting(app_version: &str) -> Self {
        Self {
            state: Phase::Starting,
            base_url: String::new(),
            daemon_version: String::new(),
            app_version: app_version.to_string(),
            base_dir: String::new(),
            pid: 0,
            adopted: false,
            message: String::new(),
        }
    }

    pub fn ready(st: &daemon::Status, adopted: bool, app_version: &str) -> Self {
        Self {
            state: Phase::Ready,
            base_url: if st.http_addr.is_empty() {
                String::new()
            } else {
                format!("http://{}", st.http_addr)
            },
            daemon_version: st.version.clone(),
            app_version: app_version.to_string(),
            base_dir: st.base_dir.clone(),
            pid: st.pid,
            adopted,
            message: String::new(),
        }
    }

    pub fn failed(message: &str, app_version: &str) -> Self {
        Self {
            message: message.to_string(),
            state: Phase::Failed,
            ..Self::starting(app_version)
        }
    }

    pub fn down(app_version: &str) -> Self {
        Self {
            state: Phase::Down,
            ..Self::starting(app_version)
        }
    }

    /// A one-line summary for the menu-bar status item.
    pub fn menu_label(&self) -> String {
        match self.state {
            Phase::Starting => "Daemon: starting…".into(),
            Phase::Ready if self.base_url.is_empty() => "Daemon: running (no HTTP listener)".into(),
            Phase::Ready => format!("Daemon: running on {}", self.base_url),
            Phase::Failed => "Daemon: failed to start".into(),
            Phase::Down => "Daemon: not running".into(),
        }
    }
}

/// AppState is Tauri-managed and shared by the commands, the tray and setup.
pub struct AppState {
    // Not read yet: setup copies the socket/pidfile/log paths into `cfg`, so the
    // resolved Paths are held for the things that need the dirs themselves
    // (revealing the data dir, the remote-host work in a later sub-project).
    #[allow(dead_code)]
    pub paths: paths::Paths,
    pub bundle: bundle::Bundle,
    pub cfg: daemon::Config,
    pub hosts: hosts::Registry,
    pub tokens: Arc<dyn keychain::TokenStore>,
    pub ssh_operations: ssh::Operations,
    pub host_runtime: Arc<hosts::RuntimeHosts>,
    pub tunnels: Arc<tunnel::Supervisor>,
    inner: Mutex<Inner>,
    /// Serialises menu-bar writes on its own, so `inner` never has to be held
    /// across a tauri call. See `set`.
    menu_lock: Mutex<()>,
    /// Identifies the daemon child we currently believe in. See
    /// `next_generation` and `exit_view`.
    generation: AtomicU64,
}

struct Inner {
    view: DaemonView,
    /// The disabled menu-bar line that mirrors `view`. Held so a transition can
    /// refresh it; None until the tray is built.
    status_item: Option<MenuItem<Wry>>,
}

impl AppState {
    pub fn new(paths: paths::Paths, bundle: bundle::Bundle, cfg: daemon::Config) -> Self {
        let view = DaemonView::starting(bundle.version());
        let hosts = hosts::Registry::new(paths.hosts_file());
        Self {
            paths,
            bundle,
            cfg,
            hosts,
            tokens: keychain::system(),
            ssh_operations: ssh::Operations::default(),
            host_runtime: Arc::new(hosts::RuntimeHosts::default()),
            tunnels: Arc::new(tunnel::Supervisor::default()),
            inner: Mutex::new(Inner {
                view,
                status_item: None,
            }),
            menu_lock: Mutex::new(()),
            generation: AtomicU64::new(0),
        }
    }

    pub fn set_status_item(&self, item: MenuItem<Wry>) {
        self.inner.lock().unwrap().status_item = Some(item);
    }

    /// generation is the id of the daemon child the app currently believes in.
    pub fn generation(&self) -> u64 {
        self.generation.load(Ordering::SeqCst)
    }

    /// next_generation retires every watcher registered so far and returns the id
    /// the next child carries.
    ///
    /// Call it when a new child is about to be watched, and when we are about to
    /// kill the current one on purpose (stop, restart): in both cases the old
    /// child's exit no longer says anything about what is running, and the caller
    /// publishes the real outcome itself.
    pub fn next_generation(&self) -> u64 {
        self.generation.fetch_add(1, Ordering::SeqCst) + 1
    }

    pub fn view(&self) -> DaemonView {
        self.inner.lock().unwrap().view.clone()
    }

    /// set publishes a new state and emits it. Emitting here (rather than at each
    /// call site) is why the banner cannot miss a transition.
    ///
    /// `inner` is NEVER held across a tauri call. `MenuItem::set_text` expands to
    /// tauri's `run_item_main_thread!`: from a background thread it posts a task
    /// to the event loop and blocks until the loop pumps it. Holding `inner`
    /// there would let a busy event loop pin the mutex, and any thread that then
    /// touched the state — including the event loop itself, inside a command —
    /// would deadlock against it. So the handle is cloned out (it is a cheap
    /// refcounted handle) and the guard is dropped first.
    pub fn set(&self, app: &AppHandle, view: DaemonView) {
        let item = {
            let mut g = self.inner.lock().unwrap();
            g.view = view.clone();
            g.status_item.clone()
        };
        if let Some(item) = item {
            // menu_lock, not `inner`: serialising the menu writes on a dedicated
            // lock keeps the invariant the old code got from holding `inner` —
            // the menu-bar line can never disagree with the banner — because the
            // label is re-read from the CURRENT view under this lock, so a
            // transition that lands while we wait wins rather than being undone.
            let _serialise = self.menu_lock.lock().unwrap();
            let _ = item.set_text(self.view().menu_label());
        }
        let diagnostic_state = match view.state {
            Phase::Starting => "starting",
            Phase::Ready => "ready",
            Phase::Failed => "failed",
            Phase::Down => "down",
        };
        let code = support::diagnostic_error_code(&view.message);
        support::append_desktop_event(
            &self.paths.app_data.join("desktop.log"),
            "daemon",
            "local",
            diagnostic_state,
            code.as_deref(),
        );
        let _ = app.emit(STATE_EVENT, view);
    }
}

/// exit_view decides what a watched child's exit means, or None when nothing
/// should be published. It is the whole of `watch_daemon`'s judgement, kept pure
/// so the rules are testable without a Tauri runtime.
///
/// Two guards, because "the child I watched exited" does NOT imply "no daemon is
/// running":
///
///  1. Generation. A newer child (or a deliberate stop) has superseded this
///     watcher, so its corpse says nothing about what is running now. Publishing
///     `down` here would stamp it over a live daemon, permanently, since nothing
///     re-probes afterwards.
///  2. A re-probe of the control socket. `ensure_up` can hand back a child that
///     is ALREADY dead: it probes Down, spawns, and the Go single-instance guard
///     (internal/daemon/daemon.go) makes the new process exit at once because
///     another daemon took the socket in between — yet `wait_ready` then gets an
///     immediate Ok from that other daemon. The watcher would fire on a corpse
///     while a healthy daemon answers. So we ask the socket instead of trusting
///     the assumption, which makes the state self-correcting rather than merely
///     less wrong.
pub fn exit_view(
    watched_generation: u64,
    current_generation: u64,
    socket: &Path,
    app_version: &str,
) -> Option<DaemonView> {
    if watched_generation != current_generation {
        return None;
    }
    match daemon::probe(socket, Duration::from_secs(1)) {
        // Something still answers: adopted, not down. We did not start whatever
        // is running, so `adopted` is the honest flag.
        daemon::Probe::Up(st) => Some(DaemonView::ready(
            &st.unwrap_or_default(),
            true,
            app_version,
        )),
        daemon::Probe::Down => Some(DaemonView::down(app_version)),
    }
}

/// watch_daemon hands a daemon WE started to `daemon::watch_exit`, which reaps it
/// the instant it dies and publishes whatever `exit_view` decides.
///
/// `generation` is the id this child was registered under (from
/// `AppState::next_generation`); a later bump retires this watcher.
///
/// Push, not polling: the same `wait()` that stops the child becoming a zombie
/// also tells us the daemon is gone, so the UI learns immediately instead of on
/// the next status tick. Reaping is mandatory regardless — `kill(pid, 0)`
/// succeeds on a zombie, so an un-reaped child makes `daemon::stop` burn its
/// entire escalation budget and always SIGKILL. A daemon we merely ADOPTED is
/// not ours to watch; the SPA's status poll covers that case.
///
/// The closure captures only the AppHandle (Clone + Send + 'static) and looks the
/// state up when it fires, which avoids threading an Arc through Tauri's managed
/// state.
pub fn watch_daemon(app: AppHandle, child: Child, generation: u64) {
    crate::daemon::watch_exit(child, move || {
        let st = app.state::<AppState>();
        if let Some(view) = exit_view(
            generation,
            st.generation(),
            &st.cfg.socket,
            st.bundle.version(),
        ) {
            st.set(&app, view);
        }
    });
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn view_serialises_with_the_field_names_the_ui_expects() {
        let v = DaemonView {
            state: Phase::Ready,
            base_url: "http://127.0.0.1:9990".into(),
            daemon_version: "1.0.0".into(),
            app_version: "1.0.0".into(),
            base_dir: "/base".into(),
            pid: 7,
            adopted: true,
            message: String::new(),
        };
        let j = serde_json::to_value(&v).unwrap();
        assert_eq!(j["state"], "ready");
        assert_eq!(j["base_url"], "http://127.0.0.1:9990");
        assert_eq!(j["daemon_version"], "1.0.0");
        assert_eq!(j["app_version"], "1.0.0");
        assert_eq!(j["adopted"], true);
        assert_eq!(j["pid"], 7);
    }

    #[test]
    fn every_phase_serialises_lowercase() {
        for (p, want) in [
            (Phase::Starting, "starting"),
            (Phase::Ready, "ready"),
            (Phase::Failed, "failed"),
            (Phase::Down, "down"),
        ] {
            assert_eq!(serde_json::to_value(p).unwrap(), want);
        }
    }

    // base_url is derived from the daemon's own http_addr, so an adopted daemon
    // with no listener yields "" and the UI shows the "no HTTP listener" banner.
    #[test]
    fn ready_from_status_builds_the_base_url() {
        let st = crate::daemon::Status {
            version: "1.0.0".into(),
            pid: 9,
            base_dir: "/base".into(),
            http_addr: "127.0.0.1:9993".into(),
        };
        let v = DaemonView::ready(&st, true, "1.0.0");
        assert_eq!(v.base_url, "http://127.0.0.1:9993");
        assert_eq!(v.state, Phase::Ready);
        assert!(v.adopted);

        let none = crate::daemon::Status {
            http_addr: String::new(),
            ..st
        };
        assert_eq!(DaemonView::ready(&none, true, "1.0.0").base_url, "");
    }

    #[test]
    fn failed_carries_the_message_and_no_base_url() {
        let v = DaemonView::failed("exec format error", "1.0.0");
        assert_eq!(v.state, Phase::Failed);
        assert_eq!(v.message, "exec format error");
        assert_eq!(v.base_url, "");
        assert_eq!(v.app_version, "1.0.0");
    }

    fn state_for(dir: &Path) -> AppState {
        AppState::new(
            paths::Paths {
                base: dir.to_path_buf(),
                runtime: dir.to_path_buf(),
                app_data: dir.to_path_buf(),
            },
            bundle::Bundle::new(dir.join("bin")),
            daemon::Config {
                socket: dir.join("tariboyd.sock"),
                pid_file: dir.join("tariboyd.pid"),
                log_file: dir.join("tariboyd.log"),
                runtime_dir: dir.to_path_buf(),
                daemon_bin: dir.join("tariboyd"),
                ready_timeout: Duration::from_millis(500),
                poll_interval: Duration::from_millis(25),
            },
        )
    }

    // The generation is what tells a watcher whether the child it holds is still
    // the one the app believes in.
    #[test]
    fn next_generation_is_monotonic_and_retires_the_previous_id() {
        let dir = tempfile::tempdir().unwrap();
        let st = state_for(dir.path());
        assert_eq!(st.generation(), 0);

        let first = st.next_generation();
        assert_eq!(first, 1);
        assert_eq!(st.generation(), first);

        let second = st.next_generation();
        assert_eq!(second, 2);
        assert_ne!(first, st.generation(), "the first id must now be stale");
    }

    // The core of the stale-watcher bug: a child from an older generation exiting
    // must publish NOTHING. Before this, its `down` was stamped over a live
    // daemon and nothing ever re-probed to undo it.
    #[test]
    fn a_stale_watcher_publishes_nothing() {
        let dir = tempfile::tempdir().unwrap();
        // No listener at all: if the generation guard were missing, the probe
        // would say Down and this would be Some(down).
        let v = exit_view(1, 2, &dir.path().join("tariboyd.sock"), "1.0.0");
        assert!(
            v.is_none(),
            "a superseded watcher must stay quiet, got {v:?}"
        );
    }

    // Self-correction: ensure_up can hand back a child that is already dead (the
    // Go single-instance guard kills it) while another daemon answers the socket.
    // The exit must then read as `ready`/adopted, never `down`.
    #[test]
    fn a_live_socket_turns_an_exit_into_ready_not_down() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("tariboyd.sock");
        crate::daemon::tests::fake_daemon(sock.clone(), crate::daemon::tests::OK_BODY);

        let v = exit_view(1, 1, &sock, "1.0.0").expect("a matching generation must publish");
        assert_eq!(
            v.state,
            Phase::Ready,
            "a daemon still answers; view = {v:?}"
        );
        assert!(v.adopted, "we did not start whatever is answering now");
        assert_eq!(v.daemon_version, "1.2.3");
    }

    // The plain case still works: our child died and nothing answers.
    #[test]
    fn a_dead_socket_publishes_down() {
        let dir = tempfile::tempdir().unwrap();
        let v = exit_view(3, 3, &dir.path().join("tariboyd.sock"), "1.0.0")
            .expect("a matching generation must publish");
        assert_eq!(v.state, Phase::Down);
    }

    // End to end through the real watcher plumbing, minus the AppHandle: a child
    // really exits, and the decision the watcher would take is the stale one.
    #[test]
    fn a_watcher_retired_before_its_child_dies_stays_quiet() {
        let dir = tempfile::tempdir().unwrap();
        let st = state_for(dir.path());
        let watched = st.next_generation();

        let child = std::process::Command::new("/bin/sh")
            .arg("-c")
            .arg("exit 0")
            .spawn()
            .expect("spawn short-lived child");

        let (tx, rx) = std::sync::mpsc::channel();
        crate::daemon::watch_exit(child, move || {
            let _ = tx.send(());
        });
        rx.recv_timeout(Duration::from_secs(5))
            .expect("the child must be reaped");

        // A restart happened while that child was dying.
        st.next_generation();
        assert!(
            exit_view(
                watched,
                st.generation(),
                &st.cfg.socket,
                st.bundle.version()
            )
            .is_none(),
            "the exit of a superseded child must not touch the published state"
        );
    }
}
