#![cfg(test)]

use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};

pub fn executable(dir: &Path, name: &str, body: &str) -> PathBuf {
    let path = dir.join(name);
    fs::write(&path, format!("#!/bin/sh\nset -eu\n{body}\n")).unwrap();
    fs::set_permissions(&path, fs::Permissions::from_mode(0o755)).unwrap();
    path
}

pub fn argv_logger(log: &Path, behavior: &str) -> String {
    format!(
        r#"printf 'BEGIN\n' >> '{log}'
for arg in "$@"; do printf 'ARG=%s\n' "$arg" >> '{log}'; done
printf 'END\n' >> '{log}'
{behavior}"#,
        log = log.display(),
    )
}

pub fn invocations(log: &Path) -> Vec<Vec<String>> {
    let text = fs::read_to_string(log).unwrap_or_default();
    let mut calls = Vec::new();
    let mut current = Vec::new();
    for line in text.lines() {
        match line {
            "BEGIN" => current.clear(),
            "END" => calls.push(current.clone()),
            _ => {
                if let Some(arg) = line.strip_prefix("ARG=") {
                    current.push(arg.to_string());
                }
            }
        }
    }
    calls
}
