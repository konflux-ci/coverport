FROM golang:1.24 AS builder

WORKDIR /workspace

# Copy go mod files
COPY go.mod go.sum ./
COPY ../go.mod ../go.sum ../

# Copy source
COPY . .
COPY ../ ../

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o coverport main.go

# Runtime image
FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=builder /workspace/coverport .

USER 65532:65532

ENTRYPOINT ["/coverport"]

