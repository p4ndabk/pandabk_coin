# Mineração na Zhu — o que o minerador procura, tecnicamente

Este documento responde uma pergunta que todo mundo faz ao comparar a Zhu com
o Bitcoin: *"por que o hash dos nossos blocos não começa com `0000...`?"* — e,
a partir dela, explica em detalhe o que a mineração de fato procura, como o
alvo (target) funciona e por que a Zhu usa **dois hashes** onde o Bitcoin usa
um só.

Os trechos de código citados são o código real: `internal/core/block.go`,
`internal/pow/{argon2,target,retarget}.go` e `internal/params/params.go`.

---

## 1. Os dois hashes de um bloco

No **Bitcoin**, o ID do bloco e o hash da prova-de-trabalho são o *mesmo*
valor: `SHA-256d(header)`. O minerador varia o nonce até esse hash ficar
abaixo do target — e um número de 256 bits "abaixo do target" é um número que
começa com zeros. Por isso os IDs de bloco do Bitcoin exibem a famosa parede
de `00000000000000000001a4b...`.

Na **Zhu**, esses dois papéis foram deliberadamente separados:

```
header (96 bytes) ──SHA-256d──► ID do bloco    ← "nome": indexa, encadeia, gossip. Barato.
header (96 bytes) ──Argon2id──► hash de PoW    ← "loteria": DEVE ficar ≤ target. Caro.
```

- **ID do bloco** = `SHA-256d(header)` (`core.Header.ID()`,
  `internal/core/block.go:30`). É o hash que aparece no explorador, no
  `prev_hash` do bloco seguinte, nas chaves do banco bbolt e nos anúncios
  `inv` do p2p. **Nenhuma regra exige zeros nele** — ele é só identidade.
- **Hash de PoW** = `Argon2id(header)` (`pow.PowHash`,
  `internal/pow/argon2.go:22`). É este que o minerador tenta fazer cair
  abaixo do target, e é este que "tem zeros na frente". Ele **não é
  armazenado nem exibido**: qualquer validador o recalcula a partir do
  header quando checa o PoW.

### Por que separar?

O Argon2id é *deliberadamente caro*: cada avaliação preenche
`Argon2Mem = 64 MiB` de RAM (`params.go:129`). Essa é a arma anti-ASIC do
projeto — memória custa caro em silício, então um chip dedicado não ganha a
vantagem absurda que ganhou no SHA-256 do Bitcoin. O node doméstico continua
competitivo ("um node em cada casa").

Mas se o *ID* do bloco fosse o hash Argon2id, toda operação corriqueira
pagaria esses 64 MiB: indexar um bloco, conferir um `inv` do gossip, montar
uma chave de UTXO, seguir um `prev_hash`. Separando os papéis, a chain usa o
SHA-256d (nanosegundos) para tudo que é identidade, e paga o Argon2id **uma
única vez por bloco**, na validação do PoW (`CheckProofOfWork`,
`internal/pow/target.go:86` — o comentário no código chama isso de "a
checagem cara").

O salt do Argon2id é fixo e serve de separador de domínio da rede
(`powSalt = "pandabk/pow/v1"`, `argon2.go:16`): outra rede com outro salt
produz hashes incompatíveis, mesmo com headers idênticos.

---

## 2. O header — a entrada da loteria

O header tem exatamente 96 bytes (`core.HeaderSize`,
`internal/core/block.go:10`):

| Campo      | Tipo     | Papel na mineração                                   |
|------------|----------|------------------------------------------------------|
| Version    | uint32   | versão das regras                                     |
| Height     | uint64   | altura do bloco                                       |
| PrevHash   | [32]byte | ID (SHA-256d) do bloco anterior — o elo da corrente   |
| MerkleRoot | [32]byte | raiz de Merkle das txs — "assina" o conteúdo do bloco |
| Timestamp  | int64    | quando o bloco foi montado                            |
| Bits       | uint32   | o target em formato compacto (nBits)                  |
| **Nonce**  | uint64   | **o único campo que o minerador varia na busca**      |

Dois detalhes importantes:

- O `MerkleRoot` prende as transações ao header: trocar qualquer tx muda a
  raiz, que muda o header, que muda os dois hashes. Não dá para "reaproveitar"
  um PoW com outro conteúdo.
- O `Bits` também está no header, ou seja, **o próprio bloco declara contra
  qual target foi minerado** — e o validador confere se esse target é o que
  as regras de retarget mandavam para aquela altura.

---

## 3. O que o minerador procura, exatamente

**Um nonce `n` tal que, interpretando os 32 bytes de
`Argon2id(header com Nonce=n)` como um inteiro de 256 bits (big-endian), o
valor seja ≤ target.**

O loop real (é literalmente assim no miner e nos testes,
`internal/miner/miner_test.go:66-73`):

```go
target := pow.CompactToTarget(b.Header.Bits)
for n := uint64(0); ; n++ {
    b.Header.Nonce = n
    hash := pow.PowHash(b.Header.Bytes(), p)          // Argon2id, ~64 MiB por chamada
    if new(big.Int).SetBytes(hash[:]).Cmp(target) <= 0 {
        break                                          // achou: bloco válido
    }
}
```

E a checagem do lado do validador (`CheckProofOfWork`, `target.go:86`):

```go
target := CompactToTarget(h.Bits)          // 1. expande o nBits do header
// target inválido ou acima do limite da rede → rejeita
hash := PowHash(h.Bytes(), p)              // 2. UM Argon2id
if new(big.Int).SetBytes(hash[:]).Cmp(target) > 0 {
    return ErrHashAboveTarget              // 3. acima do alvo → bloco inválido
}
```

A assimetria é o coração do proof of work: o minerador tentou milhares de
nonces; o validador confere **um**. Caro de achar, barato de conferir.

### Por que cada tentativa é "loteria"

O Argon2id (como todo hash criptográfico) é imprevisível: mudar 1 bit do
nonce produz um hash sem nenhuma relação com o anterior. Não existe atalho —
a única estratégia é tentar. A probabilidade de um hash aleatório de 256 bits
cair abaixo do target é:

```
P(acerto) = (target + 1) / 2²⁵⁶
```

Logo, o número esperado de tentativas por bloco é `2²⁵⁶ / (target + 1)` — e
essa mesma expressão é o **trabalho** que o bloco comprova (`BlockWork`,
`target.go:73`). É a soma desses trabalhos que o fork choice usa para decidir
qual ramo da chain é a verdade (ver `internal/chain/forkchoice.go`).

---

## 4. Target, nBits e dificuldade — a aritmética

### nBits: o target compactado

Um target é um inteiro de 256 bits — grande demais para viajar no header.
O campo `Bits` usa a codificação compacta do Bitcoin (1 byte de expoente +
3 bytes de mantissa):

```
target = mantissa × 256^(expoente − 3)
```

`CompactToTarget` (`target.go:26`) expande; `TargetToCompact` (`target.go:40`)
normaliza de volta. Exemplo real — o limite da rede
(`PowLimitBits = 0x20010000`, `params.go:132`):

```
0x20010000  →  expoente 0x20 = 32, mantissa 0x010000
target      =  0x010000 × 256^(32−3)  =  2¹⁶ × 2²³²  =  2²⁴⁸
```

### Dificuldade: quantas vezes mais difícil que o mínimo

```
dificuldade = target_limite / target_atual        (Difficulty, target.go:62)
```

- **Dificuldade 1** (o mínimo da rede): target = 2²⁴⁸. Tentativas esperadas:
  `2²⁵⁶ / 2²⁴⁸ = 256 hashes` por bloco (é o comentário do `params.go:132`).
- **Dificuldade 64** (a atual da devnet após o retarget do bloco 400):
  target = 2²⁴⁸ / 64 = 2²⁴². Tentativas esperadas: `256 × 64 = 16 384
  hashes`. A ~dezenas de ms por Argon2id, dá a ordem de minutos por bloco em
  um worker — coerente com o alvo de 1 bloco / 5 min.

### Onde estão os zeros, afinal

"Hash ≤ 2²⁴²" significa que os `256 − 242 = 14` bits mais significativos do
**hash Argon2id** têm que ser zero — em hex, os 3 primeiros dígitos e meio:

```
hash de PoW válido (dif. 64):  0002f8a1c34...   ← Argon2id: zeros exigidos
ID do mesmo bloco:             8d31b01dd08...   ← SHA-256d: zeros NÃO exigidos
```

O bloco 415 que você vê no explorador com ID `8d31b01d...` tem, sim, um hash
"com zeros" — só que é o Argon2id, que ninguém armazena. E a parede de 19+
zeros hex do Bitcoin reflete uma dificuldade na casa dos ~10¹⁴ — os zeros
crescem com o log da dificuldade, e a nossa devnet está em 64.

---

## 5. Retarget: quem escolhe o target

A cada `RetargetInterval = 100` blocos, `NextBits` (`retarget.go:16`)
recalcula o target estilo Bitcoin: escala o target atual pela razão entre o
tempo que a época *levou* e o tempo que *deveria levar*
(`100 × TargetSpacing`):

```
novo_target = target_atual × (tempo_real / tempo_esperado)
```

com dois guarda-corpos:

- o ajuste por época é limitado a `[1/MaxClamp, MaxClamp]×` (impede um salto
  absurdo por timestamps manipulados — mesmo papel do clamp de 4× do
  Bitcoin);
- o target nunca sobe além de `PowLimitBits` (a dificuldade nunca cai abaixo
  de 1).

Foi isso que apareceu no log da devnet: os 100 blocos da época saíram ~4×
mais rápido que 5 min cada → `🎯 retarget no bloco 400: 16.00 → 64.00`
(target dividido por 4, teto do clamp).

---

## 6. Resumo em uma frase

> O minerador da Zhu procura um **nonce** que faça o **Argon2id dos 96 bytes
> do header** cair **abaixo do target declarado em nBits**; o ID do bloco
> (SHA-256d) é só o nome pelo qual esse header é indexado e encadeado — por
> isso ele não tem, nem precisa ter, zeros na frente.

Para ver tudo isso ao vivo, com corrida de mineradores e recompensa simulada:

```
bin/zhu powdemo
```
