# Spec: node — orquestração, config, RPC e CLI

> Domínio do node PANDA (ver [PLAN.md](../../PLAN.md)). É o overview do node:
> junta `params`+`chain`+`mempool`+`p2p`+`miner`+`wallet` num processo só,
> exposto pelo binário `cmd/node`. Wiring manual, sem DI — mesmo espírito de
> `cmd/api/main.go`.

## Conceito

O **full node** é a unidade de soberania da rede: ele valida tudo por conta
própria (não confia em ninguém), guarda a cadeia inteira, serve outros nós e
**minera por padrão** com 1 worker (decisão de produto: com PoW memory-hard a
segurança vem da quantidade de participantes — todo node contribuindo um
pouco é o que descentraliza; desligar é opt-out via `--mine=false`). "Rodar um node" é o ato que descentraliza a rede;
por isso o projeto inteiro existe para que este binário seja **um arquivo
estático que roda em qualquer máquina** (`CGO_ENABLED=0`), leve o bastante
para ficar ligado sem incomodar.

O processo tem 4 subsistemas ligados pelo `Node`: a **chain** (estado), o
**mempool** (fila), o **p2p** (rede) e o **miner** (opcional). O CLI conversa
com o node em execução via **RPC JSON em localhost** — necessário porque o
bbolt aceita um único processo escritor: o comando `node balance` não pode
abrir o banco enquanto o node roda, então pergunta ao node pela porta RPC.

## Decisões & porquês (regra e arquitetura)

`node` é a cola: não tem regra de consenso própria, mas decide *como os
subsistemas se ligam* e *como o dono controla o node*. As decisões são sobre
ciclo de vida seguro e uma superfície de controle que não vira buraco de
segurança.

- **Wiring manual, sem DI, sem Gin/GORM.** O node monta chain→mempool→p2p→miner à
  mão no `Node`, igual ao `cmd/api/main.go` do skeleton faz com seus serviços. Um
  container de injeção esconderia a ordem de dependência que aqui é justamente o
  que precisa estar explícito (e é o que a ordem de shutdown depende). O node vive
  fora do stack Gin/GORM de propósito: é um binário estático de consenso, não uma
  API web.
- **RPC em `net/http` da stdlib, bind exclusivo em loopback.** A RPC é a interface
  de *controle do dono* (ver saldo, enviar, desligar) — não uma API pública.
  Recusar bind fora de `127.0.0.1` por construção significa que expor o node à
  internet não vaza controle por acidente; não há autenticação justamente porque
  não há como alcançá-la de fora. Gin seria peso morto para meia dúzia de métodos
  locais.
- **CLI fala com o node por RPC porque o bbolt é single-writer.** Só um processo
  pode escrever no banco. Se `node balance` abrisse o bbolt diretamente,
  competiria com o `run` e um dos dois falharia (ou pior, corromperia). Perguntar
  pela porta RPC ao node em execução é o que permite consultar/enviar com o node
  ligado. É a razão de a RPC existir, não um luxo.
- **Envelope de erro `{code, message}` espelha o `apierror`, sem importá-lo.** O
  node não usa Gin, então não pode usar o `apierror` do skeleton; mas repetir o
  *formato* de erro mantém a consistência de resposta entre as duas metades do
  repo sem criar uma dependência do node no mundo HTTP. Convenção compartilhada,
  acoplamento não.
- **Minerar é default (opt-out `--mine=false`), e exige wallet.** Detalhado no
  `miner`, mas a consequência de orquestração mora aqui: se o node vai minerar por
  padrão, precisa de um endereço para a coinbase — então o primeiro `run` cria a
  wallet (0600) e loga o endereço. A alternativa (minerar desligado por default)
  significaria uma rede que não arranca sozinha; escolhemos que instalar o node
  já é contribuir.
- **Config em camadas: flag > arquivo `panda.conf` > env `NODE_*` > default.** A
  mesma chave pode vir de quatro lugares, com precedência clara. Flags para o
  ajuste pontual, arquivo para o setup persistente (menos flags repetidos), env
  para container/systemd, default para "só funciona". Espelha o `getEnv` do
  `internal/config` sem tocá-lo — o node tem sua config, mas segue o padrão da
  casa.
- **Shutdown em ordem estrita: p2p → miner → bbolt.** Fechar o banco antes de
  parar quem escreve nele (miner e p2p aceitando blocos) corromperia uma
  transação em voo. A ordem não é estética: parar as fontes de escrita primeiro e
  o banco por último é o que garante que o SIGINT durante um IBD deixe o disco
  consistente. bbolt ser transacional cobre o resto.
- **Processo foreground simples, sem daemon/systemd/TUI/métricas.** O node é um
  processo que você liga e vê o log; empacotar daemonização, TUI ou Prometheus
  agora seria resolver problemas que o node doméstico ainda não tem. São
  evoluções futuras declaradas, não ausências acidentais — a superfície mínima é
  mais fácil de manter e de auditar.

## Objetivo

Orquestrar os subsistemas com ciclo de vida limpo (start/stop), expor a RPC
local para o CLI e definir a configuração do node.

## Escopo

Entra:
- `config.go` — flags primeiro, env `NODE_*` como fallback (espelha o padrão
  `getEnv` de `internal/config/config.go`, sem tocá-lo):
  `--datadir`/`NODE_DATADIR` (default `~/.panda`), `--listen`/`NODE_LISTEN`
  (`:9551`), `--rpc`/`NODE_RPC` (`127.0.0.1:8555`), `--peers`/`NODE_PEERS`
  (lista separada por vírgula), `--mine`/`NODE_MINE` (**default true** —
  opt-out), `--miners`/`NODE_MINERS` (default 1), `--profile`/`NODE_PROFILE`
  (`devnet`)
- `node.go` — `Node` struct: abre chain, cria mempool, sobe p2p, sobe miner
  por padrão (a menos de `--mine=false`); se o datadir não tem wallet, o
  primeiro `run` cria uma (0600) e loga o endereço — minerar por padrão exige
  um destino para a coinbase; `Start`/`Stop` com graceful shutdown em
  SIGINT/SIGTERM (fechar p2p → miner → bbolt, nessa ordem)
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

- [x] `config.go`, `node.go`, `rpc.go` + `cmd/node/main.go` e testes do que
      tem lógica (config parsing, handlers RPC com chain de teste)
- [x] Demo do PLAN.md funciona — virou o teste de integração
      `TestDemoTwoNodes`: 2 nodes in-process, B sincroniza de A, send de A
      aparece no saldo de B após confirmação
- [x] Stop fecha o bbolt limpo (teste reabre a chain sem erro; `run` liga o
      SIGINT nesse mesmo caminho)
- [x] RPC só em loopback; tentativa de bind externo falha com mensagem
- [x] `CGO_ENABLED=0 go build -o bin/panda-node ./cmd/node` produz binário
      estático
- [x] Cross-compile verde para linux/darwin/windows em amd64 e arm64
      (Raspberry Pi é cidadão de primeira classe)
- [ ] Node ocioso (sem minerar) estável abaixo de ~128 MiB de RSS; minerando
      com 1 worker, ~+64 MiB — medir numa sessão longa de devnet (não
      verificável em teste unitário)

## Fora de escopo / não fazer

- Sem autenticação de RPC nesta versão (é localhost-only por construção)
- Sem daemonização/systemd, sem TUI — processo foreground simples
- Sem métricas/prometheus (evolução futura)
