# Zhu

Full node standalone da criptomoeda Zhu — proof-of-work **memory-hard
(Argon2id)**: minerar usa 1 core e ~64 MiB de RAM, então um notebook usado
ou um Raspberry Pi competem de igual para igual, sem ASICs nem fazendas. O
princípio que decide todo trade-off do projeto: **um node em cada casa**.

> ⚠️ **Isto é uma devnet.** A rede atual é de desenvolvimento: os parâmetros
> econômicos ainda podem mudar e a chain pode ser reiniciada. Não trate Zhu
> de devnet como dinheiro.

Documentação web (identidade da rede, arquitetura, referência): rode `bin/zhu
docs` — sobe a doc em localhost e abre o navegador. Também disponível como
página estática em [docs/index.html](./docs/index.html); referência de
comandos em [docs/README.md](./docs/README.md). Guia narrado para
iniciantes: [TUTORIAL.md](./TUTORIAL.md). Plano e decisões de design em
[PLAN.md](./PLAN.md); visão em [PROPOSTA.md](./PROPOSTA.md); marca em
[BRAND.md](./BRAND.md); convenções de código em [CLAUDE.md](./CLAUDE.md).

## Requisitos

- Go 1.25+

## Build

```bash
CGO_ENABLED=0 go build -o bin/zhu ./cmd/node
```

Binário estático, sem dependências — cross-compile para linux/darwin/windows,
amd64+arm64 (Raspberry Pi incluso).

## Rodando

```bash
bin/zhu run -profile devnet   # sobe o node (minera por padrão)
```

O primeiro `run` cria a wallet do datadir (`~/.zhu` por padrão) e já começa a
minerar com 1 worker — desligue com `-mine=false`. Configuração via flags,
arquivo `zhu.conf` ou env `NODE_*` — veja o guia completo em
[docs/README.md](./docs/README.md#5-configuração) ou o
[TUTORIAL.md](./TUTORIAL.md).

## Documentação da Zhu (web)

A documentação técnica da **Zhu Network** — identidade da rede, arquitetura
dos pacotes, consenso PoW memory-hard, referência de RPC/CLI e economia — é
uma página única com a marca do projeto (`docs/index.html`). Um comando sobe
ela em localhost e abre o navegador:

```bash
CGO_ENABLED=0 go build -o bin/zhu ./cmd/node
bin/zhu docs                 # http://127.0.0.1:8600 + abre o navegador
```

Flags: `-addr` muda a porta (loopback), `-open=false` só serve sem abrir o
navegador, `-dir` aponta outra pasta. O comando procura o `docs/index.html`
a partir do diretório atual (e dos diretórios acima), então roda a partir de
qualquer subpasta do repositório. Sem o binário, é só abrir
`docs/index.html` direto no navegador. O diagrama de arquitetura editável
fica em [`docs/arquitetura-zhu.excalidraw`](./docs/arquitetura-zhu.excalidraw).

## Testes

```bash
go test ./...
go test -race ./...
```

## App de desktop

Existe também um app de desktop (`scripts/build-desktop.sh` / `cmd/desktop`)
— mesma coisa que os comandos acima, numa interface nativa; e se não houver
node rodando, o próprio app vira o node. Detalhes em
[docs/README.md, seção 13](./docs/README.md#13-app-de-desktop).
