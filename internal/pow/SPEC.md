# Spec: pow — proof of work Argon2id, target e retarget

> Domínio do node PANDA (ver [PLAN.md](../../PLAN.md)). Depende de `core` e
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

## Objetivo

Implementar a checagem de proof of work memory-hard, a aritmética de
dificuldade (nBits ↔ target ↔ trabalho) e o ajuste periódico de dificuldade.

## Escopo

Entra:
- `argon2.go` — `PowHash(headerBytes, params)` via `golang.org/x/crypto/argon2`
  (IDKey, salt fixo `"pandabk/pow/v1"`, keyLen 32)
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
