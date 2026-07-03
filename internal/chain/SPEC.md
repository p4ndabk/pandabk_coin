# Spec: chain — a blockchain como máquina de estados

> Domínio do node PANDA (ver [PLAN.md](../../PLAN.md)). Depende de `core`,
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
4. Merkle root bate com as txs; Txs[0] é coinbase e é a única
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

- [ ] `store.go`, `chain.go`, `validate.go`, `forkchoice.go`, `orphans.go` + testes
- [ ] Gênesis devnet minerado e hardcoded em `params/genesis.go`
- [ ] Teste: aceita sequência de blocos válidos (gerados programaticamente)
- [ ] Teste: rejeita cada classe de bloco inválido (PoW ruim, merkle errada,
      timestamp fora, coinbase inflada, double-spend, UTXO inexistente,
      coinbase imatura) — um teste por regra
- [ ] Teste de reorg: fork A(3 blocos) vs B(4 blocos) → muda para B e o UTXO
      set final é idêntico a um replay do zero
- [ ] Buckets documentados neste SPEC (substitui o critério de migration do
      BASE_SPEC — o node não usa GORM)

## Fora de escopo / não fazer

- Sem pruning (guardamos todos os blocos)
- Sem checkpoints hardcoded nesta versão
- Sem índice de transação por txid global (só o necessário para validação e
  balance)
