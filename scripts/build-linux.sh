#!/usr/bin/env bash
# Builda o node PANDA para LINUX: PCs/servidores (amd64) e
# Raspberry Pi 3/4/5 (arm64). Binários estáticos, sem dependências.
#
# Uso:  scripts/build-linux.sh
# Config: build.conf na raiz (veja build.conf.example)
source "$(dirname "${BASH_SOURCE[0]}")/build-common.sh"

build_target linux amd64
build_target linux arm64

echo "✅ Linux pronto. Copie o binário, dê chmod +x e rode."
