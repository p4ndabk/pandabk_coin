#!/usr/bin/env bash
# Monta o PACOTE DE DISTRIBUIÇÃO do Zhu para LINUX em dist/linux/:
# node estático para PCs/servidores (amd64) e Raspberry Pi 3/4/5 (arm64),
# desktop (se este build rodar num Linux), zhu.conf, instalador, LEIA-ME
# e SHA256SUMS — mais o compactado versionado dist/zhu-<versão>-linux.tar.gz
# pronto para enviar.
#
# Uso:  scripts/build-linux.sh
# Config: build.conf na raiz (veja build.conf.example)
source "$(dirname "${BASH_SOURCE[0]}")/build-common.sh"

pkg_init linux
build_node linux amd64 zhu-amd64
build_node linux arm64 zhu-arm64
build_desktop linux amd64 zhu-desktop-amd64
build_desktop linux arm64 zhu-desktop-arm64
write_installer_sh nao
pkg_finish

echo "✅ Linux pronto — envie o .tar.gz; o amigo extrai e roda ./instalar.sh"
