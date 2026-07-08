# Spec: mempool — pool de transações pendentes

> Domínio do node PANDA (ver [PLAN.md](../../PLAN.md)). Depende de `core` e
> `chain`.

## Conceito

Entre o momento em que alguém envia uma transação e o momento em que um
minerador a inclui num bloco, ela vive no **mempool** (memory pool) — a sala
de espera da rede. Cada nó mantém o seu: valida a transação ao receber
(assinaturas, UTXOs existem, sem double-spend contra outras já esperando),
guarda em memória e repassa aos peers. O minerador monta o próximo bloco
escolhendo as transações do mempool — tipicamente as de **maior taxa por
byte**, porque a taxa é dele. É o mercado de espaço no bloco.

Quando um bloco chega, as transações confirmadas saem do mempool. Quando
acontece um **reorg** (a cadeia troca de ramo), as transações do ramo
abandonado que não estão no novo ramo voltam ao mempool — ninguém perde a
transação, ela só volta pra fila.

## Decisões & porquês (regra e arquitetura)

O mempool é a única parte "de política" do node (o que *aceitar* esperar), em
contraste com a chain, que é "de consenso" (o que é *válido*). As decisões aqui
equilibram simplicidade e proteção contra abuso.

- **Estrutura pura em memória + mutex; a persistência é do `node`.** O mempool
  em si é um `map` protegido por mutex, sem I/O — fácil de raciocinar e de testar
  com `-race`. Ele expõe `AllTxs()` e o `internal/node` (em `mempoolfile.go`) é
  quem tira o snapshot no shutdown e recarrega no boot. Essa divisão mantém o
  mempool sem dependência de disco (a regra de negócio não sabe onde é salva) e
  concentra a decisão de "onde/como persistir" numa camada só. *(Isto revisa a
  nota histórica abaixo de que "o mempool não sobrevive a restart": a estrutura
  não persiste sozinha, mas o node a persiste — a rede não depende disso, só
  ganha um boot mais quente.)*
- **Índice `spends` (outpoint→txid) além do `pool`.** Rejeitar double-spend entre
  transações *pendentes* exige responder "este output já foi reservado?" em O(1).
  Varrer todas as txs do pool a cada admissão seria O(n) por input; o índice
  inverso paga um pouco de memória por uma admissão barata mesmo com o pool
  cheio.
- **Sem encadeamento de transações não-confirmadas.** Uma tx que gasta o output
  de outra ainda no mempool é rejeitada nesta versão. Aceitá-la exigiria validar
  cadeias de dependência e removê-las em cascata quando a base cai — uma máquina
  de estados bem mais complexa. Cortamos esse caso: quem quer gastar espera a
  confirmação. Simplicidade escolhida deliberadamente, registrada como limite.
- **Sem Replace-by-Fee (RBF).** Uma vez no pool, uma tx não é substituível por
  outra de taxa maior sobre os mesmos inputs. RBF abre questões de política e de
  UX (a tx "sumiu e virou outra") que não valem a pena para o node doméstico.
- **Sem mínimo de taxa por política (qualquer taxa ≥ 0 entra).** O corte real é o
  *evict da menor fee rate quando o pool enche* — o mercado decide, não uma
  constante arbitrária. Um piso fixo envelheceria mal (o valor "certo" muda com o
  uso); deixar o limite de tamanho fazer a triagem é auto-regulável.
- **Ordenação por fee rate (taxa/byte), não por taxa absoluta.** O recurso escasso
  é *espaço no bloco* (bytes), não a taxa em si. Uma tx grande com taxa alta pode
  render menos por byte que várias pequenas; ordenar por taxa/byte maximiza o que
  o minerador embolsa dentro do orçamento de `MaxBlockSize`.
- **`Readd` no reorg passa pela validação completa de `Add`.** As txs
  desconectadas de um ramo abandonado não voltam ao pool "na fé": o novo ramo
  pode ter gasto os mesmos inputs. Revalidar na reinserção é o que impede o
  mempool de ressuscitar uma tx que o consenso já invalidou.

## Objetivo

Manter o conjunto de transações válidas ainda não confirmadas, protegido
contra double-spend, ordenado por taxa, sincronizado com os eventos da chain.

## Escopo

Entra:
- Admissão com validação completa contra o UTXO set atual + regras estruturais
- Índice de outputs reservados (outpoint → txid) para rejeitar double-spend
  entre transações pendentes
- Ordenação por fee rate (taxa / tamanho serializado) para o miner
- `RemoveConfirmed(block)` no connect; `Readd(txs)` no reorg (revalidando)
- Limite de tamanho do pool (ex.: 5.000 txs, evict das de menor fee rate)

Fica de fora:
- Propagação na rede (p2p chama `Add` e lê para gossip)
- Replace-by-fee (RBF)

## Modelo de dados

| Estrutura | Conteúdo |
|-----------|----------|
| pool | map txid → {Tx, feeRate, tamanho, chegada} |
| spends | map outpoint → txid (outputs reservados por txs pendentes) |

Tudo em memória, protegido por mutex — o mempool não sobrevive a restart
(comportamento padrão de nodes; a rede re-propaga).

## Regras de negócio

- `Add(tx)`: rejeita se coinbase, se txid já presente, se qualquer input não
  existe no UTXO set, se input já reservado por outra tx pendente, se
  assinatura inválida, se Σin < Σout, se coinbase imatura. Taxa = Σin − Σout.
- `TopByFeeRate(maxBytes)`: transações em ordem de fee rate decrescente até
  encher o orçamento de bytes do template do miner.
- No connect de bloco: remover txs confirmadas e qualquer tx pendente que
  ficou inválida (input gasto pelo bloco).
- No reorg: `Readd` das txs desconectadas passa pela mesma validação de `Add`
  (o novo ramo pode tê-las invalidado).

## Interface do pacote

```go
func New(c *chain.Chain, p params.Params) *Mempool
func (m *Mempool) Add(tx *core.Tx) error
func (m *Mempool) Has(txid [32]byte) bool
func (m *Mempool) Get(txid [32]byte) (*core.Tx, bool)
func (m *Mempool) TopByFeeRate(maxBytes int) []*core.Tx
func (m *Mempool) RemoveConfirmed(b *core.Block)
func (m *Mempool) Readd(txs []*core.Tx)
```

## Casos de erro / edge cases

- Duas txs pendentes gastando o mesmo outpoint → segunda rejeitada (erro
  sentinela `ErrDoubleSpend`)
- Tx que gasta output de outra tx *ainda no mempool* → rejeitada nesta versão
  (sem encadeamento de não-confirmadas; simplifica a validação)
- Pool cheio → evict da menor fee rate antes de aceitar uma maior

## Critérios de aceite

- [x] `mempool.go` + `mempool_test.go`
- [x] Teste: rejeita double-spend e assinatura inválida
- [x] Teste: ordenação por fee rate e corte por orçamento de bytes
- [x] Teste: evict no connect e restore (com revalidação) no reorg
- [x] Concorrência: `go test -race` verde

## Fora de escopo / não fazer

- Sem RBF, sem encadeamento de txs não confirmadas, sem persistência do pool
- Sem mínimo de taxa por política (qualquer taxa ≥ 0 entra; mercado decide)
