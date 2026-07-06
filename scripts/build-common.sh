#!/usr/bin/env bash
# build-common.sh — biblioteca compartilhada dos scripts de build.
# Não execute direto: os scripts build-linux.sh / build-macos.sh /
# build-windows.sh fazem `source` daqui.
#
# Lê o build.conf da raiz do repo (ou o build.conf.example como fallback)
# e expõe as funções de empacotamento: cada script de SO monta um pacote
# completo em dist/<so>/ (binários + panda.conf + instalador + LEIA-ME +
# SHA256SUMS) e gera o compactado versionado dist/panda-<versão>-<so>.tar.gz
# pronto para mandar para a turma.
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

echo "🐼 build $NAME versão $VERSION → $OUTDIR/ (config: ${conf#"$ROOT"/})"
if [ -n "$RULES" ]; then
  echo "⚠️  regras de consenso do build:$RULES"
  echo "   gênesis próprio => este binário forma uma rede SEPARADA (só fala com builds das mesmas regras)"
fi

# ── empacotamento por sistema ───────────────────────────────────────────────

# pkg_init <so>: começa um pacote limpo em dist/<so>/
pkg_init() {
  PKGLABEL="$1"
  PKGDIR="$ROOT/$OUTDIR/$PKGLABEL"
  rm -rf "$PKGDIR"
  mkdir -p "$PKGDIR"
}

# build_node GOOS GOARCH NOME — binário estático do node dentro do pacote
build_node() {
  local goos="$1" goarch="$2" name="$3"
  echo "→ node $goos/$goarch"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -C "$ROOT" -trimpath \
    -ldflags "$LDFLAGS" \
    -o "$PKGDIR/$name" ./cmd/node
}

# build_desktop GOOS GOARCH NOME — a GUI usa cgo, que não cruza de SO com o
# toolchain normal. A função escolhe o caminho sozinha:
#   - host é o próprio SO → build nativo (no macOS o Xcode cruza arm64/amd64)
#   - alvo Linux/Windows em outro host → container Docker com os toolchains
#     (scripts/desktop-cross.Dockerfile); Windows sai do MinGW dentro dele
#   - sem Docker → avisa e o pacote sai sem o desktop (nunca aborta o build)
build_desktop() {
  local goos="$1" goarch="$2" name="$3"
  local hostos hostarch
  hostos="$(go env GOOS)"
  hostarch="$(go env GOARCH)"

  if [ "$goos" = "$hostos" ] && { [ "$goarch" = "$hostarch" ] || [ "$hostos" = "darwin" ]; }; then
    echo "→ desktop $goos/$goarch (cgo nativo)"
    CGO_ENABLED=1 GOARCH="$goarch" go build -C "$ROOT" -trimpath \
      -ldflags "$LDFLAGS" \
      -o "$PKGDIR/$name" ./cmd/desktop
    return 0
  fi
  if [ "$goos" = "darwin" ]; then
    echo "⚠️  desktop de macOS só builda num Mac — este pacote sai sem ele."
    return 0
  fi
  if ! docker info >/dev/null 2>&1; then
    echo "⚠️  desktop $goos/$goarch fica de fora: o cross-compile precisa de Docker"
    echo "   ativo (ex.: colima start) — ou rode scripts/build-$PKGLABEL.sh num $goos."
    return 0
  fi

  # O container roda SEMPRE nativo no arch do host (emular o toolchain do
  # Go segfaulta); quem cruza para o arch/SO alvo é o gcc de dentro dele.
  # As libs X11/GL de arquiteturas diferentes conflitam → uma imagem por
  # arch alvo (LIBARCH).
  local cc="gcc" extra="" libarch="$goarch"
  if [ "$goos" = "windows" ]; then
    cc="x86_64-w64-mingw32-gcc"
    extra=" -H windowsgui" # GUI de verdade: sem janela de console ao abrir
    libarch="$hostarch"    # MinGW não usa X11; imagem nativa serve
  elif [ "$goarch" != "$hostarch" ]; then
    case "$goarch" in
      amd64) cc="x86_64-linux-gnu-gcc" ;;
      arm64) cc="aarch64-linux-gnu-gcc" ;;
    esac
  fi
  echo "→ desktop $goos/$goarch (cgo via Docker, CC=$cc — a 1ª vez baixa imagem e pacotes)"
  if ! docker build --build-arg "LIBARCH=$libarch" -t "panda-desktop-cross-$libarch" \
      -f "$ROOT/scripts/desktop-cross.Dockerfile" "$ROOT/scripts"; then
    echo "⚠️  imagem de cross-compile falhou — pacote sai sem o desktop $goos/$goarch"
    return 0
  fi
  if ! docker run --rm \
      -v "$ROOT":/src -w /src \
      -v panda-go-mod:/go/pkg/mod -v panda-go-build:/root/.cache/go-build \
      -e CGO_ENABLED=1 -e GOOS="$goos" -e GOARCH="$goarch" -e CC="$cc" \
      "panda-desktop-cross-$libarch" \
      go build -trimpath -ldflags "$LDFLAGS$extra" \
      -o "/src/$OUTDIR/$PKGLABEL/$name" ./cmd/desktop; then
    echo "⚠️  cross-compile falhou — pacote sai sem o desktop $goos/$goarch"
    return 0
  fi
}

# pkg_finish — panda.conf + VERSAO.txt + LEIA-ME + SHA256SUMS + compactado
# versionado. Chamar por último, depois de todos os build_*.
pkg_finish() {
  cp "$ROOT/panda.conf.example" "$PKGDIR/panda.conf"
  write_versao
  write_leiame
  write_sums
  make_archive
  echo "📦 pacote:      $OUTDIR/$PKGLABEL/"
  echo "📦 para enviar: $OUTDIR/panda-$VERSION-$PKGLABEL.$ARCHIVE_EXT"
}

write_versao() {
  {
    echo "PANDA Coin — versão $VERSION"
    [ -n "$RULES" ] && echo "regras de consenso:$RULES"
    echo "build: $(date -u +%Y-%m-%dT%H:%M:%SZ) em $(go env GOOS)/$(go env GOARCH), $(go env GOVERSION)"
    if git -C "$ROOT" rev-parse --short HEAD >/dev/null 2>&1; then
      echo "commit: $(git -C "$ROOT" rev-parse --short HEAD)"
    fi
  } > "$PKGDIR/VERSAO.txt"
}

write_leiame() {
  local desktop_line="  (o app de janela não veio neste pacote — peça a quem buildou)"
  if ls "$PKGDIR"/panda-desktop* >/dev/null 2>&1; then
    desktop_line="  panda-desktop-*       — o mesmo node, com janela (o instalador escolhe)"
  fi
  cat > "$PKGDIR/LEIA-ME.txt" <<EOF
PANDA Coin — versão $VERSION
============================

O que tem aqui:
  panda-node-*          — o node completo (o instalador escolhe o da sua CPU)
$desktop_line
  panda.conf            — configuração: coloque em peers o endereço de quem te convidou
  instalar.*            — prepara o binário certo para a sua máquina
  SHA256SUMS.txt        — para conferir que os arquivos chegaram íntegros
  VERSAO.txt            — versão e regras deste build

Como começar:
  1. rode o instalador   (Linux/macOS: ./instalar.sh | Windows: instalar.bat)
  2. edite panda.conf    (a linha peers= é a que conecta você na rede)
  3. ./panda-node run    — pronto: seu node valida e minera
     ou abra o panda-desktop, se preferir janela

IMPORTANTE — todos na rede precisam do MESMO build. As regras de consenso
ficam carimbadas no bloco gênese: binário de outro build forma OUTRA rede
e a conexão é recusada ("gênesis diferente"). Na dúvida, compare o
SHA256SUMS.txt com o de quem te enviou o pacote.
EOF
}

write_sums() {
  (
    cd "$PKGDIR"
    : > SHA256SUMS.txt
    for f in *; do
      [ "$f" = "SHA256SUMS.txt" ] && continue
      echo "$(sha256 "$f")  $f" >> SHA256SUMS.txt
    done
  )
}

# make_archive — compactado com pasta raiz versionada (panda-<v>-<so>/...):
# copia o pacote para o nome final e comprime (portável entre BSD/GNU tar).
make_archive() {
  local base="panda-$VERSION-$PKGLABEL"
  local staged="$ROOT/$OUTDIR/$base"
  rm -rf "$staged"
  cp -R "$PKGDIR" "$staged"
  if [ "$PKGLABEL" = "windows" ] && command -v zip >/dev/null 2>&1; then
    ARCHIVE_EXT="zip"
    rm -f "$ROOT/$OUTDIR/$base.zip"
    (cd "$ROOT/$OUTDIR" && zip -qr "$base.zip" "$base")
  else
    ARCHIVE_EXT="tar.gz"
    tar -czf "$ROOT/$OUTDIR/$base.tar.gz" -C "$ROOT/$OUTDIR" "$base"
  fi
  rm -rf "$staged"
}

# write_installer_sh <macos: sim|nao> — instalador de Linux/macOS: escolhe
# o binário da CPU local, dá permissão e (macOS) tira a quarentena.
write_installer_sh() {
  local mac="$1"
  cat > "$PKGDIR/instalar.sh" <<'EOF'
#!/bin/sh
# Prepara o PANDA nesta máquina: escolhe os binários da sua CPU e dá permissão.
set -e
cd "$(dirname "$0")"
case "$(uname -m)" in
  x86_64)        arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "CPU $(uname -m) sem binário neste pacote"; exit 1 ;;
esac
cp -f "panda-node-$arch" panda-node
chmod +x panda-node
if [ -f "panda-desktop-$arch" ]; then
  cp -f "panda-desktop-$arch" panda-desktop
  chmod +x panda-desktop
fi
EOF
  if [ "$mac" = "sim" ]; then
    cat >> "$PKGDIR/instalar.sh" <<'EOF'
# macOS marca downloads com quarentena; sem isso o Gatekeeper bloqueia
xattr -d com.apple.quarantine panda-node panda-desktop 2>/dev/null || true
EOF
  fi
  cat >> "$PKGDIR/instalar.sh" <<'EOF'
echo "✅ pronto. Edite o panda.conf (linha peers=) e rode:  ./panda-node run"
EOF
  chmod +x "$PKGDIR/instalar.sh"
}

# write_installer_bat — o equivalente para Windows.
write_installer_bat() {
  cat > "$PKGDIR/instalar.bat" <<'EOF'
@echo off
rem Prepara o PANDA nesta maquina: escolhe o binario da sua CPU.
cd /d "%~dp0"
if "%PROCESSOR_ARCHITECTURE%"=="ARM64" (
  copy /y panda-node-arm64.exe panda-node.exe >nul
) else (
  copy /y panda-node-amd64.exe panda-node.exe >nul
)
if exist panda-desktop-amd64.exe copy /y panda-desktop-amd64.exe panda-desktop.exe >nul
echo pronto. Edite o panda.conf (linha peers=) e rode:  panda-node.exe run
EOF
}
