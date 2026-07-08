# Spec: chain — a blockchain como máquina de estados

> Domínio do node Zhu (ver [PLAN.md](../../PLAN.md)). Depende de `core`,
> `pow`, `params` e `go.etcd.io/bbolt`. É o coração do consenso: decide o que
> entra na cadeia e mantém o estado dela em disco.

## Conceito

Se `core` define as peças, `chain` é o tabuleiro. Este pacote responde três
perguntas:

1. **Esse bloco é válido?** — PoW confere? A merkle root bate? As transações
   gastam moedas que existem e não foram gastas? A coinbase não imprime mais
   que o permitido?
2. **Qual é a cadeia verdadeira?** — Quando dois mineradores acham blocos
   quase juntos, a rede bifurca (**fork**). Cada nó guarda os dois ramos e
   segue o que tem **mais trabalho acumulado** (não o mais longo — soma do
   `BlockWork` de cada bloco). Quando o outro ramo ultrapassa, o nó faz
   **reorg**: desconecta os blocos do ramo perdedor (desfazendo seus efeitos
   com os *undo sets*) e conecta o vencedor. É assim que milhares de nós
   convergem sem nenhum coordenador.
3. **Quanto cada endereço pode gastar?** — O **UTXO set**: o conjunto de todos
   os outputs ainda não gastos. É o "estado" da blockchain — a cadeia de
   blocos é o histórico; o UTXO set é a foto atual, atualizada a cada bloco
   conectado (remove os gastos, adiciona os criados).

O **bloco gênesis** é o bloco 0, idêntico e hardcoded em todos os nós — o
ponto de partida do acordo. Seu hash funciona como "ID da rede" no handshake
P2P: quem tem outro gênesis está em outra rede.

## Decisões & porquês (regra e arquitetura)

`chain` é o único pacote com autoridade para dizer "isto entra na história". As
decisões aqui são sobre *integridade sob falha* e *convergência sem
coordenador*.

- **bbolt como storage, não SQLite/GORM.** O node inteiro é um binário estático
  `CGO_ENABLED=0`; SQLite (mesmo o pure-Go) e GORM trariam peso e uma camada de
  ORM que não cabe num consenso que fala em bytes e buckets. bbolt é um
  key-value transacional em arquivo único, sem CGO, com exatamente a semântica
  que precisamos: chave→valor por bucket e transações ACID. (É por isso que o
  node vive fora do skeleton Gin/GORM do resto do repo.)
- **Connect de bloco é UMA transação bbolt (tudo ou nada).** Conectar um bloco
  toca vários buckets (blocks, blockIndex, heightIndex, utxo, undo, meta). Se um
  crash acontecesse no meio, o UTXO set poderia ficar inconsistente com a cadeia
  — corrupção silenciosa que só apareceria blocos depois. Envolver tudo numa
  transação única garante que ou o bloco entra inteiro, ou não entra: o banco
  nunca fica num estado intermediário.
- **Undo sets persistidos por bloco.** Um reorg precisa *desfazer* blocos:
  restaurar os UTXOs que eles gastaram e remover os que criaram. Recalcular isso
  reexecutando a cadeia ao contrário seria caro e propenso a erro; guardar
  explicitamente "o que este bloco consumiu" (o undo set) torna o reorg uma
  operação local e determinística. É memória em disco trocada por reorg correto e
  rápido.
- **Fork choice por trabalho acumulado, reorg completo re-validando cada bloco.**
  A ponta ativa é a de maior `cumWork`; quando um ramo lateral ultrapassa, o node
  desconecta até o ponto de bifurcação e reconecta o vencedor **validando cada
  bloco por inteiro de novo**. Não confiamos que um bloco lateral já validado
  continue válido no novo contexto de UTXO — revalidar é a diferença entre um
  reorg seguro e aceitar um double-spend que só é visível no ramo novo.
- **MTP-11 (mediana dos 11 timestamps) e janela de +2 min no futuro.** Timestamp
  de bloco é auto-declarado pelo minerador; sem regra, ele poderia mentir para
  manipular o retarget. Exigir timestamp acima da mediana dos últimos 11 blocos
  (não do último — que é manipulável isolado) e abaixo de "agora + 2 min" limita
  a fraude a uma janela estreita. É a mesma defesa do Bitcoin.
- **Double-spend checado inclusive *dentro do mesmo bloco*.** Não basta que cada
  input aponte para um UTXO existente no banco; dois inputs do mesmo bloco não
  podem gastar o mesmo output. Validar só contra o UTXO set persistido deixaria
  passar um double-spend intra-bloco — por isso a validação rastreia os gastos
  ao longo do próprio bloco.
- **Gênesis hardcoded e isento das regras de validação.** O bloco 0 não tem pai
  nem histórico contra o qual se validar; ele *é* o ponto de partida do acordo.
  Conferi-lo por hash exato contra `params.Genesis` (em vez de rodar as regras
  normais) e embutir esse hash no handshake P2P transforma "estar na mesma rede"
  numa checagem de um hash — builds com regras diferentes têm gênesis diferente e
  se recusam mutuamente por construção.
- **Orphan pool limitado com evict FIFO (~100).** Um bloco cujo pai ainda não
  chegou é guardado à espera. Sem limite, um peer malicioso poderia inundar a
  memória com órfãos que nunca resolvem. O cap com descarte do mais antigo troca
  completude por uma superfície de DoS fechada — o node caseiro não pode ficar
  refém de memória.
- **Sem pruning, sem checkpoints, sem índice global por txid.** Guardamos todos
  os blocos (auditabilidade total e didática — dá para explorar qualquer bloco);
  não cravamos checkpoints (a segurança vem só do PoW acumulado, sem "confie
  neste hash"); e indexamos apenas o necessário para validação e saldo. Cada
  "não fazer" é uma complexidade que a fase atual não justifica e que teria seu
  próprio custo de manutenção e de consenso.

## Objetivo

Validar blocos contra todas as regras de consenso, persistir a cadeia e o UTXO
set em bbolt, e implementar fork choice por trabalho acumulado com reorg
seguro.

## Escopo

Entra:
- `store.go` — buckets bbolt: `blocks` (id→bloco), `blockIndex`
  (id→{height, cumWork, status}), `heightIndex` (height→id da cadeia ativa),
  `utxo` (outpoint→{TxOut, coinbase?, height}), `undo` (id→outputs gastos,
  para reorg), `meta` (tip, address book do p2p)
- `chain.go` — `Chain` struct, `AcceptBlock`, `Tip`, `LocatorHashes` (para o
  sync do p2p)
- `validate.go` — regras de validação (abaixo)
- `forkchoice.go` — comparação de trabalho acumulado, reorg via undo sets
- `orphans.go` — pool limitado (~100) de blocos cujo pai ainda não chegou
- Mineração offline do gênesis devnet (comando dev) e hardcode em
  `params/genesis.go`

Fica de fora:
- Rede (p2p pede/entrega blocos; chain só aceita/rejeita)
- Mempool (pacote próprio; chain notifica connect/disconnect)

## Modelo de dados (buckets bbolt)

| Bucket | Chave | Valor |
|--------|-------|-------|
| blocks | block ID (32B) | bloco serializado canônico |
| blockIndex | block ID | height, cumWork (big.Int bytes), status (ativo/lateral/inválido) |
| heightIndex | height (8B BE) | block ID da cadeia ativa |
| utxo | outpoint (36B) | TxOut + flag coinbase + height de criação |
| undo | block ID | lista de UTXOs consumidos pelo bloco (para restaurar no reorg) |
| meta | "tip", ... | block ID da ponta ativa |

## Regras de negócio (validação de bloco)

1. Header: PoW válido (`pow.CheckProofOfWork`), `Bits` corretos para a altura
   (retarget na fronteira de 100 blocos), timestamp > mediana dos últimos 11
   blocos (MTP-11) e < agora + 2 min
2. `PrevHash` conhecido (senão → orphan pool + pedir o pai)
3. Bloco serializado ≤ `params.MaxBlockSize` (256 KiB) — regra de consenso
   que protege o disco/banda do node caseiro
4. Merkle root bate com as txs; Txs[0] é coinbase e é a única; a coinbase
   embute a altura do bloco no input (estilo BIP34 — garante txid único por
   bloco, senão duas coinbases idênticas colidiriam no UTXO set)
5. Cada input referencia UTXO existente e não gasto **dentro do mesmo bloco
   também** (double-spend intra-bloco)
6. Assinatura ECDSA válida por input; `SHA-256(PubKey)[:20]` == PubKeyHash do
   output gasto
7. Coinbase gasta só após `CoinbaseMaturity` (10 blocos)
8. Σ inputs ≥ Σ outputs por tx (sem overflow — usar checagem explícita);
   coinbase.value ≤ subsídio(altura) + Σ taxas do bloco
9. Gênesis: validado por hash exato contra `params.Genesis` (isento das
   regras acima)

Fork choice: bloco válido em ramo lateral é armazenado com seu cumWork; se o
cumWork do ramo ultrapassar o da ponta ativa → reorg (desconectar até o ponto
de bifurcação restaurando UTXOs via undo, conectar o novo ramo validando cada
bloco por completo, devolver txs desconectadas ao mempool via callback).

## Interface do pacote

```go
func Open(path string, p params.Params) (*Chain, error)   // cria/abre DB, insere gênesis
func (c *Chain) AcceptBlock(b *core.Block) error            // valida + conecta/armazena/reorg
func (c *Chain) Tip() (core.Header, uint64, *big.Int)       // header, altura, cumWork
func (c *Chain) LocatorHashes() [][32]byte                  // espaçamento exponencial
func (c *Chain) GetBlock(id [32]byte) (*core.Block, error)
func (c *Chain) UTXOsByPKH(pkh [20]byte) ([]UTXO, error)    // p/ balance/wallet
func (c *Chain) Close() error
```

## Casos de erro / edge cases

- Cada regra de validação tem um erro sentinela próprio (testável)
- Bloco órfão: guardado no pool (cap ~100, evict FIFO), reprocessado quando o
  pai chega
- Reorg profundo: desconectar N blocos deve restaurar o UTXO set exatamente
  (teste por replay do zero)
- Crash no meio do connect: bbolt é transacional — connect de bloco é uma
  transação única (tudo ou nada)
- Bloco duplicado (já conhecido) → no-op sem erro

## Critérios de aceite

- [x] `store.go`, `chain.go`, `validate.go`, `forkchoice.go`, `orphans.go` + testes
- [x] Gênesis devnet minerado e hardcoded em `params/genesis.go` (comando dev
      `node genesis`)
- [x] Teste: aceita sequência de blocos válidos (gerados programaticamente)
- [x] Teste: rejeita cada classe de bloco inválido (PoW ruim, merkle errada,
      timestamp fora, coinbase inflada, double-spend, UTXO inexistente,
      coinbase imatura) — um teste por regra
- [x] Teste de reorg: fork A(3 blocos) vs B(4 blocos) → muda para B e o UTXO
      set final é idêntico a um replay do zero
- [x] Buckets documentados neste SPEC (substitui o critério de migration do
      BASE_SPEC — o node não usa GORM)

## Fora de escopo / não fazer

- Sem pruning (guardamos todos os blocos)
- Sem checkpoints hardcoded nesta versão
- Sem índice de transação por txid global (só o necessário para validação e
  balance)
