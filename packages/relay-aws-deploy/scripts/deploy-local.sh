#!/usr/bin/env bash
# ローカルでコンテナイメージをビルドし、GHCR に push して CDK デプロイする。
# feature ブランチの変更を実環境で検証する際に使用。
#
# 使い方:
#   AWS_PROFILE=... ./scripts/deploy-local.sh [--tag TAG] [--no-push] [--diff]
#
# オプション:
#   --tag TAG   イメージタグ（デフォルト: dev-<short-sha>）
#   --no-push   ビルドのみ（push / deploy しない）
#   --diff      cdk diff のみ実行（deploy しない）

set -euo pipefail
cd "$(dirname "$0")/.."
REPO_ROOT="$(cd ../.. && pwd)"

IMAGE_NAME="ghcr.io/yacchi/backlog-relay"
TAG=""
NO_PUSH=false
DIFF_ONLY=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --tag)   TAG="$2"; shift 2 ;;
    --no-push) NO_PUSH=true; shift ;;
    --diff)  DIFF_ONLY=true; shift ;;
    *)       echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

if [[ -z "$TAG" ]]; then
  SHORT_SHA=$(git -C "$REPO_ROOT" rev-parse --short HEAD)
  TAG="dev-${SHORT_SHA}"
fi

FULL_IMAGE="${IMAGE_NAME}:${TAG}"

echo "==> Building ${FULL_IMAGE} (linux/arm64)"
docker buildx build \
  --platform linux/arm64 \
  -f "$REPO_ROOT/packages/relay-docker/Dockerfile" \
  -t "$FULL_IMAGE" \
  --provenance=false \
  --load \
  "$REPO_ROOT"

if $NO_PUSH; then
  echo "==> Skipping push (--no-push)"
  exit 0
fi

echo "==> Pushing ${FULL_IMAGE}"
docker push "$FULL_IMAGE"

export IMAGE_TAG="$TAG"

if $DIFF_ONLY; then
  echo "==> Running cdk diff (IMAGE_TAG=${TAG})"
  pnpm run build
  npx cdk diff --all
else
  echo "==> Deploying (IMAGE_TAG=${TAG})"
  pnpm run deploy --all
fi
