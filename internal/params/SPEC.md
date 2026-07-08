# Spec: params — parâmetros de consenso e economia

> Domínio do node Zhu (ver [PLAN.md](../../PLAN.md)). Adaptado de
> BASE_SPEC.md: este pacote não tem HTTP; a seção "Endpoints" vira "Interface
> do pacote". Não depende de nenhum outro pacote do node.

## Conceito

Toda blockchain é um acordo: milhares de máquinas que nunca se conheceram
precisam chegar ao mesmo resultado fazendo as mesmas contas. Os **parâmetros de
consenso** são os números desse acordo — recompensa por bloco, cronograma de
halving, tempo-alvo entre blocos, regras de dificuldade. Se dois nós usarem
números diferentes, eles rejeitam os blocos um do outro e a rede se parte.
Por isso tudo fica em **um único pacote**, sem lógica, que todos os outros
importam — e nunca muda depois que a rede está no ar.

O **halving** é o mecanismo de escassez do Bitcoin que replicamos: a recompensa
por bloco cai pela metade a cada N blocos, então a emissão total converge para
um teto matemático (soma geométrica). No perfil devnet: 50 ZHU iniciais,
halving a cada 1.000 blocos → supply máximo ~100.000 ZHU.

## Decisões & porquês (regra e arquitetura)

Cada escolha aqui é *regra de consenso*: mudá-la depois de a rede existir parte
a rede. Por isso as razões ficam registradas — não são preferências, são
contratos.

- **Um único pacote, sem dependências, sem lógica de I/O.** `params` é a raiz da
  árvore de dependências (`params ← core ← pow ← chain ← ...`). Se um parâmetro
  de consenso vivesse em dois lugares, uma correção poderia atualizar um e não o
  outro, e os nós divergiriam silenciosamente. Centralizar num pacote-folha
  torna a divergência impossível por construção: existe uma só fonte da verdade.
- **Perfis são funções que retornam `Params` por valor, não globais mutáveis.**
  Um `var Mainnet = Params{...}` global poderia ser alterado em runtime (por
  engano ou por teste), mudando o consenso do processo inteiro. Retornar cópia
  garante que ninguém "ajusta a rede" sem passar por um novo build.
- **`MaxSupply()` é derivado, nunca hardcoded.** O teto de emissão é a soma do
  cronograma de halving calculada em laço. Um número fixo poderia contradizer
  `BlockSubsidy` se um dos dois fosse editado; derivar garante que os dois
  concordam por definição. O preço (um laço de dezenas de iterações) é pago uma
  vez, fora do caminho quente.
- **Subunidade inteira (1 ZHU = 1e8), nunca ponto flutuante.** Dinheiro em
  `float64` acumula erro de arredondamento — dois nós somando as mesmas taxas em
  ordem diferente chegariam a saldos diferentes. Todo valor é `uint64` de
  subunidades; a conversão para Zhu é só apresentação.
- **`Argon2Threads = 1`, sempre.** O paralelismo de mineração é do `miner`
  (N workers), não de dentro de um hash. Se o número de threads entrasse no
  cálculo do Argon2, o hash dependeria do hardware de quem minerou — e deixaria
  de ser verificável por quem tem outra máquina. Manter 1 torna o PoW
  determinístico entre máquinas.
- **`MaxBlockSize = 256 KiB` é consenso, não configuração.** Um limite maior
  aceleraria o crescimento do disco e expulsaria o node doméstico — o oposto do
  princípio "um node em cada casa" (PLAN.md). Como é regra de consenso, um node
  que aceitasse blocos maiores se separaria da rede; por isso vive aqui e não num
  flag de runtime.
- **Regras do `build.conf` derivam o gênesis em vez de conferi-lo
  (`CustomBuild`).** Quando o desenvolvedor fixa spacing/halving/subsidy no
  build, esses valores entram na mensagem do bloco 0 → mudam o hash do gênesis →
  mudam o ID da rede no handshake. O efeito é intencional: um binário com
  economia diferente forma uma rede *separada por construção* e nunca se mistura
  por acidente com a rede oficial. É a trava que permite distribuir builds
  experimentais sem poluir a rede principal.
- **Perfil `test` parte da base devnet, mas sem os overrides de build.** Testes
  precisam de regras estáveis e Argon2 barato (1 MiB) para rodar em
  milissegundos; se herdassem o `build.conf` do desenvolvedor, o mesmo teste
  passaria numa máquina e falharia noutra.
- **`mainnet` é placeholder declarado.** Preferimos um perfil que existe e está
  marcado como não-usar a valores "provisórios" que alguém trataria como
  definitivos. A honestidade sobre o que ainda não foi calibrado é parte da
  regra.

## Objetivo

Centralizar todos os parâmetros de consenso e a política monetária em perfis
imutáveis (devnet/mainnet/test), para que nenhuma regra numérica fique
espalhada pelo código.

## Escopo

Entra:
- `Params` struct com todos os parâmetros de consenso
- Perfis: `DevNet()` (bloco 60s, halving 1.000), `TestNet()` (Argon2 1MiB para
  testes rápidos), `MainNet()` (placeholder — bloco 10min, definido depois)
- `BlockSubsidy(height uint64) uint64` — recompensa da coinbase por altura
- `MaxSupply() uint64` — derivado somando o cronograma de halving (divisão
  inteira, não fórmula fechada)
- `genesis.go` — spec do bloco gênesis por perfil (preenchido no M2)

Fica de fora:
- Qualquer lógica de validação (vive em `pow`/`chain`)
- Carregamento de config de runtime (vive em `internal/node/config.go`)

## Modelo de dados

| Campo | Tipo | Observação |
|-------|------|------------|
| Name | string | "devnet", "mainnet", "test" |
| TargetSpacing | time.Duration | 60s no devnet |
| RetargetInterval | uint64 | 100 blocos |
| HalvingInterval | uint64 | 1.000 blocos |
| InitialSubsidy | uint64 | 5_000_000_000 subunidades (50 ZHU; 1 ZHU = 1e8) |
| Argon2Mem / Argon2Time / Argon2Threads | uint32/uint32/uint8 | 65536 KiB / 1 / 1 no devnet; 1024 KiB no perfil test |
| PowLimitBits | uint32 | dificuldade mínima (target máximo) em nBits |
| MaxClamp | int64 | 4 — limite de ajuste por retarget (4×/¼×) |
| CoinbaseMaturity | uint64 | 10 blocos até poder gastar a coinbase |
| MaxBlockSize | uint32 | 262144 (256 KiB) — **regra de consenso**: bloco serializado maior é inválido. Mantém o crescimento do disco compatível com um node caseiro (princípio "um node em cada casa" do PLAN.md) |
| Genesis | GenesisSpec | timestamp, mensagem, nBits, nonce, hash esperado |
| DefaultPort / DefaultRPCPort | string | :9551 / 127.0.0.1:8555 |
| SeedPeers | []string | peers iniciais do perfil |

## Regras de negócio

- `BlockSubsidy(h)` = `InitialSubsidy >> (h / HalvingInterval)`; retorna 0
  quando o shift zera (fim da emissão).
- `MaxSupply()` soma `BlockSubsidy` por época de halving até zerar — o valor é
  derivado, nunca hardcoded, para não divergir do cronograma.
- Perfis são funções que retornam valor (`Params`), não ponteiros globais
  mutáveis.

## Interface do pacote

```go
func DevNet() Params
func TestNet() Params
func MainNet() Params
func (p Params) BlockSubsidy(height uint64) uint64
func (p Params) MaxSupply() uint64
```

## Casos de erro / edge cases

- Altura além do último halving → subsídio 0 (sem panic, sem overflow).
- Shift de halving ≥ 64 → tratar explicitamente como 0.

## Critérios de aceite

- [ ] `params.go`, `genesis.go`, `params_test.go` criados
- [ ] Teste: soma do cronograma de halving == `MaxSupply()`
- [ ] Teste: subsídio nas fronteiras (bloco 999, 1000, 1001) e no fim da emissão
- [ ] Nenhuma dependência de outro pacote do node
- [ ] `CGO_ENABLED=0 go build ./...` verde

## Fora de escopo / não fazer

- Sem parâmetros configuráveis em runtime que afetem consenso (só perfil)
- Sem valores de mainnet definitivos nesta fase (placeholder documentado)
