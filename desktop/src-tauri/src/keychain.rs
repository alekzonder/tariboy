//! Small credential-store seam around macOS Keychain.
//!
//! Keeping this trait tiny makes host commands testable on Linux without
//! linking Apple frameworks and prevents credentials from ever entering the
//! serialized host registry.

use std::sync::Arc;

#[cfg(any(target_os = "macos", test))]
pub const SERVICE: &str = "app.tariboy.desktop.host";

pub trait TokenStore: Send + Sync {
    fn set(&self, account: &str, token: &str) -> Result<(), String>;
    fn get(&self, account: &str) -> Result<Option<String>, String>;
    fn delete(&self, account: &str) -> Result<(), String>;
}

#[cfg(target_os = "macos")]
pub struct SystemKeychain;

#[cfg(target_os = "macos")]
impl TokenStore for SystemKeychain {
    fn set(&self, account: &str, token: &str) -> Result<(), String> {
        security_framework::passwords::set_generic_password(SERVICE, account, token.as_bytes())
            .map_err(|error| format!("store host token in Keychain: {error}"))
    }

    fn get(&self, account: &str) -> Result<Option<String>, String> {
        const ERR_SEC_ITEM_NOT_FOUND: i32 = -25300;
        match security_framework::passwords::get_generic_password(SERVICE, account) {
            Ok(bytes) => String::from_utf8(bytes)
                .map(Some)
                .map_err(|_| "host token in Keychain is not valid UTF-8".into()),
            Err(error) if error.code() == ERR_SEC_ITEM_NOT_FOUND => Ok(None),
            Err(error) => Err(format!("read host token from Keychain: {error}")),
        }
    }

    fn delete(&self, account: &str) -> Result<(), String> {
        const ERR_SEC_ITEM_NOT_FOUND: i32 = -25300;
        match security_framework::passwords::delete_generic_password(SERVICE, account) {
            Ok(()) => Ok(()),
            Err(error) if error.code() == ERR_SEC_ITEM_NOT_FOUND => Ok(()),
            Err(error) => Err(format!("delete host token from Keychain: {error}")),
        }
    }
}

#[cfg(not(target_os = "macos"))]
pub struct SystemKeychain;

#[cfg(not(target_os = "macos"))]
impl TokenStore for SystemKeychain {
    fn set(&self, _account: &str, _token: &str) -> Result<(), String> {
        Err("native host tokens require macOS Keychain".into())
    }

    fn get(&self, _account: &str) -> Result<Option<String>, String> {
        Err("native host tokens require macOS Keychain".into())
    }

    fn delete(&self, _account: &str) -> Result<(), String> {
        Ok(())
    }
}

pub fn system() -> Arc<dyn TokenStore> {
    Arc::new(SystemKeychain)
}

#[cfg(test)]
#[derive(Default)]
pub struct MemoryKeychain {
    values: std::sync::Mutex<std::collections::HashMap<String, String>>,
}

#[cfg(test)]
impl TokenStore for MemoryKeychain {
    fn set(&self, account: &str, token: &str) -> Result<(), String> {
        self.values
            .lock()
            .map_err(|_| "memory keychain lock poisoned")?
            .insert(account.to_string(), token.to_string());
        Ok(())
    }

    fn get(&self, account: &str) -> Result<Option<String>, String> {
        Ok(self
            .values
            .lock()
            .map_err(|_| "memory keychain lock poisoned")?
            .get(account)
            .cloned())
    }

    fn delete(&self, account: &str) -> Result<(), String> {
        self.values
            .lock()
            .map_err(|_| "memory keychain lock poisoned")?
            .remove(account);
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn memory_adapter_round_trips_and_deletes_idempotently() {
        let store = MemoryKeychain::default();
        assert_eq!(store.get("host-1").unwrap(), None);
        store.set("host-1", "secret").unwrap();
        assert_eq!(store.get("host-1").unwrap().as_deref(), Some("secret"));
        store.delete("host-1").unwrap();
        store.delete("host-1").unwrap();
        assert_eq!(store.get("host-1").unwrap(), None);
    }

    #[test]
    fn service_and_account_contract_is_stable() {
        assert_eq!(SERVICE, "app.tariboy.desktop.host");
    }
}
