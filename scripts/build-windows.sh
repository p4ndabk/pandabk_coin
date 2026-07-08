#!/usr/bin/env bash
# Monta o PACOTE DE DISTRIBUIÇÃO do Zhu para WINDOWS em dist/windows/:
# node .exe (amd64 e arm64, cross-compilados de Linux/macOS), desktop (só
# se este build rodar num Windows), zhu.conf, instalar.bat, LEIA-ME e
# SHA256SUMS — mais o dist/zhu-<versão>-windows.zip pronto para enviar.
#
# Uso:  scripts/build-windows.sh
# Config: build.conf na raiz (veja build.conf.example)
source "$(dirname "${BASH_SOURCE[0]}")/build-common.sh"

pkg_init windows
build_node windows amd64 zhu-amd64.exe
build_node windows arm64 zhu-arm64.exe
build_desktop windows amd64 zhu-desktop-amd64.exe
write_installer_bat
pkg_finish

echo "✅ Windows pronto — envie o .zip; o amigo extrai e roda instalar.bat"
