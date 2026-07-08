#!/usr/bin/env bash
# Monta o PACOTE DE DISTRIBUIÇÃO do Zhu para MACOS em dist/macos/:
# node estático para Apple Silicon (arm64) e Macs Intel (amd64), desktop
# (se este build rodar num Mac), zhu.conf, instalador (já remove a
# quarentena do Gatekeeper), LEIA-ME e SHA256SUMS — mais o compactado
# versionado dist/zhu-<versão>-macos.tar.gz pronto para enviar.
#
# Uso:  scripts/build-macos.sh
# Config: build.conf na raiz (veja build.conf.example)
source "$(dirname "${BASH_SOURCE[0]}")/build-common.sh"

pkg_init macos
build_node darwin arm64 zhu-arm64
build_node darwin amd64 zhu-amd64
build_desktop darwin arm64 zhu-desktop-arm64
build_desktop darwin amd64 zhu-desktop-amd64
write_installer_sh sim
pkg_finish

echo "✅ macOS pronto — envie o .tar.gz; o amigo extrai e roda ./instalar.sh"
