# Spec: core — bloco, transação (UTXO), merkle, endereço

> Domínio do node PANDA (ver [PLAN.md](../../PLAN.md)). Depende apenas de
> `internal/params`. Tipos puros + serialização canônica + hashing — sem I/O,
> sem estado.

## Conceito

**O que é uma blockchain, concretamente:** uma lista de blocos onde cada bloco
carrega o hash do bloco anterior (`PrevHash`). Alterar qualquer bloco antigo
muda seu hash, o que invalida o campo `PrevHash` do bloco seguinte, e assim por
diante até a ponta — reescrever história exige refazer todo o proof of work
dali em diante. É isso que torna a cadeia imutável na prática.

**O que é uma transação no modelo UTXO** (Unspent Transaction Output, o modelo
do Bitcoin): não existem "contas com saldo". Existem **moedas avulsas**
(outputs) trancadas para um dono. Uma transação consome outputs inteiros
existentes (inputs, provando posse com assinatura digital) e cria novos
outputs. Seu "saldo" é a soma dos outputs não gastos que seu endereço consegue
destravar — como notas na carteira: para pagar 30 com uma nota de 50, você
entrega a nota e recebe 20 de **troco** (um output de volta pra você). A
diferença `inputs − outputs` é a **taxa**, que o minerador embolsa.

A **coinbase** é a transação especial que abre cada bloco: não tem input real
e cria as moedas novas da recompensa (subsídio + taxas). É assim que todo
PANDA nasce.

A **merkle root** resume todas as transações do bloco num único hash de 32
bytes dentro do header: qualquer transação alterada muda a raiz, que muda o
hash do header, que quebra a cadeia.

## Objetivo

Definir os tipos fundamentais (bloco, transação, endereço) e sua serialização
binária canônica — a base determinística sobre a qual todo o consenso é
calculado.

## Escopo

Entra:
- `block.go` — `Header` (96 bytes canônicos) e `Block`; ID do bloco =
  SHA-256d(header)
- `tx.go` — `Tx`, `TxIn`, `TxOut`, `OutPoint`, TxID = SHA-256d(tx), construtor
  e verificação estrutural de coinbase, sighash (SIGHASH_ALL simplificado)
- `merkle.go` — merkle root sobre txids (duplicar o último em nível ímpar,
  como o Bitcoin)
- `encoding.go` — serialização binária canônica big-endian de Header/Tx/Block
- `address.go` — Base58Check: versão `0x37` (endereços começam com `P`) +
  `SHA-256(pubkey comprimida)[:20]` + checksum SHA-256d[:4]; base58 hand-rolled

Fica de fora:
- Validação contextual (UTXO existe? valor bate?) — vive em `chain`
- PoW — vive em `pow`
- Geração de chaves e construção de tx — vive em `wallet`

## Modelo de dados

| Tipo | Campos | Observação |
|------|--------|------------|
| Header | Version uint32, Height uint64, PrevHash [32]byte, MerkleRoot [32]byte, Timestamp int64, Bits uint32, Nonce uint64 | 96 bytes canônicos, big-endian |
| Block | Header, Txs []Tx | Txs[0] deve ser coinbase |
| OutPoint | TxID [32]byte, Index uint32 | referencia um output existente |
| TxIn | Prev OutPoint, Sig []byte (ASN.1 DER), PubKey []byte (33B comprimida) | coinbase: Prev = {zero, 0xffffffff}, PubKey carrega height‖extranonce (unicidade, estilo BIP34) |
| TxOut | Value uint64 (subunidades), PubKeyHash [20]byte | pay-to-pubkey-hash estrutural, sem scripts |
| Tx | Version uint32, Ins []TxIn, Outs []TxOut | |

## Regras de negócio

- Serialização **determinística**: mesma struct → mesmos bytes, sempre (por
  isso hand-rolled; gob/JSON não garantem ordem).
- **Sighash**: SHA-256d da tx serializada com, por input, o campo Sig vazio e
  o slot da PubKey substituído pelo PubKeyHash do output referenciado.
  Assinatura ECDSA P-256 via `crypto/ecdsa` (SignASN1/VerifyASN1).
- Coinbase estrutural: exatamente 1 input, OutPoint zero/0xffffffff; TxID deve
  ser única por altura (height embutida na PubKey).
- Endereço decodifica de volta ao PubKeyHash com verificação de checksum e
  versão.

## Interface do pacote

```go
func (h *Header) ID() [32]byte                    // SHA-256d
func (h *Header) Bytes() []byte                   // 96B canônicos
func (t *Tx) TxID() [32]byte
func (t *Tx) SigHash(i int, prevPKH [20]byte) [32]byte
func (t *Tx) IsCoinbase() bool
func NewCoinbase(height uint64, value uint64, pkh [20]byte, extra []byte) Tx
func MerkleRoot(txids [][32]byte) [32]byte
func PubKeyToAddress(pub []byte) string
func DecodeAddress(s string) ([20]byte, error)
func DecodeHeader([]byte) (Header, error)          // + Encode/Decode de Tx e Block
```

## Casos de erro / edge cases

- Decode de bytes truncados/corrompidos → erro, nunca panic
- Endereço com checksum inválido ou versão errada → erro
- Merkle de lista vazia (bloco sem coinbase é inválido — retornar erro/zero
  definido) e de 1 tx (root = txid)
- Tx com 0 inputs ou 0 outputs → inválida estruturalmente

## Critérios de aceite

- [ ] `block.go`, `tx.go`, `merkle.go`, `encoding.go`, `address.go` + testes
- [ ] Teste: encode/decode round-trip de Header, Tx e Block
- [ ] Teste: merkle com vetores conhecidos (1, 2, 3 txs)
- [ ] Teste: sign/verify do sighash (aceita assinatura certa, rejeita adulterada)
- [ ] Teste: endereço round-trip + rejeição de checksum inválido
- [ ] Header serializado tem exatamente 96 bytes

## Fora de escopo / não fazer

- Sem sistema de scripts (Bitcoin Script) — só pay-to-pubkey-hash estrutural
- Sem SegWit/multisig/timelocks
- Sem ripemd160 (deprecado) — hash de endereço é SHA-256 truncado
