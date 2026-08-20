#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn activation_rejects_empty_or_oversized_identifiers() {
        assert!(validate_activation(TaskNotificationActivation {
            host_id: "remote".into(),
            notification_id: "".into(),
            task_key: "ASK-1".into(),
        })
        .is_err());
        assert!(validate_activation(TaskNotificationActivation {
            host_id: "x".repeat(513),
            notification_id: "tn_1".into(),
            task_key: "ASK-1".into(),
        })
        .is_err());
    }

    #[test]
    fn activation_accepts_local_and_remote_host_identity() {
        for host_id in ["", "remote-1"] {
            assert!(validate_activation(TaskNotificationActivation {
                host_id: host_id.into(),
                notification_id: "tn_1".into(),
                task_key: "ASK-1".into(),
            })
            .is_ok());
        }
    }

    #[test]
    fn notification_copy_trims_bounded_display_fields() {
        let prepared = prepare_notification(TaskNotificationInput {
            host_id: "remote-1".into(),
            notification_id: "tn_1".into(),
            task_key: "ASK-1".into(),
            server_label: "  production  ".into(),
            agent_name: "  alice  ".into(),
        })
        .expect("valid notification");

        assert_eq!(prepared.title, "alice needs your answer");
        assert_eq!(prepared.body, "ASK-1 on production");
        assert_eq!(
            prepared.activation,
            TaskNotificationActivation {
                host_id: "remote-1".into(),
                notification_id: "tn_1".into(),
                task_key: "ASK-1".into(),
            }
        );
    }

    #[test]
    fn notification_rejects_nul_and_unbounded_display_fields() {
        let valid = TaskNotificationInput {
            host_id: "".into(),
            notification_id: "tn_1".into(),
            task_key: "ASK-1".into(),
            server_label: "local".into(),
            agent_name: "alice".into(),
        };

        assert!(prepare_notification(TaskNotificationInput {
            task_key: "ASK\0-1".into(),
            ..valid.clone()
        })
        .is_err());
        assert!(prepare_notification(TaskNotificationInput {
            server_label: " ".into(),
            ..valid.clone()
        })
        .is_err());
        assert!(prepare_notification(TaskNotificationInput {
            agent_name: "x".repeat(513),
            ..valid
        })
        .is_err());
    }

    #[test]
    fn test_activation_requires_debug_and_exact_opt_in() {
        assert!(test_activation_allowed(true, Some("1")));
        assert!(!test_activation_allowed(false, Some("1")));
        assert!(!test_activation_allowed(true, Some("true")));
        assert!(!test_activation_allowed(true, None));
    }
}

use serde::{Deserialize, Serialize};
use tauri::{AppHandle, Emitter};

const ACTIVATION_EVENT: &str = "desktop://task-notification-activated";
const MAX_FIELD_BYTES: usize = 512;

#[derive(Clone, Debug, Deserialize)]
pub struct TaskNotificationInput {
    pub host_id: String,
    pub notification_id: String,
    pub task_key: String,
    pub server_label: String,
    pub agent_name: String,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
pub struct TaskNotificationActivation {
    pub host_id: String,
    pub notification_id: String,
    pub task_key: String,
}

#[derive(Debug, Serialize)]
pub struct TaskNotificationResult {
    outcome: &'static str,
}

#[derive(Debug)]
struct PreparedNotification {
    #[cfg(target_os = "macos")]
    native_id: String,
    title: String,
    body: String,
    activation: TaskNotificationActivation,
}

fn validate_identifier(name: &str, value: &str, allow_empty: bool) -> Result<(), String> {
    if !allow_empty && value.is_empty() {
        return Err(format!("{name} is required"));
    }
    if value.len() > MAX_FIELD_BYTES {
        return Err(format!("{name} exceeds {MAX_FIELD_BYTES} bytes"));
    }
    if value.contains('\0') {
        return Err(format!("{name} contains NUL"));
    }
    Ok(())
}

fn validate_display(name: &str, value: &str) -> Result<String, String> {
    let value = value.trim();
    validate_identifier(name, value, false)?;
    Ok(value.to_string())
}

fn validate_activation(
    input: TaskNotificationActivation,
) -> Result<TaskNotificationActivation, String> {
    validate_identifier("host_id", &input.host_id, true)?;
    validate_identifier("notification_id", &input.notification_id, false)?;
    validate_identifier("task_key", &input.task_key, false)?;
    Ok(input)
}

fn prepare_notification(input: TaskNotificationInput) -> Result<PreparedNotification, String> {
    let activation = validate_activation(TaskNotificationActivation {
        host_id: input.host_id,
        notification_id: input.notification_id,
        task_key: input.task_key,
    })?;
    let server_label = validate_display("server_label", &input.server_label)?;
    let agent_name = validate_display("agent_name", &input.agent_name)?;
    #[cfg(target_os = "macos")]
    let native_id = serde_json::to_string(&(
        activation.host_id.as_str(),
        activation.notification_id.as_str(),
    ))
    .map_err(|error| format!("encode notification identity: {error}"))?;

    Ok(PreparedNotification {
        #[cfg(target_os = "macos")]
        native_id,
        title: format!("{agent_name} needs your answer"),
        body: format!("{} on {server_label}", activation.task_key),
        activation,
    })
}

impl TaskNotificationResult {
    fn shown() -> Self {
        Self { outcome: "shown" }
    }

    #[cfg(target_os = "macos")]
    fn denied() -> Self {
        Self { outcome: "denied" }
    }

    fn unavailable() -> Self {
        Self {
            outcome: "unavailable",
        }
    }
}

pub fn init(app: &AppHandle) {
    #[cfg(target_os = "macos")]
    macos::init(app);
    #[cfg(not(target_os = "macos"))]
    let _ = app;
}

pub async fn show(
    app: AppHandle,
    input: TaskNotificationInput,
) -> Result<TaskNotificationResult, String> {
    let prepared = prepare_notification(input)?;

    #[cfg(target_os = "linux")]
    return linux::show(app, prepared).await;

    #[cfg(target_os = "macos")]
    return macos::show(prepared).await;

    #[cfg(not(any(target_os = "linux", target_os = "macos")))]
    {
        let _ = (app, prepared);
        Ok(TaskNotificationResult::unavailable())
    }
}

fn activate(app: &AppHandle, input: TaskNotificationActivation) -> Result<(), String> {
    let input = validate_activation(input)?;
    crate::menu::show_window(app);
    app.emit(ACTIVATION_EVENT, input)
        .map_err(|error| format!("emit task notification activation: {error}"))
}

fn test_activation_allowed(debug_assertions: bool, opt_in: Option<&str>) -> bool {
    debug_assertions && opt_in == Some("1")
}

pub fn activate_test(app: &AppHandle, input: TaskNotificationActivation) -> Result<(), String> {
    validate_activation(input.clone())?;
    let opt_in = std::env::var("TARIBOY_DESKTOP_NOTIFICATION_TEST").ok();
    if !test_activation_allowed(cfg!(debug_assertions), opt_in.as_deref()) {
        return Err("desktop notification test activation is disabled".to_string());
    }
    activate(app, input)
}

#[cfg(target_os = "linux")]
mod linux {
    use super::*;

    pub async fn show(
        app: AppHandle,
        prepared: PreparedNotification,
    ) -> Result<TaskNotificationResult, String> {
        let result = tauri::async_runtime::spawn_blocking(move || {
            notify_rust::Notification::new()
                .appname("Tariboy")
                .summary(&prepared.title)
                .body(&prepared.body)
                .action("default", "Open")
                .show()
                .map(|handle| (handle, prepared.activation))
        })
        .await
        .map_err(|error| format!("notification task did not run: {error}"))?;

        let Ok((handle, activation)) = result else {
            return Ok(TaskNotificationResult::unavailable());
        };
        std::thread::spawn(move || {
            handle.wait_for_action(|action| {
                if action == "default" {
                    let _ = activate(&app, activation);
                }
            });
        });
        Ok(TaskNotificationResult::shown())
    }
}

#[cfg(target_os = "macos")]
mod macos {
    use super::*;
    use std::ffi::{c_char, c_int, c_void, CStr, CString};
    use std::sync::{mpsc, OnceLock};
    use std::time::Duration;

    const OUTCOME_UNAVAILABLE: c_int = 0;
    const OUTCOME_SHOWN: c_int = 1;
    const OUTCOME_DENIED: c_int = 2;
    static APP_HANDLE: OnceLock<AppHandle> = OnceLock::new();

    type ActivationCallback = unsafe extern "C" fn(*const c_char);
    type ShowCompletion = unsafe extern "C" fn(c_int, *mut c_void);

    extern "C" {
        fn tariboy_notifications_init(callback: Option<ActivationCallback>);
        fn tariboy_notification_show(
            identifier: *const c_char,
            title: *const c_char,
            body: *const c_char,
            payload_json: *const c_char,
            completion: Option<ShowCompletion>,
            context: *mut c_void,
        );
    }

    pub fn init(app: &AppHandle) {
        let _ = APP_HANDLE.set(app.clone());
        unsafe { tariboy_notifications_init(Some(activation_callback)) };
    }

    unsafe extern "C" fn activation_callback(payload_json: *const c_char) {
        if payload_json.is_null() {
            return;
        }
        let Ok(json) = CStr::from_ptr(payload_json).to_str() else {
            return;
        };
        let Ok(payload) = serde_json::from_str::<TaskNotificationActivation>(json) else {
            return;
        };
        let Ok(payload) = validate_activation(payload) else {
            return;
        };
        if let Some(app) = APP_HANDLE.get() {
            let _ = activate(app, payload);
        }
    }

    unsafe extern "C" fn show_completion(outcome: c_int, context: *mut c_void) {
        if context.is_null() {
            return;
        }
        let sender = Box::from_raw(context.cast::<mpsc::Sender<c_int>>());
        let _ = sender.send(outcome);
    }

    pub async fn show(prepared: PreparedNotification) -> Result<TaskNotificationResult, String> {
        let identifier = CString::new(prepared.native_id)
            .map_err(|_| "notification identity contains NUL".to_string())?;
        let title = CString::new(prepared.title)
            .map_err(|_| "notification title contains NUL".to_string())?;
        let body = CString::new(prepared.body)
            .map_err(|_| "notification body contains NUL".to_string())?;
        let payload = CString::new(
            serde_json::to_string(&prepared.activation)
                .map_err(|error| format!("encode notification activation: {error}"))?,
        )
        .map_err(|_| "notification activation contains NUL".to_string())?;
        let (sender, receiver) = mpsc::channel();
        let context = Box::into_raw(Box::new(sender)).cast::<c_void>();

        unsafe {
            tariboy_notification_show(
                identifier.as_ptr(),
                title.as_ptr(),
                body.as_ptr(),
                payload.as_ptr(),
                Some(show_completion),
                context,
            );
        }

        let outcome = tauri::async_runtime::spawn_blocking(move || {
            receiver.recv_timeout(Duration::from_secs(30))
        })
        .await
        .map_err(|error| format!("notification permission task did not run: {error}"))?
        .unwrap_or(OUTCOME_UNAVAILABLE);

        Ok(match outcome {
            OUTCOME_SHOWN => TaskNotificationResult::shown(),
            OUTCOME_DENIED => TaskNotificationResult::denied(),
            _ => TaskNotificationResult::unavailable(),
        })
    }
}
