// Injects the Go-side version string (internal/version.Version) so the app can
// tell "the daemon I bundle" from "the daemon that is running". `make desktop`
// may export TARIBOY_VERSION. Bare cargo commands read the same Go source so
// Rust runtime diagnostics never silently lose the alpha prerelease suffix.
fn main() {
    let target_os = std::env::var("CARGO_CFG_TARGET_OS").unwrap_or_default();
    if target_os == "macos" {
        cc::Build::new()
            .file("src/task_notifications.m")
            .flag("-fobjc-arc")
            .flag("-fblocks")
            .compile("task_notifications");
        println!("cargo:rustc-link-lib=framework=UserNotifications");
        println!("cargo:rerun-if-changed=src/task_notifications.m");
    }

    let source =
        std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("../../internal/version/version.go");
    let go_source =
        std::fs::read_to_string(&source).expect("read ../../internal/version/version.go");
    let source_version = go_source
        .lines()
        .find_map(|line| {
            line.strip_prefix("const Version = \"")
                .and_then(|value| value.strip_suffix('"'))
        })
        .expect("parse internal/version.Version");
    let v = std::env::var("TARIBOY_VERSION").unwrap_or_else(|_| source_version.to_string());
    assert_eq!(
        v, source_version,
        "TARIBOY_VERSION must match internal/version.Version"
    );
    println!("cargo:rustc-env=TARIBOY_VERSION={v}");
    println!("cargo:rerun-if-env-changed=TARIBOY_VERSION");
    println!("cargo:rerun-if-changed={}", source.display());
    tauri_build::build()
}
