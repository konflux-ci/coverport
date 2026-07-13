//! # coverage-server
//!
//! An embedded HTTP server for collecting Rust code coverage from running applications.
//!
//! Compatible with the [coverport](https://github.com/konflux-ci/coverport) CLI tool
//! for coverage collection in Kubernetes/CI environments.
//!
//! ## How it works
//!
//! When a Rust binary is compiled with `-C instrument-coverage`, LLVM inserts profiling
//! counters. This library uses LLVM's runtime profile buffer functions via FFI to
//! serialize coverage data directly into memory — no filesystem writes required.
//!
//! The key LLVM runtime functions used:
//! - `__llvm_profile_get_size_for_buffer()` — returns the byte size needed for the profile
//! - `__llvm_profile_write_buffer()` — writes the profraw data into a caller-provided buffer
//! - `__llvm_profile_reset_counters()` — zeros all coverage counters
//!
//! This means the coverage server works on **read-only root filesystems** with no
//! writable volumes, temp directories, or disk access needed.
//!
//! ## Usage
//!
//! Works with any application — async or synchronous, any runtime:
//!
//! ```rust,no_run
//! fn main() {
//!     // Spawns the coverage server on its own thread with its own tokio runtime.
//!     // Returns immediately — does not block your app.
//!     let _handle = coverage_server::start_coverage_server_standalone(53700);
//!
//!     // ... rest of your application, any framework, any runtime ...
//! }
//! ```
//!

use axum::{
    http::{HeaderMap, HeaderValue, StatusCode},
    response::IntoResponse,
    routing::get,
    Json, Router,
};
use serde::Serialize;
use std::env;
use std::net::SocketAddr;
use tokio::task::JoinHandle;
use tracing::{error, info};

extern "C" {
    /// Returns the size in bytes needed to hold the serialized profile data.
    fn __llvm_profile_get_size_for_buffer() -> u64;

    /// Writes the profile data into the provided buffer.
    /// The buffer must be at least `__llvm_profile_get_size_for_buffer()` bytes.
    /// Returns 0 on success, non-zero on failure.
    fn __llvm_profile_write_buffer(buffer: *mut std::ffi::c_char) -> i32;

    /// Resets all coverage counters to zero.
    fn __llvm_profile_reset_counters();

    /// Sets the filename for the default atexit profile dump.
    /// Setting to "/dev/null" effectively disables disk writes on exit.
    fn __llvm_profile_set_filename(filename: *const std::ffi::c_char);
}

/// Disable the LLVM default atexit handler that writes profraw to disk.
/// This must be called early to ensure no filesystem writes occur,
/// which is critical for read-only root filesystem environments.
fn disable_profile_atexit_write() {
    unsafe {
        let devnull = std::ffi::CString::new("/dev/null").unwrap();
        __llvm_profile_set_filename(devnull.as_ptr());
    }
    info!("Disabled LLVM profile atexit disk write (redirected to /dev/null)");
}

/// Serialize LLVM profile data directly into an in-memory buffer.
/// No filesystem access is performed.
fn collect_profraw_in_memory() -> Result<Vec<u8>, String> {
    unsafe {
        let size = __llvm_profile_get_size_for_buffer() as usize;
        if size == 0 {
            return Err("Profile buffer size is 0 — coverage may not be enabled".to_string());
        }

        let mut buffer: Vec<u8> = vec![0u8; size];
        let result = __llvm_profile_write_buffer(buffer.as_mut_ptr() as *mut std::ffi::c_char);
        if result != 0 {
            return Err(format!(
                "__llvm_profile_write_buffer failed with error code: {}",
                result
            ));
        }

        Ok(buffer)
    }
}

/// Build the standard coverport identification headers.
/// These allow the coverport CLI to identify this as a coverage server.
fn coverport_headers() -> HeaderMap {
    let mut headers = HeaderMap::new();
    headers.insert("X-Art-Coverage-Server", HeaderValue::from_static("1"));
    headers.insert(
        "X-Art-Coverage-Binary",
        HeaderValue::from_str(&env::current_exe().unwrap_or_default().display().to_string())
            .unwrap_or_else(|_| HeaderValue::from_static("unknown")),
    );
    headers.insert(
        "X-Art-Coverage-Pid",
        HeaderValue::from_str(&std::process::id().to_string())
            .unwrap_or_else(|_| HeaderValue::from_static("0")),
    );

    if let Ok(commit) = env::var("SOURCE_GIT_COMMIT") {
        if let Ok(val) = HeaderValue::from_str(&commit) {
            headers.insert("X-Art-Coverage-Source-Commit", val);
        }
    }
    if let Ok(url) = env::var("SOURCE_GIT_URL") {
        if let Ok(val) = HeaderValue::from_str(&url) {
            headers.insert("X-Art-Coverage-Source-Url", val);
        }
    }

    headers
}

#[derive(Debug, Serialize)]
struct CoverageResponse {
    profraw_filename: String,
    profraw_data: String,
    profraw_size: usize,
    timestamp: u64,
    coverage_enabled: bool,
}

#[derive(Debug, Serialize)]
struct ErrorResponse {
    error: String,
}

struct CoverageServer {
    port: u16,
}

impl CoverageServer {
    /// Create a new coverage server on the specified port.
    /// Port can be overridden by the `COVERAGE_PORT` environment variable.
    /// Default port is 53700 (coverport standard).
    pub fn new(default_port: u16) -> Self {
        let port = env::var("COVERAGE_PORT")
            .ok()
            .and_then(|p| p.parse().ok())
            .unwrap_or(default_port);

        Self { port }
    }

    /// Start the coverage server in a background task.
    /// Returns a JoinHandle that can be awaited if needed.
    pub async fn start(self) -> JoinHandle<()> {
        disable_profile_atexit_write();

        let app = Router::new()
            .route("/coverage", get(handle_coverage).post(handle_coverage))
            .route(
                "/coverage/reset",
                get(handle_reset_counters).post(handle_reset_counters),
            )
            .route("/health", get(handle_health));

        let addr = SocketAddr::from(([0, 0, 0, 0], self.port));
        info!("Coverage server starting on {}", addr);

        tokio::spawn(async move {
            let listener = tokio::net::TcpListener::bind(addr)
                .await
                .expect("failed to bind coverage server");
            axum::serve(listener, app).await.unwrap();
        })
    }
}

async fn handle_coverage() -> impl IntoResponse {
    let headers = coverport_headers();

    let result = collect_profraw_in_memory();
    match result {
        Ok(profraw_data) => {
            let size = profraw_data.len();
            let encoded = base64::Engine::encode(
                &base64::engine::general_purpose::STANDARD,
                &profraw_data,
            );

            let timestamp = std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap_or_default()
                .as_nanos() as u64;

            let pid = std::process::id();
            let filename = format!("coverage.{}.{}.profraw", pid, timestamp);

            (
                StatusCode::OK,
                headers,
                Json(CoverageResponse {
                    profraw_filename: filename,
                    profraw_data: encoded,
                    profraw_size: size,
                    timestamp,
                    coverage_enabled: true,
                }),
            )
                .into_response()
        }
        Err(e) => {
            error!("Failed to collect profraw data: {}", e);
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                headers,
                Json(ErrorResponse { error: e }),
            )
                .into_response()
        }
    }
}

async fn handle_reset_counters() -> impl IntoResponse {
    let headers = coverport_headers();
    unsafe {
        __llvm_profile_reset_counters();
    }
    info!("Coverage counters reset");
    (StatusCode::OK, headers, "Counters reset successfully")
}

async fn handle_health() -> impl IntoResponse {
    let headers = coverport_headers();
    (StatusCode::OK, headers, "coverage server healthy")
}

/// Start the coverage server on its own background thread with its own tokio runtime.
/// Works with any application — no async runtime required.
/// `default_port` is used unless overridden by the `COVERAGE_PORT` env var.
/// Returns immediately; the server runs until the process exits.
pub fn start_coverage_server_standalone(default_port: u16) -> std::thread::JoinHandle<()> {
    let server = CoverageServer::new(default_port);
    std::thread::spawn(move || {
        let rt = tokio::runtime::Builder::new_current_thread()
            .enable_io()
            .build()
            .expect("failed to create coverage server runtime");
        rt.block_on(async {
            server.start().await.await.unwrap();
        });
    })
}
