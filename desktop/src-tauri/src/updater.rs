use semver::Version;
use serde::Serialize;
use std::sync::Mutex;
use std::time::Duration;
use tauri::{AppHandle, Emitter, State};
use tauri_plugin_updater::{Update, UpdaterExt};

pub const STATE_EVENT: &str = "desktop://update-state";
const NETWORK_TIMEOUT: Duration = Duration::from_secs(30);
const UNAVAILABLE: &str = "Desktop updates are unavailable on this platform.";
const CHECK_FAILED: &str = "Desktop update check failed. Try again.";
const INVALID_VERSION: &str = "The update catalog did not provide a newer stable version.";
const DOWNLOAD_FAILED: &str =
    "Desktop update download or signature verification failed. Try again.";
const NO_PACKAGE: &str = "No verified Desktop update is ready to install.";
const INSTALL_FAILED: &str =
    "Desktop update installation failed. The verified package is ready to retry.";
const INSTALL_TASK_FAILED: &str =
    "Desktop update installation failed. Download it again to retry.";

#[derive(Clone, Debug, Serialize)]
pub struct UpdateView {
    revision: u64,
    current_version: String,
    phase: String,
    version: String,
    downloaded_bytes: u64,
    total_bytes: Option<u64>,
    error: String,
}

struct UpdateInner<P> {
    view: UpdateView,
    package: Option<P>,
}

pub struct UpdateState<P> {
    inner: Mutex<UpdateInner<P>>,
}

pub type DesktopUpdateState = UpdateState<(Update, Vec<u8>)>;

impl<P> UpdateState<P> {
    pub fn new(current_version: impl Into<String>) -> Self {
        Self {
            inner: Mutex::new(UpdateInner {
                view: UpdateView {
                    revision: 0,
                    current_version: current_version.into(),
                    phase: "idle".into(),
                    version: String::new(),
                    downloaded_bytes: 0,
                    total_bytes: None,
                    error: String::new(),
                },
                package: None,
            }),
        }
    }

    fn view(&self) -> UpdateView {
        self.inner.lock().unwrap().view.clone()
    }

    fn change(&self, f: impl FnOnce(&mut UpdateInner<P>)) -> UpdateView {
        let mut inner = self.inner.lock().unwrap();
        f(&mut inner);
        inner.view.revision = inner
            .view
            .revision
            .checked_add(1)
            .expect("desktop update revision overflow");
        inner.view.clone()
    }

    fn begin_check(&self) -> Option<UpdateView> {
        let mut inner = self.inner.lock().unwrap();
        if matches!(
            inner.view.phase.as_str(),
            "checking" | "downloading" | "ready" | "installing"
        ) {
            return None;
        }
        inner.package = None;
        inner.view.phase = "checking".into();
        inner.view.version.clear();
        inner.view.downloaded_bytes = 0;
        inner.view.total_bytes = None;
        inner.view.error.clear();
        inner.view.revision = inner
            .view
            .revision
            .checked_add(1)
            .expect("desktop update revision overflow");
        Some(inner.view.clone())
    }

    fn fail(&self, message: &'static str) -> UpdateView {
        self.change(|inner| {
            inner.package = None;
            inner.view.phase = "error".into();
            inner.view.error = message.into();
        })
    }

    fn up_to_date(&self) -> UpdateView {
        self.change(|inner| {
            inner.package = None;
            inner.view.phase = "up-to-date".into();
            inner.view.version.clear();
            inner.view.downloaded_bytes = 0;
            inner.view.total_bytes = None;
            inner.view.error.clear();
        })
    }

    fn begin_download(&self, version: &str) -> UpdateView {
        self.change(|inner| {
            inner.view.phase = "downloading".into();
            inner.view.version = version.into();
            inner.view.downloaded_bytes = 0;
            inner.view.total_bytes = None;
            inner.view.error.clear();
        })
    }

    fn progress(&self, bytes: usize, total: Option<u64>) -> Option<UpdateView> {
        if self.inner.lock().unwrap().view.phase != "downloading" {
            return None;
        }
        Some(self.change(|inner| {
            inner.view.downloaded_bytes = inner.view.downloaded_bytes.saturating_add(bytes as u64);
            inner.view.total_bytes = total;
        }))
    }

    fn finish_download(&self, version: &str, result: Result<P, ()>) -> UpdateView {
        match result {
            Ok(package) => self.change(|inner| {
                inner.package = Some(package);
                inner.view.phase = "ready".into();
                inner.view.version = version.into();
                inner.view.error.clear();
            }),
            Err(()) => self.fail(DOWNLOAD_FAILED),
        }
    }

    fn begin_install(&self) -> Result<(P, UpdateView), UpdateView> {
        let mut inner = self.inner.lock().unwrap();
        if inner.view.phase == "ready" {
            if let Some(package) = inner.package.take() {
                inner.view.phase = "installing".into();
                inner.view.error.clear();
                inner.view.revision = inner
                    .view
                    .revision
                    .checked_add(1)
                    .expect("desktop update revision overflow");
                return Ok((package, inner.view.clone()));
            }
        }
        if !matches!(
            inner.view.phase.as_str(),
            "checking" | "downloading" | "installing"
        ) {
            inner.view.phase = "error".into();
        }
        inner.view.error = NO_PACKAGE.into();
        inner.view.revision = inner
            .view
            .revision
            .checked_add(1)
            .expect("desktop update revision overflow");
        Err(inner.view.clone())
    }

    fn finish_install(&self, package: P, installed: bool) -> (UpdateView, bool) {
        if installed {
            return (self.view(), true);
        }
        let view = self.change(|inner| {
            inner.package = Some(package);
            inner.view.phase = "ready".into();
            inner.view.error = INSTALL_FAILED.into();
        });
        (view, false)
    }

    #[cfg(test)]
    fn has_package(&self) -> bool {
        self.inner.lock().unwrap().package.is_some()
    }
}

fn publish(app: &AppHandle, view: UpdateView) -> UpdateView {
    let _ = app.emit(STATE_EVENT, view.clone());
    view
}

fn is_newer_stable(current: &str, candidate: &str) -> bool {
    match (Version::parse(current), Version::parse(candidate)) {
        (Ok(current), Ok(candidate)) => {
            candidate.pre.is_empty() && candidate.cmp_precedence(&current).is_gt()
        }
        _ => false,
    }
}

fn supported_platform() -> bool {
    cfg!(all(target_os = "macos", target_arch = "aarch64"))
}

#[tauri::command]
pub fn desktop_update_state(state: State<DesktopUpdateState>) -> UpdateView {
    state.view()
}

#[tauri::command]
pub async fn desktop_update_download(
    app: AppHandle,
    state: State<'_, DesktopUpdateState>,
) -> Result<UpdateView, String> {
    let Some(checking) = state.begin_check() else {
        return Ok(state.view());
    };
    publish(&app, checking);

    if !supported_platform() {
        return Ok(publish(&app, state.fail(UNAVAILABLE)));
    }

    let updater = match app
        .updater_builder()
        .timeout(NETWORK_TIMEOUT)
        .version_comparator(|current, release| {
            release.version.pre.is_empty()
                && release.version.cmp_precedence(&current).is_gt()
        })
        .build()
    {
        Ok(updater) => updater,
        Err(_) => return Ok(publish(&app, state.fail(CHECK_FAILED))),
    };
    let mut update = match updater.check().await {
        Ok(Some(update)) => update,
        Ok(None) => return Ok(publish(&app, state.up_to_date())),
        Err(_) => return Ok(publish(&app, state.fail(CHECK_FAILED))),
    };
    if !is_newer_stable(&update.current_version, &update.version) {
        return Ok(publish(&app, state.fail(INVALID_VERSION)));
    }
    update.timeout = Some(NETWORK_TIMEOUT);

    let version = update.version.clone();
    publish(&app, state.begin_download(&version));
    Ok(
        match update
            .download(
                |bytes, total| {
                    if let Some(view) = state.progress(bytes, total) {
                        publish(&app, view);
                    }
                },
                || {},
            )
            .await
        {
            Ok(bytes) => publish(&app, state.finish_download(&version, Ok((update, bytes)))),
            Err(_) => publish(&app, state.finish_download(&version, Err(()))),
        },
    )
}

#[tauri::command]
pub async fn desktop_update_install(
    app: AppHandle,
    state: State<'_, DesktopUpdateState>,
) -> Result<UpdateView, String> {
    if !supported_platform() {
        return Ok(publish(&app, state.fail(UNAVAILABLE)));
    }
    let (package, installing) = match state.begin_install() {
        Ok(started) => started,
        Err(view) => return Ok(publish(&app, view)),
    };
    publish(&app, installing);

    let task = tauri::async_runtime::spawn_blocking(move || {
        let (update, bytes) = package;
        let installed = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            update.install(&bytes)
        }))
        .is_ok_and(|result| result.is_ok());
        ((update, bytes), installed)
    })
    .await;

    Ok(match task {
        Ok((package, installed)) => {
            let (view, restart) = state.finish_install(package, installed);
            if restart {
                app.restart();
            }
            publish(&app, view)
        }
        Err(_) => publish(&app, state.fail(INSTALL_TASK_FAILED)),
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Mutex;

    fn state(phase: &str, package: Option<&'static str>) -> UpdateState<&'static str> {
        UpdateState {
            inner: Mutex::new(UpdateInner {
                view: UpdateView {
                    revision: 7,
                    current_version: "1.0.0".into(),
                    phase: phase.into(),
                    version: "2.0.0".into(),
                    downloaded_bytes: 42,
                    total_bytes: Some(42),
                    error: String::new(),
                },
                package,
            }),
        }
    }

    #[test]
    fn install_without_a_verified_package_is_rejected() {
        let state = UpdateState::<&str>::new("1.0.0");

        let view = state.begin_install().unwrap_err();

        assert_eq!(view.phase, "error");
        assert_eq!(view.error, NO_PACKAGE);
        assert!(!state.has_package());
    }

    #[test]
    fn repeated_check_does_not_replace_busy_or_ready_state() {
        for (phase, package) in [
            ("checking", None),
            ("downloading", None),
            ("ready", Some("verified package")),
            ("installing", None),
        ] {
            let state = state(phase, package);

            assert!(state.begin_check().is_none(), "phase {phase}");
            assert_eq!(state.view().phase, phase);
            assert_eq!(state.has_package(), package.is_some());
            assert_eq!(state.view().revision, 7);
        }
    }

    #[test]
    fn failed_download_or_verification_cannot_become_ready() {
        let state = state("downloading", None);

        let view = state.finish_download("2.0.0", Err(()));

        assert_eq!(view.phase, "error");
        assert_eq!(view.error, DOWNLOAD_FAILED);
        assert!(!state.has_package());
    }

    #[test]
    fn failed_install_restores_the_verified_package_for_retry() {
        let state = state("ready", Some("verified package"));
        let (package, _) = state.begin_install().unwrap();

        let (view, restart) = state.finish_install(package, false);

        assert_eq!(view.phase, "ready");
        assert_eq!(view.error, INSTALL_FAILED);
        assert!(!restart);
        assert!(state.begin_install().is_ok());
    }

    #[test]
    fn successful_install_is_the_only_restart_path() {
        let failed = state("ready", Some("verified package"));
        let (package, _) = failed.begin_install().unwrap();
        assert!(!failed.finish_install(package, false).1);

        let succeeded = state("ready", Some("verified package"));
        let (package, _) = succeeded.begin_install().unwrap();
        assert!(succeeded.finish_install(package, true).1);
        assert!(!succeeded.has_package());
    }

    #[test]
    fn only_higher_stable_versions_are_accepted() {
        assert!(is_newer_stable("1.0.0", "1.0.1"));
        assert!(!is_newer_stable("1.0.0", "1.0.0"));
        assert!(!is_newer_stable("1.0.0", "1.0.0+build.1"));
        assert!(!is_newer_stable("1.0.0+build.1", "1.0.0+build.2"));
        assert!(!is_newer_stable("1.0.0", "0.9.9"));
        assert!(!is_newer_stable("1.0.0", "1.1.0-beta.1"));
        assert!(!is_newer_stable("invalid", "1.0.1"));
    }
}
