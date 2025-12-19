#!/bin/bash
# ogen生成コードのnull処理を追加するスクリプト
set -e

cd "$(dirname "$0")/.."

go run scripts/fix-ogen-null.go
