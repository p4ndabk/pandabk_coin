#!/usr/bin/env bash
# Builda o node PANDA para MACOS: Apple Silicon M1+ (arm64) e
# Macs Intel (amd64). Binários estáticos, sem dependências.
#
# Uso:  scripts/build-macos.sh
# Config: build.conf na raiz (veja build.conf.example)
source "$(dirname "${BASH_SOURCE[0]}")/build-common.sh"

build_target darwin arm64
build_target darwin amd64

echo "✅ macOS pronto. Se o Gatekeeper reclamar do binário baixado:"
echo "   xattr -d com.apple.quarantine ./panda-node"
