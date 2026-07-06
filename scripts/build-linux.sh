#!/usr/bin/env bash
# Monta o PACOTE DE DISTRIBUIÇÃO do PANDA para LINUX em dist/linux/:
# node estático para PCs/servidores (amd64) e Raspberry Pi 3/4/5 (arm64),
# desktop (se este build rodar num Linux), panda.conf, instalador, LEIA-ME
# e SHA256SUMS — mais o compactado versionado dist/panda-<versão>-linux.tar.gz
# pronto para enviar.
#
# Uso:  scripts/build-linux.sh
# Config: build.conf na raiz (veja build.conf.example)
source "$(dirname "${BASH_SOURCE[0]}")/build-common.sh"

pkg_init linux
build_node linux amd64 panda-node-amd64
build_node linux arm64 panda-node-arm64
build_desktop linux amd64 panda-desktop-amd64
build_desktop linux arm64 panda-desktop-arm64
write_installer_sh nao
pkg_finish

echo "✅ Linux pronto — envie o .tar.gz; o amigo extrai e roda ./instalar.sh"
