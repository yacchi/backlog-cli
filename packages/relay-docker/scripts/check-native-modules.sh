#!/bin/bash
# Check for native modules that would break cross-platform Docker builds
# Exit with error if any .node files (native addons) are found

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PKG_DIR="$(dirname "$SCRIPT_DIR")"

# Check for .node files (compiled native addons)
NATIVE_FILES=$(find "$PKG_DIR/node_modules" -name "*.node" 2>/dev/null || true)

if [ -n "$NATIVE_FILES" ]; then
  echo "ERROR: Native modules detected in node_modules:"
  echo "$NATIVE_FILES"
  echo ""
  echo "Native modules are architecture-dependent and will break Docker builds."
  echo "Please use pure JavaScript alternatives or build inside Docker."
  exit 1
fi

# Check for common native module patterns in package-lock or dependencies
NATIVE_PATTERNS="node-gyp|node-pre-gyp|prebuild|napi|native"
SUSPICIOUS=$(grep -r -l -E "$NATIVE_PATTERNS" "$PKG_DIR/node_modules/.package-lock.json" 2>/dev/null || true)

if [ -n "$SUSPICIOUS" ]; then
  echo "WARNING: Potentially native modules detected (check manually):"
  echo "$SUSPICIOUS"
fi

echo "No native modules detected. Safe for cross-platform Docker build."
