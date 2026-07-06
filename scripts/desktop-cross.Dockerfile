# Imagem de cross-compile do PANDA Desktop (Fyne usa cgo, que não cruza de
# SO com o toolchain normal). Roda SEMPRE nativa no arch do host (emular o
# toolchain do Go segfaulta); quem cruza é o gcc da Debian.
#
# Base debian-slim de propósito: a imagem golang oficial vem com dezenas de
# pacotes -dev pré-instalados que CONFLITAM com as versões :amd64/:arm64 do
# multiarch — aqui o apt parte do zero e só entra o necessário.
#
# LIBARCH escolhe a arquitetura das bibliotecas de X11/OpenGL que o Fyne
# linka (GL, X11, Xcursor, Xrandr, Xinerama, Xi, Xxf86vm, xkbcommon) — os
# -dev de arquiteturas diferentes não são co-instaláveis, então é uma
# imagem POR arch alvo (os scripts buildam com --build-arg LIBARCH=...).
# O MinGW (desktop de Windows) não precisa de X11 e vem em todas.
# Não é preciso buildar/rodar à mão — scripts/build-linux.sh e
# build-windows.sh cuidam de tudo.
FROM debian:bookworm-slim
COPY --from=golang:1.25-bookworm /usr/local/go /usr/local/go
ENV PATH=/usr/local/go/bin:$PATH GOPATH=/go
ARG LIBARCH=amd64
# O cross-gcc só existe para arch DIFERENTE do host (ninguém cruza para si
# mesmo) — instala condicionalmente o do LIBARCH quando ele difere.
RUN dpkg --add-architecture "$LIBARCH" \
    && apt-get update \
    && cross=""; native="$(dpkg --print-architecture)"; \
    if [ "$LIBARCH" != "$native" ]; then \
      case "$LIBARCH" in \
        amd64) cross="gcc-x86-64-linux-gnu" ;; \
        arm64) cross="gcc-aarch64-linux-gnu" ;; \
      esac; \
    fi; \
    apt-get install -y --no-install-recommends \
      gcc libc6-dev gcc-mingw-w64-x86-64 $cross \
      "libc6-dev:$LIBARCH" \
      "libgl1-mesa-dev:$LIBARCH" "libx11-dev:$LIBARCH" \
      "libxcursor-dev:$LIBARCH" "libxrandr-dev:$LIBARCH" \
      "libxinerama-dev:$LIBARCH" "libxi-dev:$LIBARCH" \
      "libxxf86vm-dev:$LIBARCH" "libxkbcommon-dev:$LIBARCH" \
    && rm -rf /var/lib/apt/lists/*
