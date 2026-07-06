#!/usr/bin/env bash
# build-common.sh — biblioteca compartilhada dos scripts de build.
# Não execute direto: os scripts build-linux.sh / build-macos.sh /
# build-windows.sh fazem `source` daqui.
#
# Lê o build.conf da raiz do repo (ou o build.conf.example como fallback)
# e expõe a função build_target GOOS GOARCH.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# defaults (mesmos do build.conf.example)
NAME="panda-node"
OUTDIR="dist"
VERSION="dev"
# regras de consenso do build (vazio = perfil devnet padrão do código)
SPACING=""
HALVING=""
SUBSIDY=""
RETARGET=""
PROFILE=""

conf="$ROOT/build.conf"
[ -f "$conf" ] || conf="$ROOT/build.conf.example"
if [ -f "$conf" ]; then
  while IFS='=' read -r key value; do
    key="$(echo "$key" | tr -d '[:space:]')"
    value="$(echo "$value" | sed 's/[[:space:]]*$//;s/^[[:space:]]*//')"
    case "$key" in
      name)    NAME="$value" ;;
      outdir)  OUTDIR="$value" ;;
      version) VERSION="$value" ;;
      spacing) SPACING="$value" ;;
      halving) HALVING="$value" ;;
      subsidy)  SUBSIDY="$value" ;;
      retarget) RETARGET="$value" ;;
      profile)  PROFILE="$value" ;;
      ''|\#*)  ;; # linha vazia ou comentário
    esac
  done < <(grep -Ev '^[[:space:]]*(#|$)' "$conf")
fi

# Monta os -ldflags: versão sempre; regras de consenso só se definidas.
# Regras diferentes mudam o gênesis => o binário forma uma REDE própria.
LDFLAGS="-s -w -X main.version=$VERSION"
RULES=""
[ -n "$SPACING" ] && LDFLAGS="$LDFLAGS -X pandabk_coin/internal/params.buildSpacing=$SPACING" && RULES="$RULES spacing=$SPACING"
[ -n "$HALVING" ] && LDFLAGS="$LDFLAGS -X pandabk_coin/internal/params.buildHalving=$HALVING" && RULES="$RULES halving=$HALVING"
[ -n "$SUBSIDY" ] && LDFLAGS="$LDFLAGS -X pandabk_coin/internal/params.buildSubsidy=$SUBSIDY" && RULES="$RULES subsidy=$SUBSIDY"
[ -n "$RETARGET" ] && LDFLAGS="$LDFLAGS -X pandabk_coin/internal/params.buildRetarget=$RETARGET" && RULES="$RULES retarget=$RETARGET"
[ -n "$PROFILE" ] && LDFLAGS="$LDFLAGS -X pandabk_coin/internal/node.DefaultProfile=$PROFILE" && RULES="$RULES profile=$PROFILE"

mkdir -p "$ROOT/$OUTDIR"

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1 # macOS
  fi
}

build_target() {
  local goos="$1" goarch="$2" ext=""
  [ "$goos" = "windows" ] && ext=".exe"
  local out="$ROOT/$OUTDIR/$NAME-$goos-$goarch$ext"
  echo "→ $goos/$goarch"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -C "$ROOT" -trimpath \
    -ldflags "$LDFLAGS" \
    -o "$out" ./cmd/node
  echo "  $out"
  echo "  sha256 $(sha256 "$out")"
}

echo "🐼 build $NAME versão $VERSION → $OUTDIR/ (config: ${conf#"$ROOT"/})"
if [ -n "$RULES" ]; then
  echo "⚠️  regras de consenso do build:$RULES"
  echo "   gênesis próprio => este binário forma uma rede SEPARADA (só fala com builds das mesmas regras)"
fi
