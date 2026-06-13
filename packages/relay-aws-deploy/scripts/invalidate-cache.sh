#!/bin/bash
# CloudFront キャッシュ無効化スクリプト
#
# Usage: ./scripts/invalidate-cache.sh [STACK_NAME]
#
# STACK_NAME を省略した場合は BacklogRelayStack を使用

set -e

STACK_NAME="${1:-BacklogRelayStack}"

echo "Fetching Distribution ID from stack: $STACK_NAME"

DIST_ID=$(aws cloudformation describe-stacks \
  --stack-name "$STACK_NAME" \
  --query "Stacks[0].Outputs[?OutputKey=='DistributionId'].OutputValue" \
  --output text 2>/dev/null)

if [ -z "$DIST_ID" ] || [ "$DIST_ID" = "None" ]; then
  echo "Error: Distribution ID not found. Is CloudFront enabled?" >&2
  exit 1
fi

echo "Distribution ID: $DIST_ID"
echo "Creating invalidation for all paths..."

INVALIDATION_ID=$(aws cloudfront create-invalidation \
  --distribution-id "$DIST_ID" \
  --paths "/*" \
  --query "Invalidation.Id" \
  --output text)

echo "Invalidation created: $INVALIDATION_ID"
echo "Done. Cache invalidation may take a few minutes to complete."
