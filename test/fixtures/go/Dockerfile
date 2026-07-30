# Build context must be the repo root:
#   docker build -f test/fixtures/go/Dockerfile -t coverport-testapp-go .
FROM golang:1.24 AS builder

WORKDIR /build

COPY instrumentation/go /instrumentation/go

COPY test/fixtures/go/go.mod ./
RUN sed -i 's|../../../instrumentation/go|/instrumentation/go|' go.mod
RUN go mod download

COPY test/fixtures/go/main.go .
RUN CGO_ENABLED=0 go build -cover -covermode=atomic -o /testapp .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /testapp /testapp
EXPOSE 8080 53700
ENTRYPOINT ["/testapp"]
