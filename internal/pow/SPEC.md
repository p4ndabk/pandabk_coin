# Spec: pow — proof of work Argon2id, target e retarget

> Domínio do node Zhu (ver [PLAN.md](../../PLAN.md)). Depende de `core` e
> `params`. Funções puras — sem estado, sem I/O.

## Conceito

**Proof of work** é uma loteria verificável: o minerador varia o `Nonce` do
header e recalcula o hash até obter um número menor que o **alvo (target)**.
Quanto menor o alvo, mais raro é acertar — essa é a "dificuldade". Achar o
nonce custa trilhões de tentativas; conferir custa um hash. É esse custo
assimétrico que protege a cadeia: reescrever um bloco antigo exige refazer o
trabalho dele e de todos os seguintes, mais rápido que a rede honesta avança.

**Por que Argon2id e não SHA-256:** SHA-256 é puro cálculo — um ASIC dedicado
é ~1.000.000× mais eficiente que uma CPU, então a mineração centraliza em quem
tem capital. O Argon2id é **memory-hard**: calcular um hash exige preencher e
acessar aleatoriamente 64 MiB de RAM, com acessos que dependem dos dados já
escritos (sem atalho paralelo). O gargalo vira banda de memória — commodity —
e a vantagem de hardware especializado cai para ~2–5×, que não paga o
investimento. Resultado: um notebook compete de igual, e a segurança da rede
vem da quantidade de participantes, não da potência de cada um.

**Dois hashes, dois papéis:** o **ID do bloco** é SHA-256d(header) — barato,
usado para indexar e encadear. O **hash de PoW** é Argon2id(header) — caro,
usado só na checagem `PowHash < target`. Separar os dois (padrão do Monero)
evita recomputar Argon2 toda vez que a chain precisa referenciar um bloco.

**Retarget:** a cada 100 blocos, todo nó recalcula o alvo:
`novo = atual × (tempo real dos 100 blocos ÷ 100 min esperados)`, com clamp de
4×/¼×. Rede cresceu → blocos rápidos demais → alvo cai → mais difícil → volta
a ~60s/bloco. O ritmo de emissão fica imune ao tamanho da rede.

## Decisões & porquês (regra e arquitetura)

O PoW é a fonte da segurança da rede e da nossa aposta de produto ("um node em
cada casa"). Cada escolha aqui é sobre *quem consegue minerar* e *quão barato é
verificar*.

- **Argon2id memory-hard em vez de SHA-256.** É a decisão central do projeto.
  SHA-256 é puro cálculo: um ASIC é ~1.000.000× mais eficiente que uma CPU e a
  mineração centraliza em quem tem capital, contradizendo o princípio doméstico.
  Argon2id força preencher e acessar aleatoriamente 64 MiB de RAM por hash, com
  dependência entre acessos (sem atalho paralelo); o gargalo vira banda de
  memória (commodity) e a vantagem de hardware dedicado cai para ~2–5×, que não
  paga o investimento. Um notebook compete de igual.
- **Variante `id` (não `i` nem `d`).** Argon2i resiste a ataques de canal
  lateral mas é mais fraco contra GPU; Argon2d é o oposto. Argon2id é o híbrido
  recomendado pela RFC 9106 para uso geral — é o default seguro quando não há
  razão para escolher um extremo.
- **Salt fixo (`zhubk/pow/v1`), não por-bloco.** No Argon2 como *password
  hashing* o salt é aleatório por senha. Aqui ele serve de **separador de
  domínio**: fixá-lo faz o PoW ser uma função pura do header (todo nó recalcula
  e confere o mesmo valor) e amarra o hash a *esta* rede — trocar o salt geraria
  hashes incompatíveis, ou seja, outra rede. O `v1` deixa espaço para uma
  migração versionada no futuro.
- **Dois hashes separados: SHA-256d para ID, Argon2id só para PoW.** Detalhado em
  `core`, mas a consequência mora aqui: a chain referencia blocos pelo ID barato
  e só paga o Argon2 caro na validação de PoW de um bloco. Se o ID fosse o hash
  de PoW, cada lookup de bloco custaria 64 MiB de RAM.
- **Fork choice por trabalho acumulado (`BlockWork`), não por altura.** A cadeia
  válida é a de maior trabalho somado, não a mais comprida. Altura é falsificável
  barato (encadear muitos blocos fáceis); trabalho não — reflete o custo real de
  energia/memória gasto. É o mesmo critério do Bitcoin e o que impede um atacante
  de "vencer" só produzindo blocos rápidos de baixa dificuldade.
- **nBits no formato compacto do Bitcoin.** Um target é um número de 256 bits;
  guardá-lo cru no header gastaria 32 bytes. O formato compacto (1 byte de
  expoente + 3 de mantissa) cabe em 4 bytes com precisão suficiente para
  dificuldade, e é o formato que qualquer material de referência de Bitcoin
  descreve.
- **Retarget estilo Bitcoin com clamp [¼×, 4×], não LWMA.** O ajuste proporcional
  ao tempo real da janela é simples de entender e auditar. O clamp impede que um
  timestamp manipulado ou uma variação brusca de hashrate faça a dificuldade
  saltar de forma catastrófica num único retarget. LWMA (média móvel ponderada,
  mais suave) fica registrado como evolução futura — não vale a complexidade
  extra nesta fase.
- **Guardas contra divisão por zero/negativo no retarget.** Janela curta +
  timestamps em segundos podem dar `actual = 0` na divisão inteira, o que zeraria
  o target (dificuldade impossível). Por isso `minActual` tem piso em 1 e
  timestamps invertidos são clampados em vez de propagados. Estabilidade
  numérica é regra de consenso: um nó que dividisse por zero e outro que
  clampasse divergiriam.
- **PoW não é verificado em headers isolados durante o IBD.** Argon2 é caro
  demais para rodar por header numa sincronização inicial; a checagem completa
  acontece ao validar o bloco inteiro. Decisão de desempenho registrada no
  PLAN.md — troca uma verificação redundante por uma sincronização viável no
  node doméstico.

## Objetivo

Implementar a checagem de proof of work memory-hard, a aritmética de
dificuldade (nBits ↔ target ↔ trabalho) e o ajuste periódico de dificuldade.

## Escopo

Entra:
- `argon2.go` — `PowHash(headerBytes, params)` via `golang.org/x/crypto/argon2`
  (IDKey, salt fixo `"zhubk/pow/v1"`, keyLen 32)
- `target.go` — `CompactToTarget(nBits) *big.Int`, `TargetToCompact`,
  `BlockWork(target)` = `2^256/(target+1)`, `CheckProofOfWork(header, params)`
- `retarget.go` — `NextBits(firstTimestamp, lastTimestamp, currentBits, params)`
  com clamp 4×/¼× e piso em `PowLimitBits`

Fica de fora:
- Loop de mineração (vive em `miner`)
- Decidir *quando* retargetar / validar contexto (vive em `chain`)

## Modelo de dados

N/A — funções puras sobre tipos de `core`/`params` e `*big.Int`.

## Regras de negócio

- `PowHash` interpreta os 32 bytes do Argon2id como inteiro big-endian;
  válido se `< target`.
- nBits usa o formato compacto do Bitcoin (1 byte expoente + 3 bytes
  mantissa); `TargetToCompact(CompactToTarget(x)) == x` para valores
  normalizados.
- Retarget nunca produz target acima de `PowLimitBits` (dificuldade mínima da
  rede) e o ajuste por época é limitado a [¼×, 4×].
- Trabalho acumulado da chain = soma de `BlockWork` — é o critério de fork
  choice (não a altura!).

## Interface do pacote

```go
func PowHash(header []byte, p params.Params) [32]byte
func CheckProofOfWork(h *core.Header, p params.Params) error
func CompactToTarget(bits uint32) *big.Int
func TargetToCompact(t *big.Int) uint32
func BlockWork(bits uint32) *big.Int
func NextBits(first, last int64, bits uint32, p params.Params) uint32
```

## Casos de erro / edge cases

- nBits malformado (mantissa/expoente fora de faixa) → target inválido, erro
- Timestamps invertidos (last < first) na janela de retarget → clampar no
  mínimo, nunca dividir por zero/negativo
- Target zero → tratar como inválido (divisão em BlockWork)

## Critérios de aceite

- [ ] `argon2.go`, `target.go`, `retarget.go` + testes
- [ ] Teste: nBits round-trip com vetores conhecidos
- [ ] Teste: PoW aceita hash abaixo do target e rejeita acima (perfil test 1MiB)
- [ ] Teste: retarget clampa em 4× e ¼× e respeita PowLimitBits
- [ ] Teste: BlockWork cresce quando o target cai
- [ ] Testes rodam em ms com o perfil test (Argon2 1MiB)

## Fora de escopo / não fazer

- Sem LWMA nesta versão (anotado como evolução futura em params)
- Sem verificação de PoW em headers durante IBD (Argon2 é caro; a checagem
  completa acontece no bloco inteiro — decisão registrada no PLAN.md)
