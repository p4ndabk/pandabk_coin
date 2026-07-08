# Spec: wallet — chaves, endereços e construção de transações

> Domínio do node Zhu (ver [PLAN.md](../../PLAN.md)). Depende de `core` e
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

## Decisões & porquês (regra e arquitetura)

A wallet lida com o segredo que controla dinheiro irreversível. As decisões aqui
priorizam *não perder a chave* e *interoperar com padrões abertos* acima de
qualquer conveniência.

- **Depende só de `core`/`params`, nunca de `chain`.** A wallet não consulta a
  blockchain: quem constrói a tx recebe os UTXOs de fora (o RPC do node os
  fornece). Isso mantém a direção de dependências (`wallet` é folha, como o
  miner) e permite construir/assinar transações sem carregar o banco — a wallet é
  testável e usável isolada. O tipo `chainlike.UTXO` existe justamente para não
  puxar `chain` só por causa de uma struct.
- **`crypto/rand` exclusivamente, nunca `math/rand`.** A entropia da chave é a
  única coisa entre o fundo e um atacante. `math/rand` é previsível (seed
  determinística) — uma chave gerada com ele é adivinhável. Esta é uma regra
  absoluta, não uma preferência.
- **ECDSA P-256 da stdlib, não uma curva/lib exótica.** P-256 (`crypto/ecdsa`) é
  auditada, ubíqua e suficiente. Trocá-la por secp256k1 (a do Bitcoin) traria uma
  dependência externa para hand-rolar cripto sensível sem ganho real para o
  projeto — a compatibilidade com carteiras Bitcoin nunca foi meta.
- **Criação com `O_EXCL` — nunca sobrescrever uma wallet existente.** `New`/
  `Restore` falham se o arquivo já existe. Sobrescrever silenciosamente uma
  `wallet.json` apagaria a chave e, com ela, os fundos, de forma irreversível.
  Falhar alto é o comportamento seguro; o custo (o usuário apaga de propósito se
  quiser) é aceitável frente à perda total.
- **Permissão 0600 no arquivo.** A chave privada em claro no disco só pode ser
  lida pelo dono. É a proteção mínima contra outro usuário da mesma máquina — de
  novo, barata e não-negociável para um segredo desse peso.
- **Seleção de UTXO largest-first.** Ordenar por valor decrescente e acumular até
  cobrir `amount + fee` minimiza o *número* de inputs na tx — menos inputs = tx
  menor = taxa menor = menos assinaturas a verificar. Não é ótimo para a
  privacidade nem para a fragmentação do UTXO set, mas é o previsível e barato,
  adequado ao escopo.
- **Troco abaixo da poeira vira taxa, não um output minúsculo.** Criar um output
  de troco de 500 subunidades geraria um UTXO que custa mais em taxa para gastar
  do que vale — lixo permanente no UTXO set de todos os nós. Incorporá-lo à taxa
  é mais limpo para a rede inteira.
- **BIP39 (12 palavras) + SLIP-0010, padrões abertos.** O backup usa a lista
  inglesa de 2048 palavras do BIP39 e a derivação SLIP-0010 para nist256p1 — não
  um esquema caseiro. Assim as mesmas 12 palavras recuperam a carteira em
  *qualquer* implementação que siga as specs (ex.: Python `mnemonic` +
  SLIP-0010), e os testes cravam os vetores oficiais (Trezor BIP39; SLIP-0010
  vector 1). Um formato próprio prenderia o usuário a este binário — o oposto do
  que um backup deve garantir. Wallets antigas (chave aleatória, pré-frase)
  seguem válidas, só sem mnemônico: nunca quebramos uma wallet existente.
- **Sem criptografia do arquivo, sem HD multi-endereço (não-metas declaradas).**
  Cifrar a `wallet.json` com senha adicionaria um fluxo de "esqueci a senha" que
  é justamente o que blockchain não perdoa; HD wallets (múltiplos endereços de
  uma seed) são conveniência que a fase atual não precisa. São limites
  conscientes, não omissões.

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
- [x] **Backup de 12 palavras (2026-07-05)**: `mnemonic.go` implementa BIP39
      (lista inglesa de 2048 palavras embutida; 128 bits + checksum) e a
      derivação SLIP-0010 da chave-mestra Nist256p1 — padrões abertos, as
      mesmas palavras recuperam a carteira em qualquer linguagem/lib que
      siga as specs (Python: `mnemonic` + qualquer SLIP-0010). Testes cravam
      os vetores oficiais (Trezor BIP39; SLIP-0010 vector 1 nist256p1).
      `NewWithMnemonic` devolve a frase UMA vez; `Restore` reconstrói de
      frase (O_EXCL, nunca sobrescreve); CLI `wallet new`/`wallet restore`;
      wallets antigas (chave aleatória) seguem válidas — só não têm frase.

## Fora de escopo / não fazer

- Sem criptografia do arquivo da wallet nesta versão (non-goal registrado)
- Sem HD wallets (BIP32/39), sem multi-endereço — 1 wallet = 1 chave
- Sem histórico de transações da wallet
