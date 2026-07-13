# coverage-server (Rust)

Embedded HTTP server that exposes LLVM code coverage data from instrumented Rust binaries at runtime via HTTP. Collects profraw data directly from memory using `__llvm_profile_write_buffer()` — no disk I/O required, works on fully read-only filesystems.

## Adding the dependency

Add `coverage-server` as an optional dependency behind a feature flag:

```toml
[features]
coverage = ["dep:coverage-server"]

[dependencies]
coverage-server = { git = "https://github.com/konflux-ci/coverport.git", subdirectory = "instrumentation/rust", optional = true }
```

## Usage

Add two lines to your `main()`:

```rust
fn main() {
    #[cfg(feature = "coverage")]
    let _coverage = coverage_server::start_coverage_server_standalone(53700);

    // The rest of your application is completely unchanged.
}
```

The server spawns on a background thread with its own tokio runtime, listening on port 53700. It shares LLVM coverage counters with your app but does not interfere with your application's logic or runtime.

## Building with coverage instrumentation

```bash
# Production build (coverage-server not compiled)
cargo build --release

# Coverage-instrumented build (for test environments)
RUSTFLAGS="-C instrument-coverage" LLVM_PROFILE_FILE=/dev/null cargo build --release --features coverage
```

## Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/coverage` | GET/POST | Returns base64-encoded profraw data as JSON |
| `/coverage/reset` | GET/POST | Resets coverage counters (for per-test coverage) |
| `/health` | GET | Health check |

## Collecting coverage

Use [coverport](../../cli/) to collect and process coverage from running applications:

```bash
coverport collect --url http://localhost:53700/coverage --test-name e2e -o ./coverage-output

COVERAGE_BINARY=./target/release/my-app coverport process \
    --coverage-dir=./coverage-output --format=rust --generate-html \
    --skip-clone --upload=false
```