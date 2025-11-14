FROM registry.access.redhat.com/ubi9/go-toolset:1.24 AS builder

WORKDIR /workspace

# Copy go mod files and source
COPY --chown=default:root go.mod go.sum ./
USER root

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the coverport binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o coverport main.go

# Runtime image - using UBI9 minimal with necessary tools
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# Install required dependencies for coverport process command
# Note: curl-minimal is already installed in ubi9-minimal
RUN microdnf install -y \
    git \
    tar \
    gzip \
    jq \
    && microdnf clean all

# Install Go toolchain (needed for coverage processing)
# Download Go 1.24 to match the builder stage
RUN curl -LO https://go.dev/dl/go1.24.0.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz && \
    rm go1.24.0.linux-amd64.tar.gz
ENV PATH="/usr/local/go/bin:${PATH}"

# Install oras CLI (for OCI artifact operations)
RUN curl -LO https://github.com/oras-project/oras/releases/download/v1.2.0/oras_1.2.0_linux_amd64.tar.gz && \
    tar -xzf oras_1.2.0_linux_amd64.tar.gz && \
    mv oras /usr/local/bin/ && \
    rm oras_1.2.0_linux_amd64.tar.gz && \
    chmod +x /usr/local/bin/oras

# Install cosign (for extracting git metadata from image attestations)
RUN curl -LO https://github.com/sigstore/cosign/releases/download/v2.4.1/cosign-linux-amd64 && \
    mv cosign-linux-amd64 /usr/local/bin/cosign && \
    chmod +x /usr/local/bin/cosign

# Copy the coverport binary
COPY --from=builder /workspace/coverport /usr/local/bin/coverport

# Create a non-root user
RUN useradd -u 65532 -r -g 0 -m -d /home/coverport -s /sbin/nologin \
    -c "coverport user" coverport && \
    mkdir -p /workspace && \
    chown -R 65532:0 /workspace && \
    chmod -R g=u /workspace

WORKDIR /workspace

USER 65532

# Verify installations
RUN coverport --version && \
    git --version && \
    oras version && \
    cosign version && \
    go version

ENTRYPOINT ["coverport"]
