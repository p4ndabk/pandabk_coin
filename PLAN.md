# PANDA Coin — blockchain PoW memory-hard em Go

> Plano de implementação do node PANDA Coin. Nada daqui foi implementado ainda —
> este documento registra as decisões de design e a ordem de execução (M1–M5).
> A visão do projeto em prosa, para leitura/áudio, está em
> [PROPOSTA.md](PROPOSTA.md).

## Contexto

O objetivo é uma criptomoeda seguindo os paradigmas do Bitcoin (proof of work,
halving, descentralização), mas com um diferencial central: **mineração acessível
a qualquer máquina comum**, com baixo consumo de energia e sem centralização em
ASICs/fazendas. A resposta técnica é um PoW **memory-hard (Argon2id)**: o gargalo
vira banda de memória RAM (commodity) em vez de FLOPS, então um notebook compete
de igual pra igual e ASIC não compensa — mesma filosofia do RandomX do Monero.

O repo atual é o skeleton base Gin+GORM (módulo `pandabk_coin` — o CLAUDE.md
ainda cita o path antigo). **Decisão**: o node é um binário standalone
`cmd/node`, sem Gin nem GORM, com storage próprio. O skeleton existente fica
intocado e compilando (pode virar block explorer no futuro).

## Princípio norteador: um node em cada casa

A meta do projeto é ser **a rede mais descentralizada possível**: qualquer
pessoa na Terra deve conseguir rodar um full node em casa — no notebook usado,
no Raspberry Pi, no PC antigo — sem sentir. Descentralização não é discurso, é
orçamento de recursos. Todo trade-off técnico se decide por esta régua: *"isso
deixa mais fácil ou mais difícil ter um node em casa?"*

Compromissos concretos que derivam disso:

| Recurso | Orçamento | Como |
|---|---|---|
| Instalação | 1 binário estático, zero dependências | `CGO_ENABLED=0`; cross-compile linux/darwin/windows, amd64+arm64 (Raspberry Pi) |
| CPU | 1 core em uso moderado (minerando) | Argon2id memory-hard + default de 1 worker; sem corrida armamentista de hardware |
| RAM | node ocioso < ~128 MiB; +64 MiB por worker minerando | matriz do Argon2 é o maior consumidor, e é limitada por config |
| Disco | crescimento previsível e baixo | **MaxBlockSize = 256 KiB como regra de consenso** (pior caso ~13 GB/ano no perfil mainnet de 10 min; blocos reais ficam muito abaixo) |
| Banda | conexão doméstica comum | gossip por `inv`/`getdata` (nada trafega duas vezes), headers de 96 B no sync |
| Rede doméstica (NAT) | funciona atrás de roteador **sem configurar nada** | node outbound-only é cidadão pleno: valida tudo, minera e propaga via conexões de saída; aceitar conexões de entrada (port forward) é opcional e só ajuda a rede — NAT traversal automático é roadmap |

## Decisões de produto

- **Rede P2P multi-nó desde o início** (descoberta de peers, sync, propagação, fork choice por trabalho acumulado)
- **Todo node minera por padrão** (decisão 2026-07-03): `node run` sobe o miner
  com 1 worker pagando a coinbase à wallet do datadir (criada automaticamente
  no primeiro run); desligar é opt-out explícito (`--mine=false`/`NODE_MINE=0`).
  Com PoW memory-hard a segurança vem da QUANTIDADE de participantes, não da
  potência de cada um — "um node em cada casa" implica "cada casa minera um
  pouco"
- **PoW: Argon2id** via `golang.org/x/crypto/argon2` (já no go.mod) — pure Go, `CGO_ENABLED=0`
- **Binário standalone `cmd/node`** com storage próprio
- **Economia dev/ciclos curtos, configurável**: bloco ~60s, halving a cada 1.000 blocos, recompensa inicial 50 PANDA (5e9 subunidades, 1 PANDA = 1e8), retarget a cada 100 blocos → supply máximo ~100.000 PANDA. Tudo num pacote `params` com perfis (devnet agora, mainnet 10min depois)

## Decisões técnicas (uma escolha por ponto)

| Ponto | Escolha | Por quê |
|---|---|---|
| Modelo de transação | **UTXO** (Bitcoin-like) | Casa com PoW/coinbase, verificação stateless, referência mais documentada |
| Assinaturas | **stdlib `crypto/ecdsa` P-256** (SignASN1, pubkey comprimida 33B) | Zero dependência nova, pure Go |
| Hash PoW vs ID | **Argon2id(header 96B, salt fixo `"pandabk/pow/v1"`) < target**; **ID do bloco = SHA-256d(header)** | Separar ID barato do hash caro é o padrão de moedas memory-hard (Monero) — o índice nunca recomputa Argon2 |
| Params Argon2id (devnet) | mem=64MiB, time=1, threads=1, keyLen=32 (~100–200ms/hash); perfil de teste com 1MiB p/ testes rápidos | Sub-segundo em laptop, memory-hard de verdade, 1 worker ≈ 64MiB |
| Dificuldade | **nBits compacto (uint32)** no header, expandido p/ `big.Int`; trabalho = `2^256/(target+1)` | Formato fixo bem entendido |
| Retarget | **Estilo Bitcoin: a cada 100 blocos, clamp 4×/¼×** | Simples e testável; LWMA fica como non-goal |
| Storage | **bbolt (`go.etcd.io/bbolt`)** — única dependência nova | Pure Go, single-file, ACID, shape KV exato (blocos por hash, UTXO set) |
| Serialização | **Binário canônico hand-rolled** em `core` (hashing determinístico); envelope P2P = JSON com length-prefix 4B (frame máx 1MiB) | gob/JSON não são determinísticos p/ consenso; JSON no wire é debugável |
| P2P | **TCP puro + length-prefix + JSON** | libp2p contradiz "deps mínimas / binário estático" |
| Wallet/CLI | **Um binário só**: `node run`, `node wallet new`, `node balance`, `node send`, `node info`; comandos falam com o node via **RPC JSON localhost (stdlib net/http, :8555)** | Um binário estático p/ distribuir; RPC evita conflito de lock do bbolt (single-writer) |
| Endereço | **Base58Check**: versão `0x37` (começa com `P`) + `SHA-256(pubkey)[:20]` + checksum SHA-256d; base58 hand-rolled (~40 linhas) | Evita ripemd160 deprecado; sem dep nova |
| Genesis | Minerado offline uma vez (comando dev `node genesis`), **hardcoded em `params`** por perfil; hash do genesis vai no handshake `version` como ID da rede | É assim que toda chain concorda no bloco 0 |
| Config do node | Flags primeiro, env fallback (`NODE_*`) em `internal/node/config.go`, espelhando o padrão `getEnv` de `internal/config/config.go` (sem tocá-lo) | Consistência sem acoplar os dois binários |

**Sighash**: assinatura cobre SHA-256d da tx com o campo sig vazio e o slot da
pubkey substituído pelo PubKeyHash do output referenciado (SIGHASH_ALL
simplificado). Sem sistema de scripts — só pay-to-pubkey-hash estrutural.

## Ajuste de dificuldade conforme a rede cresce

Cada bloco só é válido se `Argon2id(header) < alvo`. A cada 100 blocos, todo nó
faz a mesma conta determinística: novo alvo = alvo atual × (tempo real dos
últimos 100 blocos ÷ 100 minutos esperados), com clamp de 4×/¼× por retarget.

- Rede cresceu → blocos saíram rápido demais → alvo cai → mais difícil → volta a ~60s/bloco.
- Rede encolheu → blocos lentos → alvo sobe → mais fácil → volta a ~60s/bloco.
- O ritmo de emissão (e o cronograma de halving) é imune ao tamanho da rede.
- Com Argon2id não há incentivo pra escalar hardware: o custo por participante
  fica fixo e baixo (1 core + 64 MiB por worker); a segurança vem da quantidade
  de participantes, não da potência de cada um.

## Layout de pacotes

Direção de dependência estrita: `params ← core ← pow ← chain ← {mempool, p2p, miner} ← node ← cmd/node`. Structs concretos, wiring manual, sem interfaces/DI (convenção do CLAUDE.md).

```
cmd/node/main.go            dispatch de subcomandos (flag.NewFlagSet), wiring manual
internal/params/            Params + perfis DevNet/MainNet, BlockSubsidy(height), MaxSupply(), genesis.go
internal/core/              block.go (Header 96B: Version, Height, PrevHash, MerkleRoot,
                            Timestamp, Bits, Nonce), tx.go (Tx/TxIn/TxOut/OutPoint, coinbase,
                            sighash), merkle.go, encoding.go (binário canônico), address.go
internal/pow/               argon2.go (PowHash), target.go (CompactToTarget, work), retarget.go
internal/chain/             store.go (buckets bbolt: blocks, blockIndex, heightIndex, utxo,
                            undo, meta), chain.go (AcceptBlock, Tip, locators),
                            validate.go (PoW, merkle, timestamps MTP-11/+2min, coinbase =
                            subsidy+fees, UTXO, sigs, maturity 10 blocos),
                            forkchoice.go (reorg via undo sets), orphans.go (pool ~100)
internal/mempool/           pool validado por txid, anti double-spend, ordenação por fee rate,
                            remove no connect / restaura no reorg
internal/wallet/            keygen P-256, wallet.json (0600), BuildTx (seleção largest-first,
                            change, assinatura por input)
internal/p2p/               message.go (version, verack, ping/pong, addr/getaddr, inv,
                            getdata, getheaders/headers, block, tx, reject), codec.go
                            (testável com net.Pipe), peer.go, server.go (máx 8 peers),
                            sync.go (IBD: getheaders com locator, corpos em janelas de 16;
                            Argon2 verificado no bloco completo, não no header)
internal/miner/             worker pool (default 1 worker), template do mempool,
                            restart em novo tip
internal/node/              config.go (NODE_DATADIR, NODE_LISTEN :9551, NODE_RPC
                            127.0.0.1:8555, NODE_PEERS, NODE_MINE, NODE_MINERS, NODE_PROFILE),
                            node.go (Start/Stop, graceful shutdown como cmd/api),
                            rpc.go (getinfo, getbalance, sendtoaddress)
```

## Specs por domínio

As pastas de domínio já estão criadas, cada uma com sua SPEC.md (adaptada de
BASE_SPEC.md — a seção "Endpoints" vira "Interface do pacote / CLI" — e com uma
seção didática de conceito). `internal/node/SPEC.md` serve de overview do node.
Non-goals explícitos em cada spec: sem scripts, sem SPV, sem criptografia de
wallet v1, sem NAT traversal, sem explorer.

| Domínio | Spec | Conceito que ela explica |
|---|---|---|
| params | [internal/params/SPEC.md](internal/params/SPEC.md) | Parâmetros de consenso, halving e escassez |
| core | [internal/core/SPEC.md](internal/core/SPEC.md) | O que é blockchain; transações no modelo UTXO; merkle; endereços |
| pow | [internal/pow/SPEC.md](internal/pow/SPEC.md) | Proof of work; por que Argon2id memory-hard; target e retarget |
| chain | [internal/chain/SPEC.md](internal/chain/SPEC.md) | Validação, UTXO set, forks/reorg, gênesis |
| mempool | [internal/mempool/SPEC.md](internal/mempool/SPEC.md) | Fila de transações pendentes e mercado de taxas |
| wallet | [internal/wallet/SPEC.md](internal/wallet/SPEC.md) | Chaves/endereços; construção e assinatura de transações |
| p2p | [internal/p2p/SPEC.md](internal/p2p/SPEC.md) | Gossip, handshake, sincronização (IBD), peer exchange |
| miner | [internal/miner/SPEC.md](internal/miner/SPEC.md) | O loop de mineração e o orçamento de recursos baixo |
| node | [internal/node/SPEC.md](internal/node/SPEC.md) | O full node: orquestração, config, RPC local, CLI |

## Milestones (cada um testável isoladamente)

**M1 — params + core + pow**: build `CGO_ENABLED=0` verde com código existente
intocado. Testes: halving soma = max supply; encode/decode round-trip; merkle;
nBits round-trip; PoW passa/falha vs target; clamp do retarget; sign/verify sighash.

**M2 — chain + storage + genesis**: genesis devnet minerado e hardcoded; aceita
sequência de blocos válidos; rejeita cada classe de bloco inválido (PoW ruim,
merkle, timestamp, coinbase inflado, double-spend, UTXO inexistente, coinbase
imaturo); teste de reorg: fork A(3) vs B(4) → muda pra B e UTXO set bate com
replay do zero.

**M3 — wallet + txs + mempool**: `node wallet new` gera wallet.json 0600 e
endereço `P...` que round-tripa; BuildTx seleciona/troca/assina; mempool rejeita
double-spend e sig inválida, ordena por fee, evict no connect e restore no reorg.

**M4 — P2P + sync**: handshake em `net.Pipe` (mismatch de versão/genesis, happy
path); frame gigante rejeitado; integração: 2 nodes in-process, A com 50 blocos,
B vazio → B sincroniza; forks convergem pra mais trabalho; tx gossip A→B.

**M5 — miner + CLI + RPC + docs**: `node run --mine` minera sozinho na
dificuldade devnet com 1 worker (~64MiB); template reinicia em novo tip e inclui
txs por fee; `balance`/`send`/`info` via RPC; SIGINT fecha bbolt limpo; demo
2 nodes abaixo funciona; atualizar CLAUDE.md (seção Node + corrigir linha do
módulo — é `pandabk_coin`) e README; `go vet`/`go test` verdes.

## Verificação

```sh
CGO_ENABLED=0 go build ./...   # estático, incluindo cmd/api existente
go vet ./... && go test ./...  # params de teste (1MiB) mantêm rápido
CGO_ENABLED=0 go build -o bin/node ./cmd/node
```

Demo 2 nodes na mesma máquina:

```sh
# terminal 1 — minerador
./bin/node run --profile devnet --datadir ~/.panda/n1 --listen :9551 --rpc 127.0.0.1:8551 --mine
# terminal 2 — seguidor
./bin/node run --profile devnet --datadir ~/.panda/n2 --listen :9552 --rpc 127.0.0.1:8552 --peers 127.0.0.1:9551
# terminal 3 — wallet
./bin/node wallet new --datadir ~/.panda/n2
./bin/node info    --rpc 127.0.0.1:8552   # alturas convergem => sync ok
./bin/node balance --rpc 127.0.0.1:8551   # cresce conforme coinbases maturam
./bin/node send    --rpc 127.0.0.1:8551 --to P... --amount 1.5
./bin/node balance --rpc 127.0.0.1:8552   # recebe após próximo bloco
```

Sucesso = node 2 acompanha altura/tip do node 1, e um send do node 1 aparece no
saldo do node 2 após confirmação.

## Arquivos críticos

- `go.mod` — adicionar `go.etcd.io/bbolt` (única dependência nova)
- `CLAUDE.md` / `BASE_SPEC.md` — convenções a seguir; CLAUDE.md atualizado no M5
- `internal/config/config.go` — padrão env-fallback a espelhar (não modificar)
- `cmd/api/main.go` — padrão de wiring/graceful-shutdown a espelhar (não modificar)
