# Aprendizado — acompanhando a construção da Zhu

> Diário didático do projeto: a cada milestone, uma seção explicando **o que
> foi construído, por que, e como testar com as próprias mãos**. Complementa
> as `SPEC.md` de cada pasta (que têm a seção "Conceito" de cada domínio) e o
> [PLAN.md](PLAN.md).

## M1 — A fundação da mineração (params, core, pow)

### O que foi construído

Três pacotes, na ordem em que um depende do outro:

**1. `internal/params` — as regras do jogo.** Um pacote só com números: a
recompensa inicial (50 ZHU), o halving (a cada 1.000 blocos), o tempo-alvo
(60s), a janela de retarget (100 blocos), os parâmetros do Argon2id (64 MiB
por hash), o tamanho máximo de bloco (256 KiB). Está tudo em "perfis"
(`DevNet()`, `TestNet()`, `MainNet()`) porque esses números são **o acordo da
rede** — dois nós com números diferentes rejeitam os blocos um do outro. A
função mais bonita daqui é a `MaxSupply()`: ela não tem o teto de emissão
escrito em lugar nenhum — ela **soma o cronograma de halving** e o teto
(~100.000 ZHU) emerge da matemática. Teste em
`params_test.go` prova que a soma bate.

**2. `internal/core` — as peças.** O `Header` do bloco (exatos 96 bytes:
versão, altura, hash do bloco anterior, merkle root, timestamp, dificuldade,
nonce), a transação UTXO (`Tx` com inputs que gastam outputs existentes e
outputs novos), a coinbase (a tx especial que cria moedas), a merkle root
(resumo de todas as txs em 32 bytes), o endereço (Base58Check começando com
`P`), e as chaves/assinaturas (ECDSA da biblioteca padrão). O detalhe
importante é a **serialização canônica**: cada struct vira sempre exatamente
os mesmos bytes, porque os hashes (e portanto toda a segurança) são
calculados sobre esses bytes. Por isso ela é escrita à mão em
`encoding.go` em vez de usar JSON.

**3. `internal/pow` — a loteria.** Três arquivos:
- `argon2.go` — o hash de mineração: Argon2id sobre os 96 bytes do header,
  forçando 64 MiB de RAM por tentativa. Note a separação: o **ID** do bloco é
  SHA-256d (barato, pra indexar); o hash de **PoW** é o Argon2id (caro, só na
  checagem de mineração).
- `target.go` — a aritmética da dificuldade: o campo `Bits` do header (4
  bytes) expande para o *target* (um número de 256 bits); o bloco é válido se
  `Argon2id(header) < target`. E `BlockWork` calcula quanto "trabalho" cada
  bloco representa — é a soma disso que decide qual fork vence.
- `retarget.go` — o termostato: a cada 100 blocos, compara o tempo real com o
  esperado e ajusta o target, com trava de 4× por ajuste.

**Bônus: `cmd/node powdemo`** — uma bancada de mineração real para você
testar (abaixo). É o embrião do CLI definitivo; os subcomandos `run`,
`wallet`, `send` chegam nos próximos milestones.

### Como testar (amanhã de manhã ☕)

```sh
# 1. Os testes automatizados (tudo tem que passar):
go test ./internal/params ./internal/core ./internal/pow -v

# 2. Compile o node:
CGO_ENABLED=0 go build -o bin/node ./cmd/node

# 3. MINERE! Perfil devnet real: 64 MiB por hash, ~2^8 tentativas por bloco:
./bin/node powdemo
```

Você vai ver o hashrate da sua máquina (na casa de ~20 H/s com 1 worker — é
pouco mesmo, e é esse o ponto: no Bitcoin seriam milhões por segundo) e cada
bloco encontrado com seu nonce e hash.

### Experimentos pra entender na prática

```sh
# A) Cada bit de dificuldade DOBRA o trabalho médio. Compare:
./bin/node powdemo -zeros 6 -blocks 3     # ~64 tentativas/bloco
./bin/node powdemo -zeros 9 -blocks 3     # ~512 tentativas/bloco (8× mais)

# B) O efeito memory-hard: mesmo hash, 1 MiB em vez de 64 MiB:
./bin/node powdemo -profile test -zeros 8
#    → o hashrate explode (centenas de H/s). É por isso que a MEMÓRIA é a
#      arma da descentralização: com 64 MiB, ninguém consegue esse salto
#      comprando chip melhor — teria que comprar RAM, que todo mundo tem.

# C) Mais workers = mais H/s, linearmente (+64 MiB de RAM cada):
./bin/node powdemo -workers 2
#    → abra o Monitor de Atividade e veja a RAM/CPU do processo enquanto roda.

# D) A variância da loteria: rode o mesmo comando 3 vezes e compare quantas
#    tentativas cada bloco levou. Às vezes 5, às vezes 800 — a MÉDIA é 2^zeros,
#    mas cada bloco é sorte. É por isso que a rede mede 100 blocos antes de
#    ajustar a dificuldade.
```

### O que observar / perguntas pra se fazer

- Por que o nonce muda tudo? (o nonce entra nos 96 bytes → o Argon2id muda
  completamente → nova chance na loteria)
- Por que o `ID do bloco` e o `hash PoW` impressos são diferentes? (dois
  hashes, dois papéis — veja `internal/pow/SPEC.md`)
- No experimento B, quanto foi seu H/s com 1 MiB vs 64 MiB? Essa razão é o
  "preço" que a memória impõe — e vale igual pra qualquer atacante.

### O que ainda NÃO existe (e vem a seguir)

O powdemo minera blocos *soltos* — não há chain guardando eles, nem validação
de transações, nem saldo. Isso é o **M2** (`internal/chain`, tarefa #6): o
banco bbolt, as regras de validação completas, os forks/reorgs e o bloco
gênesis de verdade. Depois vêm wallet+mempool (M3), a rede P2P (M4) e o node
completo (M5).

---

## M1.5 — A corrida de mineradores (powdemo -db)

O powdemo aprendeu a competir: com `-db`, vários processos na mesma máquina
mineram a **mesma chain**, usando um arquivo SQLite compartilhado como
"quadro-negro da rede". É uma prévia didática de dois conceitos que viram
código de verdade no M2 e no M4:

- **Fork choice**: quando dois mineradores acham o bloco N quase juntos, só
  um entra (a altura é chave primária no banco — o primeiro INSERT vence).
  O outro vê `🐼 você perdeu a corrida` e o trabalho dele é descartado. Na
  rede real isso é um *bloco órfão/stale*, e é resolvido pelo trabalho
  acumulado, não por um banco.
- **Propagação de blocos**: enquanto você minera, um vigia olha o banco a
  cada segundo; se outro minerador estendeu a chain, sua busca é cancelada
  (`📥 Bob minerou o bloco 12`) e você recomeça em cima do bloco dele — o
  equivalente demo de "um bloco novo chegou pela rede".

A dificuldade agora é **derivada do banco**, não da memória do processo:
qualquer minerador reaplica o retarget época por época sobre os timestamps
gravados e chega nos MESMOS bits. É o embrião da ideia mais importante de
consenso: *todo mundo calcula as mesmas regras sobre os mesmos dados, sem
ninguém mandar em ninguém*.

### Experimentos pra fazer em casa

```sh
CGO_ENABLED=0 go build -o bin/node ./cmd/node

# Terminal 1 — Alice abre a corrida (grava as regras no banco):
./bin/node powdemo -db mineracao.db -name Alice -blocks 0 -spacing 1m -zeros 10

# Terminal 2 — Bob entra na corrida (adota as regras que já estão no banco):
./bin/node powdemo -db mineracao.db -name Bob -blocks 0

# Terminal 3 — consultas a qualquer momento (não atrapalham a mineração):
./bin/node blocks  -db mineracao.db -last 10   # os últimos blocos: quem, quando, ⏱ quanto tempo
./bin/node ranking -db mineracao.db            # placar: blocos, %, Zhu, ritmo da rede

# A) O efeito "mais gente minerando": deixe só a Alice rodando por uma época
#    (10 blocos) e anote o ritmo. Suba o Bob e espere a próxima época:
#    os blocos saem ~2× mais rápido → o RETARGET SOBE a dificuldade →
#    o ritmo volta pro alvo. É o termostato reagindo a hashrate novo.

# B) A corrida perdida: com os dois rodando, espere um 🐼 aparecer.
#    Blocos com pouca dificuldade saem tão rápido que os dois acham quase
#    juntos — igual acontece com blocos de verdade na Bitcoin (~1 stale/dia).

# C) Desligue e religue um minerador (Ctrl+C e sobe de novo): a carteira
#    dele continua lá — agora o saldo vive no banco, não na memória.

# D) Injustiça proposital: dê -workers 2 pro Bob e deixe a Alice com 1.
#    O ranking mostra o Bob levando ~2/3 dos blocos — proporcional ao
#    hashrate, exatamente como PoW distribui recompensa.
```

### O que observar / perguntas pra se fazer

- Quando você perde uma corrida, quanto trabalho foi jogado fora? Por que
  isso NÃO é um problema pra rede (e sim o custo da descentralização)?
- Os dois terminais nunca "conversam" — só leem/escrevem o mesmo banco. O
  que acontece se os relógios discordassem? (spoiler: o M2 valida timestamps)
- O `ranking` divide os blocos ~proporcionalmente ao hashrate. O que isso
  diz sobre pools de mineração e por que elas existem na Bitcoin?

### O que ainda NÃO existe (e vem a seguir)

O SQLite é um dublê: não há validação dos blocos gravados (qualquer processo
poderia escrever um bloco falso no banco!), não há transações nem UTXO, e só
funciona na mesma máquina. O **M2** (`internal/chain`, tarefa #6) troca o
dublê pela chain de verdade: bbolt, validação completa, forks armazenados e
fork choice por trabalho acumulado — e aí um bloco falso é *rejeitado pelas
regras*, não pela confiança no arquivo.

---

*(próxima seção: M2, quando for construído)*
