# Spec: wallet — chaves, endereços e construção de transações

> Domínio do node PANDA (ver [PLAN.md](../../PLAN.md)). Depende de `core` e
> `params`.

## Conceito

Uma carteira de criptomoeda **não guarda moedas** — guarda **chaves**. A chave
privada é um número secreto; dela deriva a chave pública, e do hash da pública
deriva o **endereço** (o que você compartilha para receber). As moedas vivem
na blockchain, como outputs (UTXOs) trancados para o hash da sua chave
pública; a chave privada é o que **destranca**: assinar uma transação é a
prova matemática de posse, verificável por qualquer nó sem revelar o segredo.

**Construir uma transação** é: escolher UTXOs seus que somem o valor desejado
(seleção), criar um output para o destinatário, um output de **troco** de
volta pra você (UTXOs são gastos inteiros, como notas), deixar uma diferença
como **taxa** para o minerador, e assinar cada input com a chave privada
correspondente.

Perder a chave privada = perder os fundos, para sempre — não existe "esqueci a
senha" em blockchain. Por isso o arquivo da wallet tem permissão 0600.

## Objetivo

Gerar e persistir pares de chaves, derivar endereços, e construir/assinar
transações válidas a partir dos UTXOs do dono.

## Escopo

Entra:
- `wallet.go` — keygen ECDSA P-256 (`crypto/ecdsa` + `crypto/rand`),
  `wallet.json` com permissão 0600 (chave privada em hex/base64, endereço),
  load/save, derivação de endereço via `core.PubKeyToAddress`
- `sign.go` — `BuildTx(utxos, to, amount, feeRate)`: seleção largest-first,
  output de troco (se sobra > poeira), cálculo de taxa, sighash + assinatura
  por input

Fica de fora:
- Consulta de saldo/UTXOs à chain (o chamador — RPC do node — fornece os UTXOs)
- Broadcast (vive em node/p2p)

## Modelo de dados (wallet.json)

| Campo | Tipo | Observação |
|-------|------|------------|
| private_key | string | chave P-256 serializada (SEC1/PKCS8 base64) |
| public_key | string | 33 bytes comprimidos, hex |
| address | string | Base58Check começando com `P` |

Arquivo criado com 0600; erro se já existe (não sobrescrever chave!).

## Regras de negócio

- Keygen usa exclusivamente `crypto/rand` (nunca math/rand)
- Seleção largest-first: ordena UTXOs por valor decrescente, acumula até
  cobrir `amount + fee`; erro `ErrInsufficientFunds` se não cobrir
- Troco < poeira (ex.: 1.000 subunidades) → incorporado à taxa em vez de criar
  output minúsculo
- Taxa = feeRate × tamanho estimado da tx (re-estimar após adicionar troco)
- Cada input assinado com o sighash correto (`core.SigHash`)

## Interface do pacote

```go
func New(path string) (*Wallet, error)        // gera e salva; erro se existe
func Load(path string) (*Wallet, error)
func (w *Wallet) Address() string
func (w *Wallet) PubKeyHash() [20]byte
func (w *Wallet) BuildTx(utxos []chainlike.UTXO, to [20]byte, amount, feeRate uint64) (*core.Tx, error)
```

(`chainlike.UTXO` = struct simples {OutPoint, Value, PubKeyHash} definida em
`core` ou `wallet` — sem depender de `chain` para manter a direção de
dependências.)

## Casos de erro / edge cases

- `New` sobre arquivo existente → erro (proteção contra sobrescrever chave)
- Fundos insuficientes (incluindo taxa) → `ErrInsufficientFunds`
- Amount 0 ou endereço de destino inválido → erro
- wallet.json corrompido ou com permissão errada → erro claro no Load

## Critérios de aceite

- [x] `wallet.go`, `sign.go` + testes
- [x] Teste: New/Load round-trip; arquivo sai com 0600
- [x] Teste: BuildTx seleciona, cria troco, assina — e a tx passa na validação
      de assinatura de `core`
- [x] Teste: fundos insuficientes e troco-poeira
- [x] `node wallet new` (antecipado do M5) imprime endereço `P...` que
      round-tripa no decode; `node wallet show` reexibe

## Fora de escopo / não fazer

- Sem criptografia do arquivo da wallet nesta versão (non-goal registrado)
- Sem HD wallets (BIP32/39), sem multi-endereço — 1 wallet = 1 chave
- Sem histórico de transações da wallet
