# Tutorial PANDA — sua wallet e seu node, passo a passo

> A referência completa (instalação em todos os sistemas, rede multi-node,
> Tor, todos os comandos) é a **[documentação oficial](docs/README.md)** —
> este tutorial é a versão narrada para a primeira vez.

> Guia para quem nunca rodou uma criptomoeda. Duas trilhas independentes:
> **Parte 1** cria sua wallet (5 minutos). **Parte 2** coloca um node PANDA
> para rodar na sua máquina e conversar com outros (15 minutos).
> A **Parte 3** explica com honestidade em que estágio o projeto está.

## Antes de começar

Você precisa do binário `panda-node`. Se recebeu o executável pronto, pule
este passo. Para compilar do código-fonte (requer [Go](https://go.dev) 1.25+):

```sh
git clone <repo> && cd pandabk_coin
CGO_ENABLED=0 go build -o bin/panda-node ./cmd/node
```

Sai um binário **estático, sem nenhuma dependência** — copie `bin/panda-node`
para qualquer Mac, Linux ou Raspberry Pi e ele roda. Isso é de propósito: a
meta do projeto é *um node em cada casa*.

Para buildar **para os amigos** (outros sistemas), use os scripts oficiais —
`scripts/build-linux.sh`, `scripts/build-macos.sh`, `scripts/build-windows.sh`
ou `scripts/build-all.sh` — que leem o `build.conf` do desenvolvedor e
deixam os binários em `dist/` com o sha256 de cada um (detalhes na
[documentação oficial](docs/README.md#1-instalação)).

---

## Parte 1 — Criar sua wallet

### O que você está prestes a criar

Uma wallet **não guarda moedas**. Ela guarda uma **chave secreta** (um número
gigante sorteado agora, na sua máquina, offline). As moedas vivem na
blockchain, trancadas para o "cadeado" derivado da sua chave; quem tem a
chave — e só quem tem a chave — consegue destrancá-las para gastar.

Da chave secreta deriva a chave pública, e dela o seu **endereço**, que
começa com `P`. O endereço você compartilha com qualquer um ("me paga aqui");
a chave secreta você não mostra **para ninguém, nunca**.

### Criando

```sh
./bin/panda-node wallet new
```

Você verá algo assim:

```
🔑 wallet nova em wallet.json (permissão 0600)
   endereço: PPMA1Lvdx6cNF6pkanYJzza1sfJBi3ucS1

📝 SUAS 12 PALAVRAS — anote NUM PAPEL, nesta ordem, e guarde bem:
    1. degree       5. mango       9. blue
    ...
```

As **12 palavras** (padrão BIP39, o mesmo de carteiras Bitcoin) aparecem
**uma única vez** e reconstroem sua carteira em qualquer máquina:
`panda-node wallet restore -file wallet.json palavra1 ... palavra12`.

O que aconteceu:

- Nasceu o arquivo `wallet.json` com permissão `0600` (só o seu usuário lê).
- Nenhum servidor foi consultado. Nenhum cadastro. A chave nasceu do gerador
  criptográfico do seu sistema e nunca saiu da sua máquina.
- O comando **se recusa a rodar de novo** se `wallet.json` já existe —
  sobrescrever uma wallet destruiria a chave sem volta. Para uma segunda
  wallet, use outro arquivo: `wallet new -file outra.json`.

### Backup (não pule isto)

O backup principal são as **12 palavras num papel** — guardadas, elas
recuperam a carteira mesmo que o computador vire fumaça. O `wallet.json` é
o cache local da chave; copiá-lo para um pendrive também vale. Regras de ouro:

- **Perdeu as palavras E o arquivo = perdeu os fundos.** Não existe "esqueci
  a senha", suporte, nem autoridade que recupere. É o preço da
  descentralização.
- Quem **lê as palavras** (ou copia o arquivo) passa a poder gastar seus
  fundos. Trate como dinheiro vivo: papel em lugar seguro, nunca em foto ou
  e-mail.

### Conferindo depois

```sh
./bin/panda-node wallet show           # reexibe seu endereço
```

Se aparecer `permissão do arquivo insegura`, algum processo/cópia relaxou as
permissões — rode `chmod 600 wallet.json` e tente de novo. Se aparecer
`arquivo corrompido ou inconsistente`, o conteúdo foi alterado; restaure do
backup.

---

## Parte 2 — Rodar seu node

### O que é um node e por que ter um

Um node é a sua **cópia da verdade**: ele guarda a chain inteira no seu disco
e confere cada bloco por conta própria. Você não confia em ninguém — nem em
quem te passou os blocos. Se além de validar você também **minerar**, sua
máquina concorre a criar os próximos blocos e ganhar a recompensa. Na PANDA o
proof of work é o Argon2id (memory-hard): 1 core + 64 MiB de RAM competem de
igual para igual — notebook usado e Raspberry Pi valem tanto quanto qualquer
máquina.

### Passo 1 — arquivo de configuração

Em vez de repetir flags gigantes, crie um `panda.conf` no diretório onde o
node vai rodar:

```sh
cp panda.conf.example panda.conf
```

E edite. Para o **primeiro node** da sua rede (ex.: a máquina que fica sempre
ligada):

```ini
name=Servidor          # aparece nos logs e no placar
db=servidor.db         # sua cópia local da chain
listen=:9551           # expõe a chain para outros nodes da rede
spacing=1m             # alvo: 1 bloco por minuto
zeros=10               # dificuldade inicial
retarget=10            # reajusta a dificuldade a cada 10 blocos
blocks=0               # minerar sem parar (Ctrl+C para sair)
```

Flag na linha de comando sempre **vence** o arquivo, e `blocks`/`ranking`
aproveitam o mesmo `panda.conf`.

### Passo 2 — ligar

```sh
./bin/panda-node powdemo
```

Você verá o banner com a dificuldade atual e, a cada bloco encontrado:

```
✅ [15:25:35] bloco 42 minerado por Servidor!
   ⏱  38s para minerar (alvo 1m0s) | 412 tentativas | nonce 16
   recompensa  +50 PANDA  →  sua carteira: 2100 PANDA (42 blocos)
```

Cada tentativa de hash preenche 64 MiB de RAM — é isso que mantém ASICs fora
do jogo. O retarget observa o ritmo real e ajusta a dificuldade sozinho para
perseguir o `spacing` configurado.

Numa máquina que fica 24/7 (Mac):

```sh
caffeinate -is ./bin/panda-node powdemo     # segura o sleep enquanto roda
```

(Notebook de tampa fechada também precisa de `sudo pmset -a disablesleep 1`.)

### Passo 3 — conectar um segundo node

Na **outra máquina**, o `panda.conf` aponta para a primeira (descubra o IP
dela com `ipconfig getifaddr en0` no Mac):

```ini
name=David
db=david.db            # SEMPRE um banco próprio — ver regra de ouro abaixo
peer=192.168.1.10:9551 # o node sempre-ligado
listen=:9551           # opcional: você também vira peer de um terceiro
spacing=1m             # MESMAS regras do outro lado, sempre
zeros=10
retarget=10
blocks=0
```

**As regras (`spacing`/`zeros`/`retarget`) têm que ser idênticas nos dois
lados** — é o equivalente demo de "concordar no bloco gênesis". Ao subir:

- Se o banco local está vazio, o node **adota a chain inteira do peer**
  (o "sync inicial") e sai minerando do bloco seguinte.
- Os dois mineram de forma independente e se reconciliam continuamente: se
  as chains divergem, vence a com **mais trabalho acumulado** — o outro lado
  descarta o trecho perdedor (é o "reorg", igualzinho ao Bitcoin).
- Se o peer cair, seu node avisa (`📴 peer inalcançável`) e **continua
  minerando sozinho**; quando o peer volta (`🔌 peer voltou`), reconciliam.

**Regra de ouro**: todo node tem seu **próprio** `db=`. Sua cópia local é o
que te torna independente — você nunca fica refém de outra máquina estar de
pé, e automaticamente pode servir um terceiro node com `listen=`.

### Passo 4 — acompanhar

No mesmo diretório (o `panda.conf` é encontrado sozinho):

```sh
./bin/panda-node blocks -last 10     # últimos blocos da chain
./bin/panda-node ranking             # placar por minerador
```

### Problemas comuns

| Sintoma | O que é / o que fazer |
|---|---|
| `ℹ️ adotando as regras já gravadas em ...` | O banco foi criado com outras flags; as gravadas valem. Para regras novas, use outro arquivo `db=`. |
| `📴 peer inalcançável` | Normal se o outro node está desligado — você segue minerando sozinho e o reconcile resolve quando ele voltar. Confira IP/porta e firewall se persistir. |
| `🩹 removi N bloco(s) solto(s)...` no startup | Auto-reparo de um banco antigo com blocos fora de sequência. Inofensivo: os blocos voltam pelo reconcile, em ordem. |
| Primeiro bloco demorando muito | A dificuldade gravada reflete o poder de mineração de quando havia mais máquinas. O retarget alivia a cada `retarget` blocos (até 4× por vez). |

---

## Parte 3 — O node de verdade (`node run`)

A Parte 2 usa a **bancada didática** (`powdemo`) — ótima para VER a
mineração e a corrida, mas os blocos dela não carregam transações. O node
completo já existe e liga tudo: chain validada por consenso, mempool, rede
p2p e **mineração ligada por padrão pagando a SUA wallet**.

```sh
./bin/panda-node run
```

No primeiro `run`, o node cria sozinho uma wallet no datadir (default
`~/.panda/wallet.json`, permissão 0600) e mostra o endereço — **faça backup
dela** (Parte 1 explica por quê). O banner já mostra as regras do jogo e o
estado da sua chain:

```
🐼 PANDA node no ar

   perfil       devnet
   datadir      /Users/voce/.panda
   p2p          [::]:9551
   rpc          127.0.0.1:8555   (info/balance/send falam aqui)
   mineração    LIGADA — 1 worker(s), 1 core e ~64 MiB cada
   consenso     1 bloco a cada 1m0s | retarget a cada 100 blocos | halving a cada 1000
   recompensa   50 PANDA pelo próximo bloco
   chain        altura 342, dificuldade 4.00
```

E os logs narram a vida da rede — quem conecta, quem desconecta, cada bloco
com sua dificuldade, e os eventos de consenso:

```
🤝 peer 192.168.1.20:53112 conectado (entrada) — altura declarada 0
⛓️  192.168.1.10:9551 tem mais trabalho acumulado (altura 342 vs nossa 128) — sincronizando
📥 bloco 129 recebido da rede (1 txs, dificuldade 4.00) — 9c01ab34
✅ bloco 343 minerado (2 txs, dificuldade 4.00) — 3fca90e1
🎯 retarget no bloco 400: dificuldade 4.00 → 5.10 (alvo: 1 bloco a cada 1m0s)
✂️  halving no bloco 1000: recompensa agora 25 PANDA por bloco
👋 peer 192.168.1.20:53112 desconectado
```

> **Os blocos estão saindo bem mais rápido que o alvo?** Normal no começo de
> uma rede: todo mundo parte da **dificuldade mínima (1.00)** e o retarget só
> corrige a cada 100 blocos, subindo no máximo 4× por vez. Em 2–3 retargets
> (~200–300 blocos) o ritmo converge para o alvo de 1 bloco/minuto — acompanhe
> pelas linhas 🎯. É o mesmo comportamento do Bitcoin em 2009.

Daí em diante:

```sh
./bin/panda-node info                          # estado do node (abaixo)
./bin/panda-node balance                       # seu saldo (coinbases maduras)
./bin/panda-node send -to P... -amount 1.5     # envia PANDA
```

```
perfil       devnet
altura       342
tip          8a3f...
dificuldade  4.00 (bits 1e040000)
alvo         1 bloco a cada 60s
recompensa   50 PANDA (próximo halving no bloco 1000)
peers        2
mempool      0 tx(s)
minerando    sim (21.7 H/s)
endereço     PPMA1Lvdx6cNF6pkanYJzza1sfJBi3ucS1
```

Dois nodes na mesma máquina (ou dois computadores — troque 127.0.0.1 pelo
IP real):

```sh
# terminal 1
./bin/panda-node run -datadir ~/.panda/n1 -listen :9551 -rpc 127.0.0.1:8551
# terminal 2
./bin/panda-node run -datadir ~/.panda/n2 -listen :9552 -rpc 127.0.0.1:8552 -peers 127.0.0.1:9551
```

O segundo node baixa e **valida** cada bloco do primeiro (sync inicial),
depois os dois competem minerando e convergem sempre para a cadeia com mais
trabalho. Um `send` de um lado aparece no `balance` do outro após a
confirmação (a recompensa de minerar leva 10 blocos para "maturar" antes de
poder ser gasta — regra de consenso). Quem quiser só validar sem minerar:
`-mine=false`. O `panda.conf` também funciona aqui (chaves `datadir`,
`listen`, `rpc`, `peers`/`peer`, `mine`, `miners`, `profile`).

### Prefere uma janela?

Existe também o **app de desktop** (`scripts/build-desktop.sh`): a mesma
coisa que os comandos acima, numa interface nativa clean — e se não houver
node rodando, o próprio app vira o node. Detalhes na
[documentação oficial, seção 13](docs/README.md#13-app-de-desktop).

## Parte 4 — Em que estágio isso está (honestidade obrigatória)

Todos os 5 milestones do plano estão implementados e testados: consenso
completo (bloco só entra queimando Argon2id e seguindo todas as regras —
editar o banco na mão não gera moedas), wallet, mempool, rede p2p com sync
inicial e o node completo acima. O que ainda é verdade:

- **Isto é uma devnet**: rede de desenvolvimento, com economia de ciclos
  curtos (bloco de ~60s, halving a cada 1.000). Os parâmetros econômicos
  finais ainda estão em discussão (há inclinação para emissão constante sem
  halving) — a chain atual pode ser zerada/reiniciada até a rede "de
  verdade" nascer. Não trate PANDA de devnet como valor.
- **Sem descoberta automática de peers além do gossip**: você ainda aponta
  `-peers` para alguém que conhece (o address book propaga o resto).
- **RPC só em localhost**, sem autenticação — é a interface do dono do node.
