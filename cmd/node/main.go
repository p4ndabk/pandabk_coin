// O binário node é o full node PANDA. Nesta fase (M1.6) ele expõe a bancada
// de mineração powdemo — proof of work real (Argon2id) com retarget,
// recompensa simulada e uma corrida entre vários mineradores: na mesma
// máquina via SQLite compartilhado (-db) ou em máquinas diferentes via TCP
// (-listen/-peer) — além dos comandos de consulta blocks e ranking. Ver
// cmd/node/SINCRONIZACAO.md para como a sincronização funciona nos dois
// modos. Os subcomandos definitivos (run, wallet, send, ...) chegam nos
// milestones seguintes; ver internal/node/SPEC.md.
package main

import (
	"fmt"
	"os"
)

// version é injetada pelos scripts de build (scripts/build-*.sh) via
// -ldflags "-X main.version=...", lida do build.conf do desenvolvedor.
var version = "dev"

// banner é a cara do node no terminal — aparece no help e na subida do run.
const banner = `
                        __                      __
 .-----.---.-.-----.--|  .---.-.   .----.-----|__.-----.
 |  _  |  _  |     |  _  |  _  |   |  __|  _  |  |     |
 |   __|___._|__|__|_____|___._|   |____|_____|__|__|__|
 |__|                            🐼  um node em cada casa
`

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "run":
		runRun(os.Args[2:])
	case "info":
		runInfo(os.Args[2:])
	case "balance":
		runBalance(os.Args[2:])
	case "send":
		runSend(os.Args[2:])
	case "block":
		runBlock(os.Args[2:])
	case "powdemo":
		runPowDemo(os.Args[2:])
	case "blocks":
		runBlocks(os.Args[2:])
	case "ranking":
		runRanking(os.Args[2:])
	case "genesis":
		runGenesis(os.Args[2:])
	case "wallet":
		runWallet(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Printf("panda-node %s\n", version)
	case "help", "-h", "--help":
		if len(os.Args) > 2 {
			helpFor(os.Args[2])
		} else {
			usage()
		}
	default:
		fmt.Fprintf(os.Stderr, "subcomando desconhecido: %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(banner)
	fmt.Printf(`
 panda-node %s — o full node da PANDA Coin

USO
  panda-node <comando> [flags]
  panda-node help <comando>      flags de um comando (idem: <comando> -h)

O node de verdade:
  run       sobe o full node: chain validada (bbolt), mempool, rede p2p e
            mineração LIGADA por padrão (1 worker; -mine=false desliga).
            A coinbase paga a wallet do datadir (criada no primeiro run).
  info      altura/tip/peers/mempool/hashrate do node em execução (via RPC)
  balance   saldo de um endereço (default: a wallet do node)
  send      envia PANDA: node send -to P... -amount 1.5
  block     explora um bloco por dentro: node block 42 | node block <hash>
            (vazio = a ponta) — coinbase, transações, valores e destinos
  wallet    new: gera chave + 12 palavras de backup (BIP39); restore: recupera
            a carteira só com as palavras; words: reexibe as palavras;
            address: reexibe o endereço
  genesis   (dev) minera o bloco 0 de um perfil
  version   versão do binário (definida no build.conf de quem compilou)

Bancada didática (a demo que precedeu o node):
  powdemo   corrida de mineradores com PoW real e recompensa simulada
  blocks    lista os últimos blocos de uma corrida (-db ou -peer)
  ranking   placar por minerador de uma corrida (-db ou -peer)

Exemplo — dois nodes de verdade na mesma máquina:
  node run -profile devnet -datadir ~/.panda/n1 -listen :9551 -rpc 127.0.0.1:8551
  node run -profile devnet -datadir ~/.panda/n2 -listen :9552 -rpc 127.0.0.1:8552 -peers 127.0.0.1:9551
  node info    -rpc 127.0.0.1:8552      # alturas convergem => sync ok
  node balance -rpc 127.0.0.1:8551      # cresce conforme coinbases maturam
  node send    -rpc 127.0.0.1:8551 -to P... -amount 1.5

Configuração por arquivo (menos flags repetidos):
  copie panda.conf.example para panda.conf no diretório onde roda o node —
  powdemo/blocks/ranking o encontram sozinhos (ou use -config caminho).
  As chaves são os nomes dos flags (name=David, db=david.db, listen=:9551,
  peer=..., spacing=1m, zeros=10, ...); flag na linha de comando vence.
  Com o arquivo no lugar, "node powdemo" e "node blocks" bastam.

Exemplos:
  node powdemo                                  # 3 blocos, perfil devnet (64 MiB/hash)
  node powdemo -blocks 0 -spacing 10m           # madrugada estilo Bitcoin: blocos de ~10min
  node powdemo -zeros 9 -workers 2              # mais difícil, 2 cores (+64 MiB de RAM)
  node powdemo -profile test                    # Argon2 de 1 MiB: veja a diferença de H/s

  # corrida de mineradores NA MESMA MÁQUINA (um terminal por minerador, mesmo -db):
  node powdemo -db mineracao.db -name Alice -blocks 0 -spacing 1m -zeros 10
  node powdemo -db mineracao.db -name Bob   -blocks 0
  node blocks  -db mineracao.db -last 10
  node ranking -db mineracao.db

  # corrida entre DOIS MACS na mesma rede (ver SINCRONIZACAO.md):
  # no Mac A (abre a corrida e expõe pra rede):
  node powdemo -db alice.db -name Alice -listen :9551 -blocks 0 -spacing 1m -zeros 10
  # no Mac B (SEMPRE com -db próprio — ele sincroniza uma cópia completa,
  # não fica refém do Mac A continuar ligado; descubra o IP do Mac A com
  # "ipconfig getifaddr en0"):
  node powdemo -db bob.db -peer 192.168.1.10:9551 -name Bob -blocks 0
  node blocks  -db bob.db -last 10

  # Bob agora tem uma cópia completa e pode servir um TERCEIRO Mac, mesmo
  # que o Mac A da Alice já tenha sido desligado:
  node powdemo -db bob.db -peer 192.168.1.10:9551 -listen :9552 -name Bob -blocks 0
  node powdemo -db carol.db -peer 192.168.1.11:9552 -name Carol -blocks 0
`, version)
}

// helpFor delega para o -h do próprio comando — o flag package imprime os
// flags e sai; wallet não usa um FlagSet único, então o texto vem daqui.
func helpFor(cmd string) {
	switch cmd {
	case "run":
		runRun([]string{"-h"})
	case "info":
		runInfo([]string{"-h"})
	case "balance":
		runBalance([]string{"-h"})
	case "send":
		runSend([]string{"-h"})
	case "block":
		runBlock([]string{"-h"})
	case "powdemo":
		runPowDemo([]string{"-h"})
	case "blocks":
		runBlocks([]string{"-h"})
	case "ranking":
		runRanking([]string{"-h"})
	case "genesis":
		runGenesis([]string{"-h"})
	case "wallet":
		fmt.Print(`uso: panda-node wallet <subcomando> [-file wallet.json | -datadir DIR]
  new       gera chave nova + 12 palavras de backup (nunca sobrescreve)
  restore   recupera a carteira: wallet restore palavra1 ... palavra12
  words     reexibe as 12 palavras da wallet (só para os SEUS olhos)
  address   reexibe o endereço (alias: show)
`)
	default:
		fmt.Fprintf(os.Stderr, "comando desconhecido: %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
}
