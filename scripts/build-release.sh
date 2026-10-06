#!/usr/bin/env bash
# AiKlog 爱库录 发行构建：npm → webdist → go build（注入 version）
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${1:-0.1.1}"
OUT="${ROOT}/dist/aiklog"
OS="${GOOS:-linux}"
ARCH="${GOARCH:-amd64}"

echo "==> version ${VERSION}  target ${OS}/${ARCH}"

echo "==> web build"
cd "${ROOT}/web"
npm install --no-fund --no-audit
npm run build

echo "==> embed webdist"
rm -rf "${ROOT}/server/internal/handler/webdist"
cp -r "${ROOT}/web/dist" "${ROOT}/server/internal/handler/webdist"

echo "==> go build"
mkdir -p "${ROOT}/dist"
cd "${ROOT}/server"
CGO_ENABLED=0 GOOS="${OS}" GOARCH="${ARCH}" \
  go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
  -o "${OUT}" ./cmd/aikmap

ls -lh "${OUT}"
echo "==> done: ${OUT}"
