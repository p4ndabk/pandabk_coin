# PANDA Coin — Documentação Oficial

> A PANDA é uma criptomoeda proof-of-work **memory-hard (Argon2id)**: minerar
> usa 1 core e ~64 MiB de RAM, então um notebook usado ou um Raspberry Pi
> competem de igual para igual — sem ASICs, sem fazendas. O princípio que
> decide todo trade-off do projeto: **um node em cada casa**.
>
> ⚠️ **Isto é uma devnet.** A rede atual é de desenvolvimento: os parâmetros
> econômicos ainda podem mudar e a chain pode ser reiniciada. Não trate
> PANDA de devnet como dinheiro.

---

## Índice

1. [Instalação (build para Linux, macOS e Windows)](#1-instalação)
2. [Sua wallet](#2-sua-wallet)
3. [Seu primeiro node](#3-seu-primeiro-node)
4. [Comandos do dia a dia](#4-comandos-do-dia-a-dia)
5. [Configuração (panda.conf, flags e variáveis de ambiente)](#5-configuração)
6. [Rede com vários nodes](#6-rede-com-vários-nodes)
7. [Node sempre ligado (o servidor da turma)](#7-node-sempre-ligado)
8. [Mineração](#8-mineração)
9. [Usando com Tor](#9-usando-com-tor)
10. [Referência de todos os comandos](#10-referência-de-todos-os-comandos)
11. [Os módulos por dentro](#11-os-módulos-por-dentro)
12. [Solução de problemas](#12-solução-de-problemas)
13. [App de desktop (sem terminal)](#13-app-de-desktop)

---

## 1. Instalação

O node é **um único binário estático, sem nenhuma dependência** — depois de
buildado, é copiar o arquivo para qualquer máquina e rodar.

### Requisitos (só para buildar)

- [Go](https://go.dev/dl/) 1.25 ou superior
- O código-fonte: `git clone <repo> && cd pandabk_coin`

### Build na sua própria máquina

**Linux / macOS:**

```sh
CGO_ENABLED=0 go build -o bin/panda-node ./cmd/node
```

**Windows (PowerShell):**

```powershell
$env:CGO_ENABLED = "0"
go build -o bin\panda-node.exe .\cmd\node
```

### Build para outras máquinas (scripts oficiais)

Você builda no seu computador e manda **um pacote pronto** para os amigos —
eles não precisam de Go, nem de instalar nada. Cada sistema tem seu script:

```sh
scripts/build-linux.sh      # Linux: PCs/servidores (amd64) + Raspberry Pi (arm64)
scripts/build-macos.sh      # macOS: Apple Silicon (arm64) + Intel (amd64)
scripts/build-windows.sh    # Windows: .exe para amd64 + arm64
scripts/build-all.sh        # os três de uma vez
```

Cada script monta um pacote completo em `dist/<so>/` e o compactado
versionado para enviar (`dist/panda-<versão>-<so>.tar.gz`, `.zip` no
Windows). Dentro do pacote:

```
dist/macos/
  panda-node-arm64      binários estáticos do node (o instalador escolhe)
  panda-node-amd64
  panda-desktop         o app com janela — entra quando o build roda no próprio SO*
  panda.conf            configuração pronta para editar (linha peers=)
  instalar.sh           escolhe o binário da CPU, dá permissão (e no macOS
                        remove a quarentena do Gatekeeper); Windows: instalar.bat
  LEIA-ME.txt           instruções de 3 passos para o amigo
  VERSAO.txt            versão, regras de consenso do build, data e commit
  SHA256SUMS.txt        conferência de integridade de cada arquivo
```

O amigo recebe o compactado, extrai, roda o instalador, edita o
`panda.conf` e sobe o node — três passos, descritos no LEIA-ME.

\* o desktop usa cgo e não cross-compila: `build-macos.sh` num Mac inclui o
desktop de Mac; para o desktop de Linux/Windows, rode o script no próprio
sistema (o pacote avisa quando ele ficou de fora).

Os scripts leem o **`build.conf`** (copie de `build.conf.example`), que é a
configuração **do desenvolvedor que compila** — responsabilidade separada
do `panda.conf`, que configura o **node em execução** (seção 5):

```ini
# build.conf — só de quem compila
name=panda-node      # nome-base dos binários
outdir=dist          # pasta de saída
version=0.1.0-dev    # aparece em `panda-node version` e no banner do run

# Regras de consenso DESTE build (opcionais — a economia da sua rede):
#spacing=1m          # meta de tempo por bloco
#halving=1000        # recompensa cai pela metade a cada N blocos
#subsidy=50          # recompensa inicial em PANDA inteiros
#retarget=100        # reajusta a dificuldade a cada N blocos (menor = corrige mais rápido)
#profile=devnet      # perfil default do binário
```

> ⚠️ **Regras definidas no build formam uma rede própria.** Mudar
> `spacing`/`halving`/`subsidy` muda o bloco gênesis — e o gênesis é o ID da
> rede no handshake: o binário só conversa com outros compilados com as
> **mesmas** regras (os demais são recusados com `gênesis diferente — outra
> rede`). É proposital: builds diferentes não se contaminam. Distribua o
> mesmo build para toda a turma; o banner do `run` e o `panda-node info`
> mostram as regras embutidas.

> 💡 No Linux/macOS, depois de copiar o binário: `chmod +x panda-node`.
> No macOS, se o Gatekeeper reclamar de binário baixado:
> `xattr -d com.apple.quarantine ./panda-node`.

---

## 2. Sua wallet

Uma wallet **não guarda moedas — guarda uma chave secreta**. As moedas vivem
na blockchain, trancadas para o seu endereço; a chave é o que destranca.

```sh
./panda-node wallet new
```

```
🔑 wallet nova em wallet.json (permissão 0600)
   endereço: PPMA1Lvdx6cNF6pkanYJzza1sfJBi3ucS1

📝 SUAS 12 PALAVRAS — anote NUM PAPEL, nesta ordem, e guarde bem:

    1. degree       5. mango       9. blue
    2. convince     6. lunar      10. hazard
    3. shine        7. crawl      11. matrix
    4. tourist      8. useful     12. tent
```

As **12 palavras** (padrão BIP39, o mesmo do Bitcoin) são o seu backup
humano: em qualquer máquina, elas reconstroem exatamente a mesma carteira —

```sh
./panda-node wallet restore -file wallet.json palavra1 palavra2 ... palavra12
```

Três regras que não têm exceção:

1. **Anote as 12 palavras num papel agora** — elas aparecem uma única vez.
   Papel guardado = carteira recuperável para sempre, mesmo se o computador
   morrer. Não existe "esqueci a senha".
2. **Nunca mostre as palavras (nem o `wallet.json`) a ninguém.** Quem lê,
   gasta.
3. O **endereço** (começa com `P`) é público — é ele que você compartilha
   para receber.

Para rever seu endereço depois: `./panda-node wallet address`. Esqueceu de
anotar? Enquanto o arquivo existir, `./panda-node wallet words` reexibe as
palavras (elas ficam gravadas dentro do próprio `wallet.json` — mais um
motivo para o arquivo nunca sair do seu controle).

> 💡 Você nem precisa deste passo para começar: o `node run` cria uma wallet
> sozinho no primeiro uso (em `~/.panda/wallet.json`). Este comando existe
> para quem quer criar/guardar a chave antes, ou ter mais de uma.

---

## 3. Seu primeiro node

```sh
./panda-node run
```

Pronto. O que acontece:

```
🐼 PANDA node no ar

   perfil       devnet
   datadir      /Users/voce/.panda
   p2p          [::]:9551
   rpc          127.0.0.1:8555   (info/balance/send falam aqui)
   mineração    LIGADA — 1 worker(s), 1 core e ~64 MiB cada
```

- O node guarda **sua cópia da blockchain** em `~/.panda/chain.db` e valida
  cada bloco por conta própria — você não confia em ninguém.
- **Ele já está minerando** (1 core, ~64 MiB). Cada bloco que você achar
  paga 50 PANDA para a sua wallet.
- Se não existia wallet no datadir, ele criou uma e mostrou o endereço no
  log — **faça o backup**.
- `Ctrl+C` encerra com segurança (nunca corrompe o banco). As transações
  ainda pendentes são salvas em `mempool.json` no datadir e voltam para a
  fila no próximo boot (revalidadas — se confirmaram enquanto o node dormia,
  são descartadas). Ao conectar num peer, os nodes também trocam as
  pendentes que cada um conhece, então uma transação sobrevive enquanto
  qualquer node da rede estiver de pé.

Sozinho, seu node minera uma chain só sua. A graça começa na
[seção 6](#6-rede-com-vários-nodes), conectando com os amigos.

---

## 4. Comandos do dia a dia

Com o node rodando, abra **outro terminal** (os comandos falam com o node
pela porta RPC local — nunca abrem o banco diretamente):

```sh
./panda-node info
```
```
perfil       devnet
altura       1204
tip          30b3c9159b5c7461...
dificuldade  5.10 (bits 1e032500)
alvo         1 bloco a cada 60s
recompensa   25 PANDA (próximo halving no bloco 2000)
peers        3
mempool      2 tx(s)
minerando    sim (24.6 H/s)
endereço     PPMA1Lvdx6cNF6pkanYJzza1sfJBi3ucS1
```

> 💡 **Dificuldade 1.00 e blocos saindo rápido demais?** Toda rede nova
> começa na dificuldade mínima; o retarget corrige a cada 100 blocos (até
> 4× por vez) até o ritmo bater no alvo — acompanhe pelas linhas `🎯` no
> log do node.

```sh
./panda-node balance
```
```
endereço   PPMA1Lvdx6cNF6pkanYJzza1sfJBi3ucS1
saldo      6350 PANDA (127 UTXOs)
gastável   5900 PANDA (coinbases maduras, nada pendente)
```

> 💡 **Por que "saldo" ≠ "gastável"?** Recompensa de mineração só pode ser
> gasta 10 blocos depois de minerada (regra de consenso — protege contra
> reorgs). O saldo total inclui as recompensas ainda "maturando".

```sh
./panda-node send -to PEC69ijTweUjXAGF81hExKRRVctgNpJuXp -amount 1.5
```
```
📤 enviado! txid 8a3bd0c1...
   a transação está no mempool; confirma quando um minerador incluí-la num bloco.
```

A transação viaja pela rede, um minerador qualquer a inclui num bloco, e o
destinatário vê o valor no `balance` dele — normalmente em 1–2 blocos.

```sh
./panda-node block 42        # explora um bloco por dentro (vazio = a ponta)
```
```
bloco        42  (301 confirmação(ões))
hash         9c01ab34...
quando       2026-07-05 13:01:02
dificuldade  4.00 (bits 1e040000)  nonce 1183
transações   2

tx 1  4f9e21c07a55...  (coinbase — a recompensa deste bloco)
   →  PPMA1Lvdx6cNF6pkanYJzza1sfJBi3ucS1  10 PANDA

tx 2  8a3bd0c1e99f...
   gasta  7bc19d02aa14...:0
   →  PEC69ijTweUjXAGF81hExKRRVctgNpJuXp  1.5 PANDA
   →  PPMA1Lvdx6cNF6pkanYJzza1sfJBi3ucS1  8.49 PANDA
```

É a blockchain sem mistério: a primeira transação de todo bloco é a
coinbase (quem minerou, recebendo a recompensa), e as demais mostram cada
UTXO gasto e para onde os valores foram. No app de desktop, a aba
**Blocos** faz o mesmo.

---

## 5. Configuração

> **Dois arquivos, duas responsabilidades:** o `build.conf` (seção 1) é de
> quem **compila**; o `panda.conf` desta seção é de quem **roda o node**.
> Quem só recebe o binário pronto nunca toca no `build.conf`.

Três formas, nesta ordem de precedência (a de cima vence):

1. **Flag na linha de comando** — `./panda-node run -listen :9552`
2. **Arquivo `panda.conf`** — no diretório onde você roda o node
3. **Variável de ambiente** — `NODE_LISTEN=:9552`

### panda.conf

Formato `chave=valor`, uma por linha, `#` comenta. As chaves são os nomes
dos flags. Um arquivo típico de quem roda um node:

```ini
# panda.conf
datadir=/home/voce/.panda
listen=:9551                  # aceita conexões de outros nodes
peers=192.168.1.10:9551       # quem você conhece (vírgula separa vários)
mine=true
miners=1
profile=devnet
```

Com o arquivo no lugar, `./panda-node run` basta.

### Flags e variáveis do `run`

| Flag | Env | Default | O que faz |
|---|---|---|---|
| `-datadir` | `NODE_DATADIR` | `~/.panda` | onde vivem `chain.db` e `wallet.json` |
| `-listen` | `NODE_LISTEN` | `:9551` | porta P2P; **vazio = só conexões de saída** (funciona atrás de NAT sem configurar nada) |
| `-rpc` | `NODE_RPC` | `127.0.0.1:8555` | RPC local (recusa qualquer endereço fora de loopback) |
| `-peers` | `NODE_PEERS` | — | nodes iniciais, `host:porta` separados por vírgula |
| `-peer` | — | — | atalho para um único peer |
| `-proxy` | `NODE_PROXY` | — | proxy SOCKS5 para **toda** conexão de saída (ex.: `127.0.0.1:9050` do Tor — permite discar peers `.onion`); vazio = direto |
| `-advertise` | `NODE_ADVERTISE` | — | endereço anunciado aos peers (ex.: `seuendereco.onion:9551` de um hidden service); vazio = o do `-listen` |
| `-mine` | `NODE_MINE` | `true` | mineração (desligue com `-mine=false`) |
| `-miners` | `NODE_MINERS` | `1` | workers de mineração (1 core + ~64 MiB cada) |
| `-profile` | `NODE_PROFILE` | `devnet` | perfil de consenso (`devnet` ou `test`) |

---

## 6. Rede com vários nodes

### Dois nodes na mesma máquina (para experimentar)

```sh
# terminal 1
./panda-node run -datadir ~/.panda/n1 -listen :9551 -rpc 127.0.0.1:8551
# terminal 2
./panda-node run -datadir ~/.panda/n2 -listen :9552 -rpc 127.0.0.1:8552 -peers 127.0.0.1:9551
```

O segundo node baixa e **valida** toda a chain do primeiro (sync inicial),
e daí em diante os dois competem minerando. Confira com
`./panda-node info -rpc 127.0.0.1:8552` — as alturas convergem.

### Nodes em máquinas diferentes (a rede da turma)

Na máquina que vai receber conexões, descubra o IP local:

```sh
ipconfig getifaddr en0    # macOS
hostname -I               # Linux
ipconfig                  # Windows
```

Cada amigo aponta o `panda.conf` para quem ele conhece:

```ini
# na casa do João (conhece o servidor da Maria)
peers=192.168.1.10:9551
```

Não precisa todo mundo conhecer todo mundo: os nodes trocam endereços entre
si (`getaddr`/`addr`) e o address book fica salvo no banco — depois da
primeira conexão, seu node redisca a rede sozinho, mesmo que o peer
original esteja offline.

**Como a rede se mantém coerente, em uma frase cada:**

- Blocos e transações se espalham por **fofoca** (`inv`/`getdata` — nada
  trafega duas vezes).
- Todo node **valida tudo** — bloco inválido morre na primeira casa que
  chegar, não importa quem mandou.
- Se dois mineradores acham blocos quase juntos, vence o ramo com **mais
  trabalho acumulado**; o outro é reorganizado automaticamente.

> 💡 **Atrás de roteador (NAT)?** Rode com `-listen ""` (ou `listen=` vazio
> no conf). Seu node só abre conexões de saída — e mesmo assim é cidadão
> pleno: valida, minera e propaga. Abrir porta no roteador (port forward de
> `9551`) é opcional e só ajuda a rede a ter mais pontos de entrada.

---

## 7. Node sempre ligado

Toda rede de amigos merece uma máquina que nunca dorme — é o ponto de
encontro que os outros usam como primeiro `peers=`.

### Linux (systemd)

`/etc/systemd/system/panda-node.service`:

```ini
[Unit]
Description=PANDA Coin node
After=network-online.target

[Service]
User=panda
ExecStart=/home/panda/panda-node run -datadir /home/panda/.panda
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```sh
sudo systemctl enable --now panda-node
journalctl -u panda-node -f        # acompanhar o log
```

### macOS

```sh
caffeinate -is ./panda-node run    # segura o sleep enquanto o node roda
```

Notebook de tampa fechada precisa também de
`sudo pmset -a disablesleep 1` (e ficar na tomada).

### Windows

Agende o `panda-node.exe run` no Agendador de Tarefas ("ao iniciar o
computador", "executar mesmo sem usuário logado") e desative a suspensão em
Energia.

---

## 8. Mineração

**Todo node minera por padrão.** É uma decisão de projeto: com PoW
memory-hard, a segurança da rede vem da *quantidade* de participantes, não
da potência de cada um — cada casa minerando um pouco é o que descentraliza.

- O custo é fixo e baixo: **1 core + ~64 MiB de RAM por worker** (default 1).
- Cada bloco achado paga **50 PANDA** (devnet) para a wallet do datadir.
- A recompensa "matura" por **10 blocos** antes de poder ser gasta.
- A dificuldade se ajusta sozinha a cada 100 blocos para manter ~1 bloco
  por minuto (devnet), não importa quantos nodes entrem ou saiam.
- Quer doar mais cores? `-miners 2` (cada um soma 1 core + 64 MiB).
- Só validar, sem minerar: `-mine=false`.

> 💡 Num notebook na bateria, 1 core constante esquenta. Rode na tomada, ou
> desligue a mineração quando estiver na rua — o node continua útil para a
> rede só validando e repassando.

---

## 9. Usando com Tor

Tor esconde o IP do seu node (de quem conecta em você e de quem você
conecta). O node tem suporte nativo: `-proxy` roteia toda a saída pelo
SOCKS5 do Tor (e é o Tor quem resolve os `.onion`), e `-advertise` faz um
hidden service anunciar o `.onion` aos peers em vez do endereço local.
Funciona igual em Linux, macOS e Windows — sem `torsocks`.

### 9.1 Instalar o Tor

```sh
# Debian/Ubuntu/Raspberry Pi
sudo apt install tor
# macOS
brew install tor
# Windows: instale o "Tor Expert Bundle" (torproject.org)
```

### 9.2 Receber conexões como serviço onion (hidden service)

Edite o `torrc` (`/etc/tor/torrc` no Linux, `/opt/homebrew/etc/tor/torrc`
no macOS) e acrescente:

```
HiddenServiceDir /var/lib/tor/panda-node/
HiddenServicePort 9551 127.0.0.1:9551
```

Reinicie o Tor (`sudo systemctl restart tor` / `brew services restart tor`)
e leia seu endereço onion:

```sh
sudo cat /var/lib/tor/panda-node/hostname
# exemplo: pandaxyzabc...def.onion
```

Rode o node aceitando conexões **só do Tor local** (o mundo externo não vê
a porta, só o onion) e anunciando o `.onion` aos peers:

```sh
./panda-node run -listen 127.0.0.1:9551 -advertise pandaxyzabc...def.onion:9551
```

Compartilhe `pandaxyzabc...def.onion:9551` com os amigos — esse é o seu
endereço na rede, sem expor seu IP nem abrir porta no roteador. Sem o
`-advertise`, o node anunciaria o `127.0.0.1:9551` local (inútil para os
outros); com ele, o peer exchange espalha seu onion e outros nodes Tor
conseguem te descobrir.

### 9.3 Conectar em peers .onion

Quem disca para um `.onion` aponta o `-proxy` para o SOCKS5 do Tor local
(porta 9050 por padrão) — o destino é resolvido pelo próprio Tor, nenhum
DNS ou TCP vaza por fora:

```sh
./panda-node run -proxy 127.0.0.1:9050 -peers pandaxyzabc...def.onion:9551 -listen ""
```

No `panda.conf` (e na aba Ajustes do desktop) são as chaves `proxy=` e
`peers=`. Os dois lados podem combinar os papéis: receber como hidden
service (9.2) **e** discar via `-proxy` — aí ninguém expõe IP.

> ⚠️ **Ressalvas honestas:**
> - Latência via Tor é maior e o circuito demora alguns segundos para
>   nascer; o sync inicial é mais lento. Para a rede de amigos em LAN, Tor
>   é opcional — ele brilha para nodes distantes que não querem expor IP
>   nem mexer em roteador.
> - Com `-proxy`, **toda** a saída passa pelo Tor (de propósito, para não
>   vazar). Se o daemon `tor` não estiver rodando, o node fica sem
>   conexões de saída — confira com `panda-node info` (peers > 0).

---

## 10. Referência de todos os comandos

### O node de verdade

| Comando | O que faz | Flags principais |
|---|---|---|
| `run` | sobe o full node (chain + mempool + p2p + miner) | ver [seção 5](#5-configuração) |
| `info` | altura, dificuldade, recompensa/halving, peers, mempool, hashrate | `-rpc` |
| `balance` | saldo de um endereço | `-rpc`, `-address` (default: wallet do node) |
| `send` | envia PANDA | `-rpc`, `-to P...`, `-amount 1.5`, `-fee-rate` |
| `block` | explora um bloco: coinbase, transações, valores, destinos | `-rpc`, altura ou hash (vazio = ponta) |
| `wallet new` | cria wallet + 12 palavras de backup (0600; nunca sobrescreve) | `-file` ou `-datadir` |
| `wallet restore` | recupera a wallet a partir das 12 palavras | `-file` ou `-datadir`, palavras como argumentos |
| `wallet words` | reexibe as 12 palavras gravadas no wallet.json | `-file` ou `-datadir` |
| `wallet address` | reexibe o endereço | `-file` ou `-datadir` |
| `genesis` | (dev) minera o bloco 0 de um perfil | `-profile` |
| `version` | versão do binário (do `build.conf` de quem compilou) | — |

Todos aceitam `-config caminho.conf` (default: `panda.conf` do diretório
atual, se existir).

### A bancada didática (a demo que precedeu o node)

Boa para *ver* a mineração acontecer com placar e logs verbosos — mas os
blocos dela não carregam transações; a rede de verdade é o `run`.

| Comando | O que faz | Flags principais |
|---|---|---|
| `powdemo` | corrida de mineradores com PoW real | `-name`, `-db`, `-listen`, `-peer`, `-spacing`, `-zeros`, `-blocks`, `-workers` |
| `blocks` | últimos blocos de uma corrida | `-db` ou `-peer`, `-last` |
| `ranking` | placar por minerador | `-db` ou `-peer` |

---

## 11. Os módulos por dentro

Para quem quiser ler o código (cada pacote tem um `SPEC.md` com uma seção
"Conceito" didática):

| Módulo | Papel | Conceito que ensina |
|---|---|---|
| `internal/params` | perfis de consenso (devnet/test), subsídio, halving | escassez programada |
| `internal/core` | bloco, transação UTXO, merkle, endereços, serialização canônica | as peças do jogo |
| `internal/pow` | Argon2id, target/nBits, retarget | por que memory-hard democratiza |
| `internal/chain` | validação completa, UTXO set, fork choice, reorg (bbolt) — o banco explicado gaveta a gaveta em [BLOCKS.md](./BLOCKS.md) | o consenso em si |
| `internal/mempool` | fila de transações por taxa, anti double-spend | o mercado de espaço no bloco |
| `internal/wallet` | chaves, wallet.json, construção/assinatura de tx | posse = assinatura |
| `internal/p2p` | handshake, gossip, sync inicial, address book | a descentralização literal |
| `internal/miner` | template, workers, restart em novo tip | a loteria proporcional |
| `internal/node` | orquestração, config, RPC localhost | tudo junto num processo |

A direção de dependência é estrita:
`params ← core ← pow ← chain ← {mempool, wallet} ← {p2p, miner} ← node ← cmd/node`.

---

## 12. Solução de problemas

| Sintoma | Causa e solução |
|---|---|
| `porta RPC em uso?` | outro node usando a mesma `-rpc` — troque a porta ou pare o outro processo |
| `abrindo a chain ... outro node rodando no mesmo datadir?` | o bbolt aceita 1 processo por banco — cada node precisa do seu `-datadir` |
| `não consegui falar com o node em 127.0.0.1:8555` | o `run` não está de pé, ou está noutra porta — confira com `-rpc` |
| `a RPC só aceita bind em loopback` | por segurança a RPC nunca escuta na rede — para controlar de longe, use SSH até a máquina |
| `gênesis diferente — outra rede` no log | o peer roda outro perfil (`devnet` × `test`) ou outra versão do genesis — alinhem o `-profile` |
| saldo não cresce ao minerar | as recompensas maturam por 10 blocos — veja a linha `gastável` do `balance` |
| `wallet: arquivo já existe` | proteção contra sobrescrever chave — para outra wallet, use outro `-file` |
| `permissão do arquivo insegura` | `chmod 600 wallet.json` |
| dois nodes não conectam na LAN | firewall bloqueando a porta 9551 da máquina com `-listen`; teste `nc -vz IP 9551` |

---

## 13. App de desktop

Para quem prefere uma **janela** a um terminal: o `panda-desktop` faz o
mesmo que o console — e a mais: se não houver node rodando, **ele mesmo
vira o node**. Interface nativa escrita em Go (Fyne) — sem Electron, sem
navegador embutido — com visual clean claro/escuro automático.

### Como funciona (híbrido)

Ao abrir, o app procura um node na RPC local (`127.0.0.1:8555` ou o que
estiver no `panda.conf`):

- **Achou** → vira um *painel* do node que já roda no terminal/servidor.
- **Não achou** → sobe o node **dentro do próprio app** (mesma config:
  flags > `panda.conf` > `NODE_*`), minerando por padrão. Fechar a janela
  desliga tudo com segurança.

Na **primeira vez** (sem `panda.conf` salvo), o app abre uma tela de
boas-vindas pedindo o essencial — o peer de quem te convidou, se quer
minerar, portas — salva tudo e só então liga o node. A configuração fica em
`~/.panda/panda.conf` (o app acha sozinho, pode abrir por clique duplo) e é
editável a qualquer momento pela aba **Ajustes**, com a opção de reiniciar
o node na hora para aplicar.

Seis abas: **Início** (o painel completo, estilo mempool.space caseiro:
saldo, altura, dificuldade, peers, hashrate, tempo médio por bloco vs
alvo, contagem para o retarget com previsão de subida/queda da
dificuldade, contagem para o halving, os últimos blocos com quem minerou
cada um, e a fila de transações esperando bloco), **Carteira** (endereço +
copiar, aviso de backup e o **extrato**: cada entrada e saída confirmada
com valor, bloco, data e a contraparte — recompensas de mineração
marcadas, taxa mostrada à parte nas saídas, com filtro
tudo/transações/mineração e paginação), **Enviar**
(com confirmação), **Blocos** (explorador: as transações dentro de cada
bloco), **Atividade** (os logs do node ao vivo: peers conectando, blocos,
retarget, halving) e **Ajustes** (o panda.conf na interface).

### Build

A GUI usa cgo (renderização nativa), então **builde em cada sistema**. O
pacote de distribuição (`scripts/build-<so>.sh`, seção 2) **já inclui o
desktop** quando roda no próprio SO — este script é o atalho de
desenvolvimento que builda só a GUI, reusando o `build.conf` (versão E
regras de consenso, que valem também para o node embutido):

```sh
scripts/build-desktop.sh      # sai em dist/panda-desktop-<os>-<arch>
```

Pré-requisitos de compilação (só para quem builda; o binário final não
depende de nada):

| Sistema | Precisa de |
|---|---|
| macOS | Xcode Command Line Tools (`xcode-select --install`) |
| Linux | `gcc`, `libgl1-mesa-dev`, `xorg-dev` |
| Windows | MinGW-w64 (ou builde no WSL com os pacotes de Linux) |

> 💡 Cross-compile da GUI é possível com
> [fyne-cross](https://github.com/fyne-io/fyne-cross) (usa Docker), mas o
> caminho simples é rodar o script uma vez em cada sistema.

O node de terminal continua sendo o binário estático de sempre — o desktop
é opcional, para quem quiser.

---

*Documentação da devnet PANDA. Detalhes de design: [PLAN.md](../PLAN.md) ·
Guia narrado para iniciantes: [TUTORIAL.md](../TUTORIAL.md) · Visão do
projeto: [PROPOSTA.md](../PROPOSTA.md).*
