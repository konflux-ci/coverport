# CoverPort

Multi-language coverage collection system for containerized applications. Collect code coverage from running containers via HTTP with no volume mounts or deployment modifications.

## 📦 What's Inside

- **`instrumentation/`** - Coverage servers (Go, Python, Node.js) that embed into your applications to expose coverage via HTTP
- **`clients/`** - Client libraries (Go, Python, Node.js) to collect coverage from Kubernetes pods using port-forwarding
- **`coverage-processor/`** - Automated Tekton pipeline that processes coverage artifacts from Quay.io webhooks and uploads to SonarCloud (Konflux integration)

## 🚀 Quick Overview

1. **Inject** coverage server during build (`-cover` flag for Go, coverage.py for Python)
2. **Collect** coverage from running pods via HTTP (no volumes needed)
3. **Process** automatically via webhooks when artifacts are pushed to Quay.io
4. **Upload** to SonarCloud with proper Git context from SLSA attestations