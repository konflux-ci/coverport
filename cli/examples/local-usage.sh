#!/bin/bash
# Example: Using coverport CLI locally

set -e

echo "🚀 coverport - Local Usage Examples"
echo "===================================="

# Example 1: Discover pods by label selector
echo ""
echo "Example 1: Discover pods by label selector"
echo "-------------------------------------------"
coverport discover \
  --namespace=default \
  --label-selector=app=myapp \
  --verbose

# Example 2: Collect coverage from discovered pods
echo ""
echo "Example 2: Collect coverage from pods"
echo "--------------------------------------"
coverport collect \
  --namespace=default \
  --label-selector=app=myapp \
  --test-name="manual-test-$(date +%Y%m%d-%H%M%S)" \
  --output=./coverage-output \
  --verbose

# Example 3: Collect from specific images
echo ""
echo "Example 3: Collect from specific container images"
echo "--------------------------------------------------"
coverport collect \
  --images=quay.io/myorg/app1:latest,quay.io/myorg/app2:latest \
  --test-name="image-based-test" \
  --output=./coverage-output \
  --verbose

# Example 4: Collect and push to registry (requires authentication)
echo ""
echo "Example 4: Collect and push to OCI registry"
echo "--------------------------------------------"
# Make sure you're logged in: docker login quay.io
coverport collect \
  --namespace=default \
  --label-selector=app=myapp \
  --test-name="pushed-coverage-$(date +%Y%m%d-%H%M%S)" \
  --output=./coverage-output \
  --push \
  --registry=quay.io \
  --repository=myorg/coverage-artifacts \
  --expires-after=7d \
  --verbose

# Example 5: Using a snapshot file
echo ""
echo "Example 5: Using a snapshot file"
echo "---------------------------------"
cat > /tmp/snapshot.json << 'EOF'
{
  "components": [
    {
      "name": "myapp",
      "containerImage": "quay.io/myorg/myapp:latest"
    }
  ]
}
EOF

coverport collect \
  --snapshot-file=/tmp/snapshot.json \
  --test-name="snapshot-based-test" \
  --output=./coverage-output \
  --verbose

# Example 6: Collect without auto-processing (manual control)
echo ""
echo "Example 6: Collect without auto-processing"
echo "-------------------------------------------"
coverport collect \
  --namespace=default \
  --label-selector=app=myapp \
  --test-name="raw-collection" \
  --output=./coverage-output \
  --auto-process=false \
  --verbose

echo ""
echo "✅ All examples complete!"
echo "📁 Coverage data saved to: ./coverage-output"

