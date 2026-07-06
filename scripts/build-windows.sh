#!/usr/bin/env bash
# Builda o node PANDA para WINDOWS (amd64 e arm64), a partir de
# Linux/macOS — cross-compile, sai um .exe sem dependências.
#
# Uso:  scripts/build-windows.sh
# Config: build.conf na raiz (veja build.conf.example)
source "$(dirname "${BASH_SOURCE[0]}")/build-common.sh"

build_target windows amd64
build_target windows arm64

echo "✅ Windows pronto. É só copiar o .exe e rodar no Prompt/PowerShell."
