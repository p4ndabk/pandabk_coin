# Como um minerador sabe "em qual bloco está" (powdemo -db)

Pergunta: quando um segundo minerador entra na corrida, ele precisa se
sincronizar com o primeiro? Sim — mas de um jeito bem mais simples do que a
sincronização de uma rede P2P de verdade (que vem no M4). Aqui não existe
troca de mensagens entre os processos: **os dois só leem e escrevem o mesmo
arquivo SQLite**. A "sincronização" é o ato de ler esse arquivo antes de
minerar.

## Conceito

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

## O que acontece quando um minerador entra

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

## E durante a mineração? (a parte contínua)

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

## E se os dois acharem quase ao mesmo tempo? (a corrida de verdade)

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

## Resumindo em uma frase

Não existe protocolo de sincronização — existe um arquivo compartilhado, uma
leitura na entrada (`tip()` + `initMeta()`), um polling de 1s durante a busca
(o vigia), e uma constraint de banco (`height PRIMARY KEY`) como árbitro
final da corrida. A "rede" inteira é essas quatro peças.

## Ver com os próprios olhos

```sh
# Termine de ler este arquivo, depois olhe o histórico com o tempo de cada bloco:
./bin/node blocks -db mineracao.db -last 15

# Repare no campo "quando" (hora:min:seg): blocos do MESMO minerador em
# sequência rápida = ele ganhou a corrida sozinho por um tempo; blocos
# ALTERNANDO de minerador em segundos = os dois estavam disputando cada
# bloco (dificuldade baixa pro tamanho do hashrate combinado).
```
