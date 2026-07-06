#!/usr/bin/env bash
# Monta o PACOTE DE DISTRIBUIÇÃO do PANDA para MACOS em dist/macos/:
# node estático para Apple Silicon (arm64) e Macs Intel (amd64), desktop
# (se este build rodar num Mac), panda.conf, instalador (já remove a
# quarentena do Gatekeeper), LEIA-ME e SHA256SUMS — mais o compactado
# versionado dist/panda-<versão>-macos.tar.gz pronto para enviar.
#
# Uso:  scripts/build-macos.sh
# Config: build.conf na raiz (veja build.conf.example)
source "$(dirname "${BASH_SOURCE[0]}")/build-common.sh"

pkg_init macos
build_node darwin arm64 panda-node-arm64
build_node darwin amd64 panda-node-amd64
build_desktop darwin arm64 panda-desktop-arm64
build_desktop darwin amd64 panda-desktop-amd64
write_installer_sh sim
pkg_finish

echo "✅ macOS pronto — envie o .tar.gz; o amigo extrai e roda ./instalar.sh"
