use std::{
    env,
    path::{Path, PathBuf},
    process::Command,
};

fn main() {
    println!("cargo:rerun-if-env-changed=RUSTC");
    println!("cargo:rerun-if-env-changed=PROFILE");
    println!("cargo:rerun-if-env-changed=TARGET");
    println!("cargo:rerun-if-env-changed=PARAFLOW_SOURCE_COMMIT");
    println!("cargo:rerun-if-env-changed=PARAFLOW_SOURCE_STATE");

    let manifest_dir = PathBuf::from(
        env::var_os("CARGO_MANIFEST_DIR").expect("Cargo must set CARGO_MANIFEST_DIR"),
    );
    let repository_root = manifest_dir.join("../../..");
    for git_metadata in [
        repository_root.join(".git/HEAD"),
        repository_root.join(".git/index"),
    ] {
        if git_metadata.exists() {
            println!("cargo:rerun-if-changed={}", git_metadata.display());
        }
    }

    emit("PARAFLOW_BUILD_PROFILE", env_value("PROFILE"));
    emit("PARAFLOW_BUILD_TARGET", env_value("TARGET"));
    emit("PARAFLOW_BUILD_RUSTC", rustc_version());
    emit(
        "PARAFLOW_BUILD_GIT_COMMIT",
        env::var("PARAFLOW_SOURCE_COMMIT").unwrap_or_else(|_| {
            git_output(&repository_root, &["rev-parse", "HEAD"])
                .unwrap_or_else(|| "unknown".to_owned())
        }),
    );
    emit(
        "PARAFLOW_BUILD_GIT_STATE",
        env::var("PARAFLOW_SOURCE_STATE").unwrap_or_else(|_| git_state(&repository_root)),
    );
}

fn env_value(name: &str) -> String {
    env::var(name).unwrap_or_else(|_| "unknown".to_owned())
}

fn rustc_version() -> String {
    let rustc = env::var_os("RUSTC").unwrap_or_else(|| "rustc".into());
    command_output(Command::new(rustc).arg("--version"))
        .unwrap_or_else(|| "unknown".to_owned())
}

fn git_state(repository_root: &Path) -> String {
    let Some(status) = git_output(repository_root, &["status", "--porcelain"]) else {
        return "unknown".to_owned();
    };
    if status.is_empty() {
        "clean".to_owned()
    } else {
        "dirty".to_owned()
    }
}

fn git_output(repository_root: &Path, arguments: &[&str]) -> Option<String> {
    let mut command = Command::new("git");
    command.arg("-C").arg(repository_root).args(arguments);
    command_output(&mut command)
}

fn command_output(command: &mut Command) -> Option<String> {
    let output = command.output().ok()?;
    if !output.status.success() {
        return None;
    }
    Some(String::from_utf8_lossy(&output.stdout).trim().to_owned())
}

fn emit(name: &str, value: String) {
    let sanitized = value.replace(['\r', '\n'], " ");
    println!("cargo:rustc-env={name}={sanitized}");
}
