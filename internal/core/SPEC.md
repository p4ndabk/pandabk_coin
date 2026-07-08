# Spec: core — bloco, transação (UTXO), merkle, endereço

> Domínio do node Zhu (ver [PLAN.md](../../PLAN.md)). Depende apenas de
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
Zhu nasce.

A **merkle root** resume todas as transações do bloco num único hash de 32
bytes dentro do header: qualquer transação alterada muda a raiz, que muda o
hash do header, que quebra a cadeia.

## Decisões & porquês (regra e arquitetura)

Este pacote define o **formato de bytes canônico** do consenso: dois nós só
concordam sobre um bloco se serializarem os mesmos campos exatamente nos mesmos
bytes. Toda decisão aqui protege esse determinismo.

- **Serialização hand-rolled big-endian, não gob/JSON/protobuf.** `encoding/gob`
  e `encoding/json` não garantem ordem estável de campos nem representação única
  de um número — o mesmo bloco poderia gerar bytes diferentes, logo hashes
  diferentes, logo IDs de bloco diferentes. Escrevemos o encoder à mão para que
  `struct → bytes` seja uma função total e determinística. Big-endian ("network
  byte order") porque é a convenção de protocolos de rede e não depende do
  endianness da CPU.
- **ID do bloco = SHA-256d(header) ≠ hash de PoW.** O header tem 96 bytes fixos;
  hashear só ele (e não o bloco inteiro) torna o encadeamento barato. Usar
  SHA-256d (duplo) para o ID e reservar o Argon2 caro só para a checagem de PoW
  (em `pow`) evita recomputar o hash memory-hard toda vez que a chain precisa
  referenciar um bloco. São dois hashes com dois papéis, deliberadamente
  separados (padrão do Monero).
- **Header de 96 bytes de tamanho fixo.** Campos de largura fixa (sem varint no
  header) tornam o parsing e o hashing triviais e à prova de ambiguidade de
  comprimento. O custo (alguns bytes a mais que o mínimo teórico) é irrelevante
  frente à simplicidade de um formato sem surpresas.
- **Modelo UTXO, não contas com saldo.** UTXO permite validar transações sem um
  estado global mutável de "saldo por conta": cada input aponta para um output
  específico e ou ele existe e não foi gasto, ou a tx é inválida. É também o
  modelo do Bitcoin, o que mantém o projeto didático alinhado ao material de
  referência que o usuário vai encontrar por aí.
- **Pay-to-pubkey-hash *estrutural*, sem Bitcoin Script.** Não há máquina de
  scripts: o output guarda um `PubKeyHash` e o gasto exige assinatura da chave
  correspondente, ponto. Um interpretador de script seria uma superfície de bugs
  e de consenso enorme para um recurso que o projeto não precisa. Escolhemos o
  caso de uso mais comum do Bitcoin e o embutimos direto na regra.
- **Endereço = SHA-256 truncado a 20 bytes, sem RIPEMD-160.** O Bitcoin usa
  RIPEMD-160 por razões históricas; é um algoritmo pouco usado e depreciado em
  bibliotecas modernas. SHA-256 truncado dá 20 bytes de resistência prática para
  o nosso caso, com uma primitiva que já está na stdlib e é auditada.
- **Base58Check hand-rolled com versão `0x37`.** Base58 evita caracteres
  ambíguos (0/O, l/1); o byte de versão `0x37` faz todo endereço começar com `P`
  (identidade visual da moeda) e o checksum de 4 bytes (SHA-256d) pega erro de
  digitação **antes** de qualquer moeda ser enviada para um endereço inexistente
  — um pagamento perdido é irreversível, então a validação barata no cliente vale
  muito.
- **Coinbase carrega a altura na PubKey (estilo BIP34).** Duas coinbases da mesma
  recompensa para o mesmo endereço teriam o mesmo TxID e colidiriam no índice de
  UTXO. Embutir a altura (mais um extranonce) garante TxID único por bloco sem
  precisar de um campo extra.
- **SIGHASH_ALL simplificado, um só modo de assinatura.** O Bitcoin tem vários
  modos de sighash; nós assinamos sempre a transação inteira (todos os inputs e
  outputs). Menos modos = menos regras de consenso = menos como errar. ECDSA
  P-256 via `crypto/ecdsa` da stdlib pelo mesmo motivo do SHA-256: primitiva
  auditada, não hand-rolled.

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
