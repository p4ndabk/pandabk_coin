#!/usr/bin/env bash
# Builda o PANDA Desktop (GUI nativa em Go/Fyne) para a MÁQUINA ATUAL.
#
# A GUI usa cgo (renderização nativa), então NÃO cross-compila como o node —
# builde em cada sistema (ou use fyne-cross com Docker, veja a documentação).
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
out="$ROOT/$OUTDIR/$NAME-desktop-$goos-$goarch$ext"

echo "→ desktop $goos/$goarch (cgo)"
CGO_ENABLED=1 go build -C "$ROOT" -trimpath \
  -ldflags "$LDFLAGS" \
  -o "$out" ./cmd/desktop
echo "  $out"
echo "  sha256 $(sha256 "$out")"
echo "✅ Desktop pronto para $goos. Para os outros sistemas, rode este script lá (ou fyne-cross)."
