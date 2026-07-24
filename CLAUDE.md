# Zhu — full node em Go

Full node standalone da criptomoeda Zhu — proof-of-work **memory-hard
(Argon2id)**, storage próprio em bbolt, **sem Gin nem GORM**, build estático
(`CGO_ENABLED=0`). Plano e decisões de design: [PLAN.md](./PLAN.md); guia de
uso: [TUTORIAL.md](./TUTORIAL.md).

Module path: `zhu`

## Arquitetura

- Pacotes (dependência estrita `params ← core ← pow ← chain ← {mempool,
  wallet} ← {p2p, miner} ← node ← cmd/node`): cada um com `SPEC.md` próprio
  (seção "Conceito" didática — o usuário aprende blockchain pelo projeto).
- Consenso: PoW Argon2id memory-hard (ID do bloco = SHA-256d ≠ hash de PoW),
  UTXO, retarget estilo Bitcoin, MaxBlockSize 256 KiB como regra de consenso.
- **Todo node minera por padrão** (1 worker, ~64 MiB); opt-out `-mine=false`.
  A coinbase paga a wallet do datadir (criada no primeiro `run`).
- RPC JSON localhost-only em `/rpc` para a CLI (`info`/`balance`/`send`) —
  bbolt é single-writer, a CLI nunca abre o banco de um node em execução.
- Config: flags > arquivo `zhu.conf` (chave=valor, mesmas chaves dos
  flags) > env `NODE_*` > defaults.
- Testes seguem o padrão do repo (`service_test.go`-equivalente por pacote,
  `go test -race` verde); princípio de produto: "um node em cada casa" —
  todo trade-off se decide pela régua do node doméstico.

```
CGO_ENABLED=0 go build -o bin/zhu ./cmd/node
bin/zhu run -profile devnet          # sobe o node (minera por padrão)
bin/zhu info|balance|send            # CLI via RPC localhost
bin/zhu powdemo                      # bancada didática de mineração
```

## Spec-first para novos pacotes

[BASE_SPEC.md](./BASE_SPEC.md) é o template de spec para um novo pacote do
node. Antes de implementar um novo `internal/<domínio>/`, copie
`BASE_SPEC.md` para `internal/<domínio>/SPEC.md` e preencha (objetivo,
escopo, modelo de dados, regras de negócio, interface do pacote/CLI, casos
de erro, critérios de aceite, não-objetivos explícitos). Veja
`internal/p2p/SPEC.md` ou `internal/chain/SPEC.md` como exemplo de como fica
depois de adaptado — sempre com uma seção "Conceito" didática.

## Commands

```
CGO_ENABLED=0 go build -o bin/zhu ./cmd/node   # build estático do node
go vet ./...
go test ./...
go test -race ./...
```

## Docs

`bin/zhu docs` sobe a documentação web (identidade da rede, arquitetura,
consenso, referência de RPC/CLI) em localhost e abre o navegador — página
estática em [docs/index.html](./docs/index.html), fonte em
[docs/README.md](./docs/README.md). Diagrama de arquitetura editável em
[docs/arquitetura-zhu.excalidraw](./docs/arquitetura-zhu.excalidraw).
