# Spec: node — orquestração, config, RPC e CLI

> Domínio do node PANDA (ver [PLAN.md](../../PLAN.md)). É o overview do node:
> junta `params`+`chain`+`mempool`+`p2p`+`miner`+`wallet` num processo só,
> exposto pelo binário `cmd/node`. Wiring manual, sem DI — mesmo espírito de
> `cmd/api/main.go`.

## Conceito

O **full node** é a unidade de soberania da rede: ele valida tudo por conta
própria (não confia em ninguém), guarda a cadeia inteira, serve outros nós e
— opcionalmente — minera. "Rodar um node" é o ato que descentraliza a rede;
por isso o projeto inteiro existe para que este binário seja **um arquivo
estático que roda em qualquer máquina** (`CGO_ENABLED=0`), leve o bastante
para ficar ligado sem incomodar.

O processo tem 4 subsistemas ligados pelo `Node`: a **chain** (estado), o
**mempool** (fila), o **p2p** (rede) e o **miner** (opcional). O CLI conversa
com o node em execução via **RPC JSON em localhost** — necessário porque o
bbolt aceita um único processo escritor: o comando `node balance` não pode
abrir o banco enquanto o node roda, então pergunta ao node pela porta RPC.

## Objetivo

Orquestrar os subsistemas com ciclo de vida limpo (start/stop), expor a RPC
local para o CLI e definir a configuração do node.

## Escopo

Entra:
- `config.go` — flags primeiro, env `NODE_*` como fallback (espelha o padrão
  `getEnv` de `internal/config/config.go`, sem tocá-lo):
  `--datadir`/`NODE_DATADIR` (default `~/.panda`), `--listen`/`NODE_LISTEN`
  (`:9551`), `--rpc`/`NODE_RPC` (`127.0.0.1:8555`), `--peers`/`NODE_PEERS`
  (lista separada por vírgula), `--mine`/`NODE_MINE`, `--miners`/`NODE_MINERS`
  (default 1), `--profile`/`NODE_PROFILE` (`devnet`)
- `node.go` — `Node` struct: abre chain, cria mempool, sobe p2p, sobe miner se
  `--mine`; `Start`/`Stop` com graceful shutdown em SIGINT/SIGTERM (fechar
  p2p → miner → bbolt, nessa ordem)
- `rpc.go` — `net/http` stdlib (sem Gin), **bind exclusivo em localhost**:
  `getinfo` (altura, tip, peers, mempool, hashrate), `getbalance(address)`,
  `getnewutxos(address)` (para o wallet build), `sendrawtx(hex)`,
  `sendtoaddress(to, amount)` (usa a wallet do datadir)
- Subcomandos do `cmd/node` (`flag.NewFlagSet` por subcomando): `run`,
  `wallet new`, `wallet address`, `balance`, `send`, `info`, `genesis`
  (dev-only: minera o gênesis)

Fica de fora:
- RPC autenticada/exposta externamente (localhost only nesta versão)
- Explorer/HTTP público (pode vir do skeleton Gin no futuro — fora deste node)

## Modelo de dados

N/A — orquestração. Config é struct em memória.

## Regras de negócio

- RPC **recusa bind fora de loopback** nesta versão (é a interface de
  controle do dono do node, não uma API pública)
- `send` sem wallet no datadir → erro claro instruindo `node wallet new`
- Shutdown: nunca fechar o bbolt antes de parar quem escreve (miner/p2p)
- `--mine` exige wallet no datadir (endereço da coinbase)

## Interface (CLI)

```
node run     --profile devnet --datadir ~/.panda/n1 --listen :9551 --rpc 127.0.0.1:8551 [--mine] [--miners 1] [--peers host:port,...]
node wallet new|address --datadir ...
node info    --rpc 127.0.0.1:8551
node balance --rpc ... [--address P...]
node send    --rpc ... --to P... --amount 1.5
node genesis --profile devnet          # dev-only, minera e imprime o gênesis
```

RPC: POST JSON em `/rpc` `{"method": "...", "params": {...}}` → resposta
`{"result": ...}` ou `{"error": {"code", "message"}}` (mesmo espírito de
envelope do apierror, sem importá-lo).

## Casos de erro / edge cases

- Datadir travado por outro processo (bbolt lock) → mensagem clara, exit 1
- Porta em uso (listen/rpc) → mensagem clara, exit 1
- CLI sem node rodando (RPC recusada) → "node não está rodando em <addr>?"
- SIGINT durante IBD → shutdown limpo sem corromper o banco (transações bbolt)

## Critérios de aceite

- [ ] `config.go`, `node.go`, `rpc.go` + `cmd/node/main.go` e testes do que
      tem lógica (config parsing, handlers RPC com chain de teste)
- [ ] Demo do PLAN.md funciona: 2 nodes na mesma máquina, B sincroniza de A,
      send de A aparece no saldo de B após confirmação
- [ ] SIGINT fecha o bbolt limpo (reabrir sem erro)
- [ ] RPC só em loopback; tentativa de bind externo falha com mensagem
- [ ] `CGO_ENABLED=0 go build -o bin/node ./cmd/node` produz binário estático
- [ ] Cross-compile verde para linux/darwin/windows em amd64 e arm64
      (`GOOS=linux GOARCH=arm64 go build ./cmd/node` — Raspberry Pi é
      cidadão de primeira classe, ver princípio "um node em cada casa")
- [ ] Node ocioso (sem minerar) estável abaixo de ~128 MiB de RSS; minerando
      com 1 worker, ~+64 MiB

## Fora de escopo / não fazer

- Sem autenticação de RPC nesta versão (é localhost-only por construção)
- Sem daemonização/systemd, sem TUI — processo foreground simples
- Sem métricas/prometheus (evolução futura)
