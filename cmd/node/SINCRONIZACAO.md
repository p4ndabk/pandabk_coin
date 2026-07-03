# Como um minerador sabe "em qual bloco está" (powdemo -db / -listen / -peer)

Pergunta: quando um segundo minerador entra na corrida, ele precisa se
sincronizar com o primeiro? Sim — mas de um jeito bem mais simples do que a
sincronização de uma rede P2P de verdade (que vem no M4). Este arquivo cobre
os DOIS modos que existem hoje:

- **Mesma máquina (`-db`)**: os processos só leem e escrevem o mesmo arquivo
  SQLite — não tem rede nenhuma envolvida. Seção "Modo 1" abaixo.
- **Máquinas diferentes (`-listen` / `-peer`)**: cada Mac tem seu PRÓPRIO
  banco, e a sincronização vira, de fato, uma troca de mensagens em TCP —
  o embrião didático do protocolo P2P real do M4. Seção "Modo 2" abaixo.

## Modo 1 — mesma máquina (`-db`)

### Conceito

Numa rede P2P real (Bitcoin, e futuramente PANDA/M4), um node novo pergunta
ativamente para os peers "qual é o seu tip?" e baixa os blocos que faltam
(handshake `version`/`verack`, depois `getheaders`). Isso existe porque cada
node tem sua **própria cópia** da chain guardada em disco (bbolt, no nosso
caso) — sem perguntar, ele não sabe o que perdeu.

No powdemo `-db`, não existe cópia local nenhuma: **o SQLite compartilhado É
a única fonte de verdade**, e os dois processos apontam pro mesmo arquivo.
Então "sincronizar" vira só "ler antes de agir" — não tem nada pra baixar ou
reconciliar, porque nunca existiu uma cópia divergente pra começo de
conversa. É uma simplificação deliberada da demo: o preço dela é que só
funciona na mesma máquina (mesmo arquivo).

### O que acontece quando um minerador entra

`cmd/node/powdemo.go`, função `runPowDemoShared` — dois momentos de leitura:

1. **Adoção das regras (`store.initMeta`, demostore.go:97)** — o primeiro
   minerador a abrir o banco grava profile/spacing/retarget/zeros na tabela
   `meta` (um único `INSERT ... ON CONFLICT DO NOTHING`, atômico: mesmo se
   dois processos subirem juntos, só um grava). Todo mundo que entra depois
   lê essas quatro chaves e **adota** — se suas flags de linha de comando
   divergirem do que já está gravado, aparece um aviso:

   ```
   ℹ️  adotando as regras já gravadas em mineracao.db: perfil test,
      alvo 9m0s/bloco, retarget a cada 10, zeros iniciais 4
   ```

   Isso garante que os dois calculam a mesma dificuldade — se cada um usasse
   seu próprio `-spacing`, o retarget divergiria e não haveria mais uma
   chain só, e sim duas contas diferentes do mesmo histórico.

2. **Leitura do tip (`store.tip`, demostore.go:141)** — antes de minerar
   qualquer bloco, o minerador roda `SELECT height, id FROM blocks ORDER BY
   height DESC LIMIT 1`. O resultado aparece direto no banner de abertura:

   ```
      banco             mineracao.db (altura atual: 44)
   ```

   Isso já é a "sincronização inicial": o Bob, ao entrar, descobre que a
   Alice já minerou 44 blocos e começa a minerar o 45 — nunca o 1. Não tem
   handshake, não tem download: é uma query.

### E durante a mineração? (a parte contínua)

Sincronizar só na entrada não bastaria — se a Alice já estivesse minerando o
bloco 45 havia 10 segundos quando o Bob liga, e o Bob minerasse o 45 mais
rápido, a Alice ficaria minerando um bloco que já não existe mais sem saber.
Por isso tem um **vigia** rodando durante toda a busca
(`runPowDemoShared`, o `go func()` com o `ticker` de 1 segundo):

```go
t := time.NewTicker(time.Second)
for {
    select {
    case <-t.C:
        if nt, err := store.tip(); err == nil && nt.height >= height {
            cancelMine() // alguém já publicou este bloco: para de minerar
            return
        }
    }
}
```

A cada segundo, ele repete a mesma pergunta ("qual é o tip agora?"). Se a
altura no banco já alcançou ou passou a que você está minerando, seu
`mine()` é cancelado na hora — sem esperar você terminar — e você vê:

```
📥 [09:15:24] Bob minerou o bloco 46 (+50 PANDA para ele, ⏱ 2.2s) — 3.0s de
   trabalho descartado, recomeçando no 47
```

Esse polling de 1s é o *modelo mental* mais próximo de um node real recebendo
um bloco novo pela rede (mensagem `inv`/`block` no P2P) — só que aqui, em vez
de alguém te avisar, você fica perguntando ao banco.

### E se os dois acharem quase ao mesmo tempo? (a corrida de verdade)

O polling de 1s tem uma folga: dois blocos fáceis podem sair com menos de 1s
de diferença, e nenhum dos dois vigias vai detectar a tempo. É aí que a
`insertBlock` (demostore.go) é a rede de segurança final: `height` é
`PRIMARY KEY` na tabela `blocks`, então o **banco** — não o relógio de
ninguém — decide quem chegou primeiro. O segundo `INSERT` na mesma altura
falha com violação de chave única, e vira:

```
🐼 [09:15:21] Bob registrou o bloco 45 primeiro — você perdeu a corrida
   (1.2s de trabalho descartado)
```

Repare a diferença entre os dois emojis nos logs:
- **📥** = você descobriu o bloco de alguém *enquanto ainda procurava* (o
  vigia te salvou antes de terminar)
- **🐼** = você **terminou** de minerar, tentou publicar, e alguém te venceu
  no `INSERT` por uma fração de segundo — típico quando a dificuldade está
  baixa e os blocos saem em rajadas

Isso é a versão didática de um **bloco órfão/stale**: na Bitcoin de verdade
acontece por volta de uma vez por dia, resolvido pelo trabalho acumulado (a
chain com mais Argon2id gasto vence), não por uma constraint de banco. É
exatamente esse fork choice de verdade que entra no M2 (`internal/chain`).

### Resumindo o Modo 1 em uma frase

Não existe protocolo de sincronização — existe um arquivo compartilhado, uma
leitura na entrada (`tip()` + `initMeta()`), um polling de 1s durante a busca
(o vigia), e uma constraint de banco (`height PRIMARY KEY`) como árbitro
final da corrida. A "rede" inteira é essas quatro peças.

### Ver com os próprios olhos

```sh
# Termine de ler este arquivo, depois olhe o histórico com o tempo de cada bloco:
./bin/node blocks -db mineracao.db -last 15

# Repare no campo "quando" (hora:min:seg): blocos do MESMO minerador em
# sequência rápida = ele ganhou a corrida sozinho por um tempo; blocos
# ALTERNANDO de minerador em segundos = os dois estavam disputando cada
# bloco (dificuldade baixa pro tamanho do hashrate combinado).
```

## Modo 2 — máquinas diferentes (`-listen` / `-peer`)

### Por que não dá pra só "compartilhar o banco"

O Modo 1 funciona porque um arquivo local pode ser lido/escrito por vários
processos com lock do sistema operacional. Isso não existe entre dois Macs:
não tem um arquivo em comum, só uma rede. Então cada Mac PRECISA da sua
própria cópia — e a "sincronização" deixa de ser "ler antes de agir" e vira
de verdade uma troca de mensagens, do jeito que qualquer rede P2P faz.

Por isso o powdemo exige `-db` **junto** com `-peer`: o node que sincroniza
nunca fica sem uma cópia própria. É uma correção deliberada em cima de uma
versão mais simples (e mais frágil) que veio antes — nela o minerador
remoto não guardava nada localmente e dependia 100% de uma conexão ao vivo
com o primeiro Mac. O problema: se aquele Mac desligasse, o segundo minerador
ficava cego (não sabia nem em que altura estava) e não tinha nada pra
oferecer a um terceiro node. Com `-db` obrigatório, isso não acontece mais —
ver "O que muda quando o peer cai" mais abaixo.

### As três peças do Modo 2

`cmd/node/netstore.go` implementa o transporte; `cmd/node/powdemo.go`
(`runPowDemoShared`) orquestra as três peças:

1. **`serveRace` (o lado `-listen`)** — abre uma porta TCP e responde
   requisições sobre o banco `-db` LOCAL desse processo: "qual seu tip?",
   "me dá o bloco da altura N", "aqui vai um bloco que eu minerei". Cada
   operação do banco (`tip`, `blockAt`, `insertBlock`, ...) vira uma mensagem
   — o mesmo formato de fio já decidido pro protocolo definitivo do M4:
   TCP puro, frame com prefixo de 4 bytes (tamanho) + corpo JSON.

2. **`syncFromPeer` (a sincronização INICIAL)** — antes de minerar qualquer
   coisa, quem tem `-peer` pergunta o tip remoto e baixa, um por um, todo
   bloco que ainda não tem localmente:

   ```
   📡 sincronizando bob.db com 192.168.1.10:9551: 0 → 44 blocos...
   ✅ sincronizado — 44 blocos copiados. bob.db agora tem uma cópia completa.
   ```

   Isso é a versão em miniatura do IBD (*Initial Block Download*) que um
   node Bitcoin faz na primeira vez que liga.

3. **`replicateFromPeer` (a sincronização CONTÍNUA)** — depois da carga
   inicial, uma goroutine em segundo plano pergunta o tip do peer a cada 1
   segundo e replica qualquer bloco novo pro banco local. A parte elegante:
   ela não precisa avisar mais ninguém que um bloco novo chegou — o **vigia**
   do Modo 1 (aquele que olha `local.tip()` a cada segundo) já existia e já
   reage sozinho, porque agora é o TIP LOCAL que mudou. A sincronização de
   rede e a corrida de mineração continuam sendo dois mecanismos separados
   que nem sabem um do outro.

### O que muda quando você mesmo acha um bloco

Com `-peer`, minerar um bloco não é mais "escreve no seu banco e pronto" — a
autoridade da corrida é o banco do peer (é ele que decide quem venceu cada
altura, com a mesma constraint `height PRIMARY KEY` do Modo 1). O fluxo
(`submitBlock` em powdemo.go):

1. Envia o bloco pro peer (`insert_block` por TCP).
2. **Peer aceitou** → espelha o mesmo bloco no seu banco local (agora os
   dois bancos concordam nessa altura).
3. **Peer recusou (corrida perdida)** → busca no peer quem venceu de verdade
   e espelha O BLOCO DO VENCEDOR localmente — nunca o seu. Se não fizesse
   isso, seu banco local divergiria silenciosamente do que a rede aceitou.

### O que muda quando o peer cai

Esta é a pergunta que motivou o Modo 2 ter `-db` obrigatório. Se o Mac que
você sincronizou originalmente for desligado:

- Você **não trava**: `submitBlock` recebe um erro de rede (não uma corrida
  perdida) e o minerador só avisa e tenta de novo daqui a 1 segundo —
  continua minerando normalmente, usando seu próprio banco como base.
- Você **não fica cego**: seu banco já tem a cópia completa de tudo que
  existia até o momento em que o peer caiu — `node blocks -db bob.db`
  funciona sem rede nenhuma.
- Você **pode virar peer de um terceiro node**: se você também tiver rodado
  com `-listen`, um Carol pode apontar `-peer` pra VOCÊ e sincronizar uma
  cópia completa — mesmo que o Mac original da Alice já esteja desligado há
  horas. Isso é literalmente "um node em cada casa": a rede não depende de
  nenhuma máquina específica continuar de pé.

O que continua sendo uma simplificação da demo: a autoridade de cada
submissão ainda é UM peer só (quem você apontou com `-peer`), não um voto
entre vários — se dois peers diferentes aceitassem blocos divergentes pra
mesma altura ao mesmo tempo, não há reconciliação automática entre eles (é
exatamente o fork choice por trabalho acumulado que o M2/M4 resolvem de
verdade). Pra uma corrida de dois Macs em casa, isso não chega a aparecer.

### Ver com os próprios olhos

```sh
# Mac A — abre a corrida:
./bin/node powdemo -db alice.db -name Alice -listen :9551 -blocks 0 -spacing 1m -zeros 10

# Mac B — sincroniza (repare no "📡 sincronizando..." na entrada) e também
# expõe sua própria cópia, pra poder servir um terceiro Mac depois:
./bin/node powdemo -db bob.db -peer <ip-do-mac-a>:9551 -listen :9552 -name Bob -blocks 0

# Confirme que bob.db é uma cópia de verdade, sem tocar na rede:
./bin/node blocks -db bob.db -last 10

# Desligue o Mac A (Ctrl+C na Alice) e observe o Mac B no log: ele avisa
# "não consegui publicar... tentando de novo" mas continua minerando.

# Ligue um Mac C sincronizando DO MAC B (não do Mac A, que está desligado):
./bin/node powdemo -db carol.db -peer <ip-do-mac-b>:9552 -name Carol -blocks 0
```
