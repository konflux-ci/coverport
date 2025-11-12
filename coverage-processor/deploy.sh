#!/bin/bash
set -e

# Coverage Processor Deployment Script
# This script deploys all Kubernetes and Tekton manifests for the Coverage processor

NAMESPACE="coverage-processor"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "=========================================="
echo "Coverage Processor Deployment"
echo "=========================================="
echo ""

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}[✓]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[!]${NC} $1"
}

print_error() {
    echo -e "${RED}[✗]${NC} $1"
}

print_info() {
    echo -e "${BLUE}[i]${NC} $1"
}

# Load configuration from config.env if it exists
if [ -f "${SCRIPT_DIR}/config.env" ]; then
    print_info "Loading configuration from config.env"
    source "${SCRIPT_DIR}/config.env"
fi

# Check if oc or kubectl is available
if command -v oc &> /dev/null; then
    KUBE_CMD="oc"
    print_status "Using oc CLI"
elif command -v kubectl &> /dev/null; then
    KUBE_CMD="kubectl"
    print_status "Using kubectl CLI"
else
    print_error "Neither oc nor kubectl is installed. Please install one of them first."
    exit 1
fi

# Check if cluster is accessible
if ! $KUBE_CMD cluster-info &> /dev/null; then
    print_error "Cannot connect to Kubernetes cluster. Please check your kubeconfig."
    exit 1
fi

print_status "Connected to Kubernetes cluster"

# Check if Tekton is installed
if ! $KUBE_CMD get crd tasks.tekton.dev &> /dev/null; then
    print_warning "Tekton Pipelines CRD not found. Tekton may not be installed."
    echo "Install Tekton Pipelines with:"
    echo "  kubectl apply -f https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml"
    read -p "Do you want to continue anyway? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

if ! $KUBE_CMD get crd eventlisteners.triggers.tekton.dev &> /dev/null; then
    print_warning "Tekton Triggers CRD not found. Tekton Triggers may not be installed."
    echo "Install Tekton Triggers with:"
    echo "  kubectl apply -f https://storage.googleapis.com/tekton-releases/triggers/latest/release.yaml"
    echo "  kubectl apply -f https://storage.googleapis.com/tekton-releases/triggers/latest/interceptors.yaml"
    read -p "Do you want to continue anyway? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

print_status "Tekton prerequisites check passed"

echo ""
echo "=========================================="
echo "Configuration"
echo "=========================================="
echo ""

# Prompt for Smee.io channel URL if not set
if [ -z "$SMEE_CHANNEL_URL" ]; then
    print_warning "Smee.io channel URL not configured"
    echo ""
    echo "You need a Smee.io channel to forward Quay.io webhooks to your cluster."
    echo "Get a channel from: https://smee.io/new"
    echo ""
    read -p "Enter your Smee.io channel URL (e.g., https://smee.io/abc123): " SMEE_CHANNEL_URL
    
    if [ -z "$SMEE_CHANNEL_URL" ]; then
        print_error "Smee.io channel URL is required"
        exit 1
    fi
fi

print_status "Smee.io channel: $SMEE_CHANNEL_URL"

# Prompt for SonarCloud token if not set
if [ -z "$SONAR_TOKEN" ]; then
    print_warning "SonarCloud token not configured"
    echo ""
    echo "You need a SonarCloud token to upload coverage."
    echo "Get your token from: https://sonarcloud.io/account/security"
    echo ""
    read -p "Enter your SonarCloud token (or press Enter to configure later): " SONAR_TOKEN
    
    if [ -z "$SONAR_TOKEN" ]; then
        print_warning "Will skip secret creation. You must create it manually before running tasks."
        SKIP_SECRET=true
    fi
fi

# Set default SONAR_HOST_URL if not provided
if [ -z "$SONAR_HOST_URL" ]; then
    SONAR_HOST_URL="https://sonarcloud.io"
fi

echo ""
echo "Configuration summary:"
echo "  - Namespace: $NAMESPACE"
echo "  - Smee.io channel: $SMEE_CHANNEL_URL"
echo "  - SonarCloud host: $SONAR_HOST_URL"
if [ -n "$SONAR_TOKEN" ]; then
    echo "  - SonarCloud token: ****${SONAR_TOKEN: -4}"
else
    echo "  - SonarCloud token: (not configured)"
fi
echo ""

read -p "Continue with deployment? (Y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Nn]$ ]]; then
    echo "Deployment cancelled."
    exit 0
fi

echo ""
echo "=========================================="
echo "Deploying Coverage Processor"
echo "=========================================="
echo ""

# Step 1: Create namespace
echo "1. Creating namespace..."
$KUBE_CMD apply -f "${SCRIPT_DIR}/k8s/namespace.yaml"
print_status "Namespace created: ${NAMESPACE}"
echo ""

# Step 2: Create RBAC resources
echo "2. Creating RBAC resources..."
$KUBE_CMD apply -f "${SCRIPT_DIR}/k8s/rbac.yaml"
print_status "ServiceAccount, Roles, and RoleBindings created"
echo ""

# Step 3: Create or check SonarCloud secret
echo "3. Setting up SonarCloud secret..."
if [ "$SKIP_SECRET" = true ]; then
    print_warning "Skipping secret creation. Create it manually with:"
    echo "  $KUBE_CMD create secret generic sonar-token \\"
    echo "    --from-literal=SONAR_TOKEN=your-token-here \\"
    echo "    --from-literal=SONAR_HOST_URL=https://sonarcloud.io \\"
    echo "    -n ${NAMESPACE}"
elif $KUBE_CMD get secret sonar-token -n ${NAMESPACE} &> /dev/null; then
    print_info "Secret 'sonar-token' already exists"
    read -p "Do you want to update it with the new token? (y/N) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        $KUBE_CMD delete secret sonar-token -n ${NAMESPACE}
        $KUBE_CMD create secret generic sonar-token \
            --from-literal=SONAR_TOKEN="${SONAR_TOKEN}" \
            --from-literal=SONAR_HOST_URL="${SONAR_HOST_URL}" \
            -n ${NAMESPACE}
        print_status "SonarCloud secret updated"
    else
        print_info "Using existing secret"
    fi
else
    $KUBE_CMD create secret generic sonar-token \
        --from-literal=SONAR_TOKEN="${SONAR_TOKEN}" \
        --from-literal=SONAR_HOST_URL="${SONAR_HOST_URL}" \
        -n ${NAMESPACE}
    print_status "SonarCloud secret created"
fi
echo ""

# Step 4: Deploy gosmee webhook forwarder with substituted Smee.io URL
echo "4. Deploying webhook forwarder (gosmee)..."
# Create temporary file with substituted values
TEMP_GOSMEE=$(mktemp)
sed "s|__SMEE_CHANNEL_URL__|${SMEE_CHANNEL_URL}|g" \
    "${SCRIPT_DIR}/k8s/gosmee-deployment.yaml" > "$TEMP_GOSMEE"
$KUBE_CMD apply -f "$TEMP_GOSMEE"
rm "$TEMP_GOSMEE"
print_status "gosmee forwarder deployment created with channel: $SMEE_CHANNEL_URL"
echo ""

# Step 5: Deploy Tekton task
echo "5. Deploying Tekton task..."
$KUBE_CMD apply -f "${SCRIPT_DIR}/tekton/tasks/coverage-task.yaml"
print_status "Coverage task created"
echo ""

# Step 6: Deploy Tekton triggers
echo "6. Deploying Tekton triggers..."
$KUBE_CMD apply -f "${SCRIPT_DIR}/tekton/trigger-binding.yaml"
$KUBE_CMD apply -f "${SCRIPT_DIR}/tekton/trigger-template.yaml"
$KUBE_CMD apply -f "${SCRIPT_DIR}/tekton/eventlistener.yaml"
print_status "Tekton triggers created"
echo ""

# Wait for deployments to be ready
echo "7. Waiting for deployments to be ready..."
echo -n "   - gosmee-forwarder: "
$KUBE_CMD wait --for=condition=available --timeout=60s \
    deployment/gosmee-forwarder -n ${NAMESPACE} 2>/dev/null && echo "✓" || echo "✗"
echo -n "   - EventListener: "
$KUBE_CMD wait --for=condition=available --timeout=60s \
    deployment -l eventlistener=coverage-listener -n ${NAMESPACE} 2>/dev/null && echo "✓" || echo "✗"
print_status "Deployments are ready"
echo ""

echo "=========================================="
echo "Deployment Summary"
echo "=========================================="
echo ""

# Show deployment status
echo "Resources in namespace ${NAMESPACE}:"
$KUBE_CMD get all -n ${NAMESPACE}
echo ""

echo "Tekton resources:"
$KUBE_CMD get tasks,eventlisteners,triggerbindings,triggertemplates -n ${NAMESPACE}
echo ""

echo "=========================================="
echo "Next Steps"
echo "=========================================="
echo ""
echo "1. Verify gosmee forwarder is running:"
echo "   $KUBE_CMD logs -n ${NAMESPACE} -l component=gosmee-forwarder -f"
echo ""
echo "2. Verify EventListener is running:"
echo "   $KUBE_CMD logs -n ${NAMESPACE} -l eventlistener=coverage-listener -f"
echo ""
echo "3. Test manually with example TaskRun:"
echo "   # Edit the coverage artifact reference"
echo "   vi examples/manual-taskrun.yaml"
echo "   $KUBE_CMD apply -f examples/manual-taskrun.yaml"
echo "   tkn tr logs -f -n ${NAMESPACE} coverage-manual-test"
echo ""
echo "4. Configure Quay.io webhook (for automatic processing):"
echo "   - Go to your Quay.io repository settings → Webhooks"
echo "   - Add webhook URL: ${SMEE_CHANNEL_URL}"
echo "   - Select event: 'Repository Push'"
echo "   - Save and test"
echo ""
echo "5. Push a coverage artifact to trigger automatic processing:"
echo "   oras push quay.io/yourrepo/coverage-artifacts:test \\"
echo "     ./covcounters.* ./covmeta.* ./metadata.json"
echo ""
echo "6. Monitor TaskRuns:"
echo "   tkn tr list -n ${NAMESPACE}"
echo "   tkn tr logs -f -n ${NAMESPACE} <taskrun-name>"
echo ""

# Save configuration for future use
if [ ! -f "${SCRIPT_DIR}/config.env" ] && [ -n "$SMEE_CHANNEL_URL" ]; then
    cat > "${SCRIPT_DIR}/config.env" <<EOF
# Coverage Processor Configuration
# Auto-generated on $(date)

SMEE_CHANNEL_URL=${SMEE_CHANNEL_URL}
SONAR_HOST_URL=${SONAR_HOST_URL}
# Note: SONAR_TOKEN is not saved for security. Add it manually if needed.

EOF
    print_status "Configuration saved to config.env"
    echo ""
fi

print_status "Deployment complete!"
echo ""
echo "For more information, see README.md"
