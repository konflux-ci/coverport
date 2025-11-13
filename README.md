# CoverPort

Multi-language coverage collection system for containerized applications running in Kubernetes. Collect code coverage from running containers via HTTP with no volume mounts or deployment modifications.

## 📦 What's Inside

- **`cli/`** - **coverport CLI** - Kubernetes-native tool for collecting coverage from running pods via port-forwarding. Supports Konflux snapshot integration, multi-component collection, and OCI artifact publishing.
- **`instrumentation/`** - Coverage HTTP servers (Go, Python, Node.js) that embed into your applications to expose coverage data via HTTP endpoint (port 9095).
- **`coverage-processor/`** - Automated Tekton pipeline that processes coverage artifacts from Quay.io webhooks, extracts Git metadata from SLSA attestations, and uploads remapped coverage to SonarCloud.
