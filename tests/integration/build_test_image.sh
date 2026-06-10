#!/bin/bash
# Build the unified test environment Docker image

set -e

IMAGE_NAME="scproxy-test-env"
IMAGE_TAG="latest"

cd "$(dirname "$0")"

echo "Building ${IMAGE_NAME}:${IMAGE_TAG} (using Aliyun mirrors)..."
docker build -f Dockerfile.test -t ${IMAGE_NAME}:${IMAGE_TAG} .

echo "✅ Build complete: ${IMAGE_NAME}:${IMAGE_TAG}"
echo ""
echo "Verify tools:"
docker run --rm ${IMAGE_NAME}:${IMAGE_TAG} bash -c "echo 'Node: \$(node --version)'; echo 'npm: \$(npm --version)'; echo 'Go: \$(go version)'; echo 'Rust: \$(rustc --version)'; echo 'Cargo: \$(cargo --version)'"