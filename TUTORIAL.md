# Tutorial PANDA — sua wallet e seu node, passo a passo

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

⚠️  faça backup deste arquivo AGORA: quem perde a chave perde os fundos,
   e não existe recuperação — é assim que blockchain funciona.
```

O que aconteceu:

- Nasceu o arquivo `wallet.json` com permissão `0600` (só o seu usuário lê).
- Nenhum servidor foi consultado. Nenhum cadastro. A chave nasceu do gerador
  criptográfico do seu sistema e nunca saiu da sua máquina.
- O comando **se recusa a rodar de novo** se `wallet.json` já existe —
  sobrescrever uma wallet destruiria a chave sem volta. Para uma segunda
  wallet, use outro arquivo: `wallet new -file outra.json`.

### Backup (não pule isto)

Copie `wallet.json` para pelo menos um lugar fora desta máquina — um pendrive
guardado, outro computador, um gerenciador de senhas. Regras de ouro:

- **Perdeu o arquivo = perdeu os fundos.** Não existe "esqueci a senha",
  suporte, nem autoridade que recupere. É o preço da descentralização.
- Quem **copia** o arquivo passa a poder gastar seus fundos. Trate o backup
  com o mesmo cuidado que dinheiro vivo.

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

## Parte 3 — Em que estágio isso está (leia antes de empolgar)

A rede que você acabou de rodar é a **bancada didática** (`powdemo`): o proof
of work, o retarget, a corrida entre mineradores e a reconciliação por
trabalho acumulado são **reais** — mas os blocos não carregam transações, e a
"carteira" do placar é um contador por nome de minerador no banco da demo,
**ainda não ligada à sua wallet da Parte 1**.

Por baixo, o node de verdade já existe como biblioteca e está testado:

- **Consenso completo (M2)**: validação total de blocos (PoW, merkle,
  timestamps, UTXOs, assinaturas, coinbase limitada a subsídio+taxas),
  storage transacional, fork choice e reorg seguro. Aqui, "inserir no banco
  na mão" não gera moedas — bloco só entra queimando trabalho e seguindo as
  regras.
- **Wallet e mempool (M3)**: a wallet da Parte 1 já constrói e assina
  transações válidas; o mempool valida, ordena por taxa e sobrevive a reorgs.

O que falta para a sua wallet receber recompensas de verdade:

- **M4 — rede P2P real**: handshake, gossip de blocos/transações e sync
  inicial usando a chain do M2 (substitui o mecanismo da demo).
- **M5 — o node completo**: `node run --mine` minerando com coinbase para o
  SEU endereço, e `node balance` / `node send` falando com o node via RPC.

Quando o M5 chegar, este tutorial ganha a parte final: enviar PANDA do seu
node para o endereço de outra pessoa e ver o saldo confirmar do outro lado.
