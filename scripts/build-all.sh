#!/usr/bin/env bash
# Monta os TRÊS pacotes de distribuição de uma vez (Linux, macOS e
# Windows): dist/<so>/ + os compactados versionados prontos para enviar
# (dist/panda-<versão>-linux.tar.gz, -macos.tar.gz e -windows.zip).
#
# Rodado num Mac com Docker ativo (colima start), os três saem COMPLETOS,
# desktop incluído: macOS nativo (Xcode cruza arm64/amd64), Linux e
# Windows pela imagem de cross-compile (scripts/desktop-cross.Dockerfile).
#
# Uso:  scripts/build-all.sh
set -euo pipefail
dir="$(dirname "${BASH_SOURCE[0]}")"

"$dir/build-linux.sh"
"$dir/build-macos.sh"
"$dir/build-windows.sh"

echo "🎉 todos os pacotes prontos — os compactados para enviar estão em dist/"
