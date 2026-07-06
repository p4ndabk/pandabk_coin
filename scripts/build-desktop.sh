#!/usr/bin/env bash
# Builda SÓ o PANDA Desktop (GUI nativa em Go/Fyne) para a MÁQUINA ATUAL —
# atalho de desenvolvimento. O pacote de distribuição completo (que já
# inclui o desktop quando buildado no próprio SO) sai dos scripts
# build-linux.sh / build-macos.sh / build-windows.sh.
#
# A GUI usa cgo (renderização nativa), então NÃO cross-compila como o node.
# Pré-requisitos: macOS = Xcode Command Line Tools; Linux = gcc,
# libgl1-mesa-dev, xorg-dev; Windows = MinGW-w64.
#
# Reusa o build.conf: versão E as regras de consenso (o node embutido do
# desktop obedece às mesmas regras do panda-node do mesmo build).
#
# Uso:  scripts/build-desktop.sh
source "$(dirname "${BASH_SOURCE[0]}")/build-common.sh"

goos="$(go env GOOS)"
goarch="$(go env GOARCH)"
ext=""
[ "$goos" = "windows" ] && ext=".exe"
out="$ROOT/$OUTDIR/panda-desktop-$goos-$goarch$ext"

echo "→ desktop $goos/$goarch (cgo)"
CGO_ENABLED=1 go build -C "$ROOT" -trimpath \
  -ldflags "$LDFLAGS" \
  -o "$out" ./cmd/desktop
echo "  $out"
echo "  sha256 $(sha256 "$out")"
label="$goos"
[ "$goos" = "darwin" ] && label="macos"
echo "✅ Desktop pronto para $goos. Pacote completo de cliente: scripts/build-$label.sh"
