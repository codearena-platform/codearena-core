#!/bin/bash
set -e

PWD_PATH=$(pwd)
OUT_DIR="docs/api"

# Auto-detect container engine
CONTAINER_ENGINE="podman"
if ! command -v podman &> /dev/null; then
    if command -v docker &> /dev/null; then
        CONTAINER_ENGINE="docker"
    else
        echo "Error: Neither 'podman' nor 'docker' found in PATH."
        exit 1
    fi
fi

echo -e "\033[0;36mUsing container engine: $CONTAINER_ENGINE\033[0m"

# Ensure output dir exists
mkdir -p "$OUT_DIR"

# Check if generator image exists
if [[ "$($CONTAINER_ENGINE images -q codearena-gen 2> /dev/null)" == "" ]]; then
  echo -e "\033[0;33mGenerator image 'codearena-gen' not found. Building...\033[0m"
  $CONTAINER_ENGINE build -t codearena-gen -f scripts/Dockerfile.gen scripts
fi

echo -e "\033[0;36mRunning protoc via $CONTAINER_ENGINE...\033[0m"
echo "Mounting: $PWD_PATH -> /workspace"

# Run Container
# We set working directory to the proto folder so imports resolve correctly
# We output OpenAPI to ../../../docs/api
# We output Go code to ../../../pkg/api/v1 (relative to api/proto/v1)
$CONTAINER_ENGINE run --rm \
    -v "${PWD_PATH}:/workspace" \
    -w /workspace/api/proto/v1 \
    codearena-gen \
    --proto_path=. \
    --go_out=../../../pkg/api/v1 --go_opt=paths=source_relative \
    --go-grpc_out=../../../pkg/api/v1 --go-grpc_opt=paths=source_relative \
    --openapi_out=../../../docs/api \
    arena.proto bot_api.proto

echo -e "\033[0;32mDone!\033[0m"
ls -l "$OUT_DIR"
