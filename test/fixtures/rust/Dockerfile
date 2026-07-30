# Build context must be the repo root:
#   docker build -f test/fixtures/rust/Dockerfile -t coverport-testapp-rust .
FROM rust:1.80 AS builder

WORKDIR /build

COPY instrumentation/rust /instrumentation/rust

COPY test/fixtures/rust/Cargo.toml ./
RUN sed -i 's|path = "../../../instrumentation/rust"|path = "/instrumentation/rust"|' Cargo.toml

COPY test/fixtures/rust/src/ src/

ENV RUSTFLAGS="-C instrument-coverage"
RUN cargo build --release

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /build/target/release/coverport-testapp-rust /testapp
EXPOSE 8080 53700
ENTRYPOINT ["/testapp"]
