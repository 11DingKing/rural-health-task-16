#!/usr/bin/env bash
set -e

IMAGE_NAME="${1:-rehabcert-eval}"
DOCKER_PLATFORM="${2:-linux/amd64}"

docker build --platform "$DOCKER_PLATFORM" -f eval.Dockerfile -t "$IMAGE_NAME" .

echo "Built image: $IMAGE_NAME (platform: $DOCKER_PLATFORM)"
