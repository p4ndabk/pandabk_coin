# Spec: miner — o loop de mineração

> Domínio do node Zhu (ver [PLAN.md](../../PLAN.md)). Depende de `core`,
> `pow`, `chain`, `mempool`, `params`.

## Conceito

**Minerar** é o trabalho que mantém a rede viva, e tem dupla função: (1)
decidir quem escreve o próximo bloco — sem eleição nem autoridade, ganha quem
achar primeiro um nonce cujo Argon2id do header fique abaixo do alvo; e (2)
**emitir moeda**: o vencedor inclui a coinbase pagando a si mesmo o subsídio
da altura + as taxas das transações incluídas. É a "loteria proporcional":
sua chance de ganhar cada rodada é a fração do hashrate total que você
contribui.

O ciclo do minerador: montar um **template** (coinbase + transações do mempool
ordenadas por taxa + merkle root + header apontando para a ponta atual) →
iterar nonces calculando Argon2id → se achou, submeter à chain e anunciar via
p2p → se **outro** nó achou primeiro (a ponta mudou), descartar o trabalho e
recomeçar do novo tip. Trabalho sobre um tip velho é desperdício — o bloco
nasceria órfão.

**O requisito "recursos baixos" do projeto vive aqui:** cada worker é uma
goroutine que usa 1 core + ~64 MiB (a matriz do Argon2id). O default é **1
worker** — minerar não deve dominar a máquina de ninguém; quem quiser doar
mais cores aumenta via config. Não há vantagem em hardware exótico (ver
`internal/pow/SPEC.md`), então a rede cresce em número de participantes, não
em consumo por participante.

## Decisões & porquês (regra e arquitetura)

O miner é onde a aposta de produto ("um node em cada casa, minerando um pouco")
encontra o código. As decisões são sobre *não desperdiçar trabalho* e *não
dominar a máquina*.

- **Default de 1 worker, opt-in para mais.** Minerar é o comportamento padrão do
  node (é o que descentraliza), mas não pode tornar a máquina do usuário
  inutilizável — senão ninguém deixa ligado, e a rede morre. Um worker (1 core,
  ~64 MiB) é o custo mínimo de participar; quem quer doar mais aumenta
  conscientemente. O default protege a adoção, que é a segurança da rede.
- **Paralelismo por workers, não por threads do Argon2.** N goroutines varrendo
  faixas de nonce *disjuntas* (via extranonce por worker) escalam a busca sem
  mudar o custo de um hash. Se aumentássemos as threads internas do Argon2, o
  hash dependeria do hardware e deixaria de ser verificável entre máquinas (ver
  `params`: `Argon2Threads = 1`). Escalar "para fora" mantém o PoW determinístico.
- **Template sempre sobre `chain.Tip()`; novo tip cancela via `context`
  imediatamente.** Trabalho sobre um tip velho é lixo — o bloco nasceria órfão.
  Assinar o evento de "tip mudou" (local ou da rede) e cancelar os workers na
  hora, remontando o template, garante que todo ciclo de CPU vai para o bloco que
  ainda pode vencer. É a diferença entre minerar e desperdiçar energia.
- **Coinbase limitada a `BlockSubsidy(height) + Σ taxas`, montada localmente.** O
  miner poderia "tentar" pagar a si mesmo mais que o permitido, mas a chain
  rejeitaria o bloco — trabalho perdido. Respeitar o teto na montagem evita
  minerar algo que seria recusado. O endereço de pagamento é a wallet do datadir,
  o que amarra "minerar por padrão" a "ter um destino para a moeda" (por isso o
  node cria a wallet no primeiro `run`).
- **Minerar bloco só-coinbase quando o mempool está vazio é válido e desejado.**
  No começo da rede (ou em períodos sem transações) não há txs para incluir; um
  bloco só com a coinbase ainda avança a cadeia, emite moeda e mantém o retarget
  funcionando. Tratar isso como caso normal (não erro) é o que deixa a rede
  arrancar do zero.
- **Estado efêmero, sem persistência.** O template e os contadores por worker
  vivem só na memória do processo — se o node reinicia, remonta do tip atual. Não
  há nada a salvar: o valor está na cadeia (persistida pela `chain`), não no
  trabalho em andamento.
- **Sem stratum, sem pools, sem GPU, sem auto-tuning.** Pools e stratum
  centralizariam a mineração — exatamente o que o Argon2id memory-hard existe para
  evitar. GPU não traz vantagem relevante (o gargalo é memória). Auto-tuning de
  workers pela carga da máquina é conveniência de evolução futura. Cada ausência
  é coerente com o ponto do projeto, não uma lacuna.

## Objetivo

Produzir blocos válidos a partir do mempool, com orçamento de CPU/memória
configurável e reinício imediato quando a ponta muda.

## Escopo

Entra:
- Montagem de template: coinbase (endereço da wallet do nó, height +
  extranonce), txs de `mempool.TopByFeeRate` até o orçamento de bytes
  (`params.MaxBlockSize` menos o overhead de header/coinbase), merkle root,
  header com bits corretos para a altura (retarget na fronteira)
- Pool de N workers (default 1), cada um varrendo uma faixa de nonce
- Canal de notificação de novo tip (assinado da chain/node) → cancela workers
  e remonta o template
- Timestamp do header atualizado periodicamente durante a busca
- Bloco achado → `chain.AcceptBlock` + callback de broadcast (p2p)

Fica de fora:
- Stratum/pool de mineração externo (non-goal)
- Estatísticas além de hashrate aproximado no log

## Modelo de dados

N/A — estado efêmero (template atual, contadores por worker).

## Regras de negócio

- Template sempre construído sobre `chain.Tip()`; bits vindos de `pow.NextBits`
  quando a altura cruza a fronteira de retarget
- Extranonce na coinbase diferencia workers (nonce de 64 bits por worker em
  faixas disjuntas)
- Novo tip (local ou da rede) invalida o template imediatamente — workers
  cancelados via context
- Coinbase respeita `params.BlockSubsidy(height)` + Σ taxas — nunca mais que
  isso (a chain rejeitaria)

## Interface do pacote

```go
func New(c *chain.Chain, m *mempool.Mempool, p params.Params, payTo [20]byte, workers int) *Miner
func (mn *Miner) Start(ctx context.Context, onBlock func(*core.Block))
func (mn *Miner) Stop()
func (mn *Miner) HashRate() float64   // hashes/s aproximado, p/ getinfo
```

## Casos de erro / edge cases

- Mempool vazio → mina bloco só com coinbase (válido e necessário no começo
  da rede)
- Tip muda no exato momento em que um worker acha um bloco → AcceptBlock trata
  (vira bloco lateral ou é rejeitado; não crashar)
- `workers` ≤ 0 → erro de config; workers > NumCPU → aviso no log
- Stop durante busca → workers drenados via context, sem goroutine vazada

## Critérios de aceite

- [x] `miner.go` + `miner_test.go`
- [x] Teste (perfil test, Argon2 1MiB, dificuldade mínima): minera um bloco
      válido que a chain aceita
- [x] Teste: template inclui txs do mempool por fee rate e a coinbase soma
      subsídio + taxas (e o bloco do template é aceito pela chain)
- [x] Teste: sinal de novo tip remonta o template (TipChanged + poll de tip)
- [x] Default de 1 worker documentado; `go test -race` verde

## Fora de escopo / não fazer

- Sem stratum/pools, sem GPU (o ponto do projeto é exatamente não precisar)
- Sem auto-tuning de workers pela carga da máquina (evolução futura)
