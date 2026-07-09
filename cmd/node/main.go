// O binário zhu é o full node da rede Zhu — uma blockchain proof-of-work
// memory-hard (Argon2id) desenhada para uma obsessão só: um node em cada
// casa. Roda como arquivo estático em qualquer máquina, valida tudo por
// conta própria e minera por padrão (1 worker, ~64 MiB). Além do node, o
// binário guarda a bancada didática `powdemo` — onde se aprende PoW de
// verdade — e os comandos de consulta `blocks`/`ranking`. Os subcomandos do
// node de verdade (run, wallet, send, ...) e a arquitetura estão em
// internal/node/SPEC.md.
package main

import (
	"fmt"
	"os"
)

// version é injetada pelos scripts de build (scripts/build-*.sh) via
// -ldflags "-X main.version=...", lida do build.conf do desenvolvedor.
var version = "dev"

// banner é a cara do Zhu no terminal — aparece no help e na subida do run.
// O mascote é calmo, econômico e teimoso: fala pouco, e sobre a rede.
const banner = `
       .__                           __                       __    
_______|  |__  __ __    ____   _____/  |___  _  _____________|  | __
\___   /  |  \|  |  \  /    \_/ __ \   __\ \/ \/ /  _ \_  __ \  |/ /
 /    /|   Y  \  |  / |   |  \  ___/|  |  \     (  <_> )  | \/    < 
/_____ \___|  /____/  |___|  /\___  >__|   \/\_/ \____/|__|  |__|_ \
      \/    \/             \/     \/                              \/   竹 ₍ᵔ.˛.ᵔ₎
mais um broto. para a rede.
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
	case "docs":
		runDocs(os.Args[2:])
	case "zen":
		runZen()
	case "version", "-v", "--version":
		fmt.Printf("zhu %s\n", version)
	case "help", "-h", "--help":
		if len(os.Args) > 2 {
			helpFor(os.Args[2])
		} else {
			usage()
		}
	default:
		fmt.Fprintf(os.Stderr, "Zhu não conhece esse comando: %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(banner)
	fmt.Printf(`
 zhu %s — o full node da rede Zhu

 Rodar um node é o ato que descentraliza a rede. Este binário é a sua
 lanterna: liga, valida tudo sozinho e acende uma luz ao lado das outras.

USO
  zhu <comando> [flags]
  zhu help <comando>       flags de um comando (idem: <comando> -h)

O node de verdade:
  run       acende a lanterna: chain validada (bbolt), mempool, rede p2p e
            mineração LIGADA por padrão (1 worker; -mine=false desliga).
            A coinbase paga a wallet do datadir (criada no primeiro run).
  info      altura/tip/peers/mempool/hashrate do node em execução (via RPC)
  balance   saldo de um endereço (default: a wallet do node)
  send      envia ZHU: zhu send -to P... -amount 1.5
  block     explora um bloco por dentro: zhu block 42 | zhu block <hash>
            (vazio = a ponta) — coinbase, transações, valores e destinos
  wallet    new: gera chave + 12 palavras de backup (BIP39); restore: recupera
            a carteira só com as palavras; words: reexibe as palavras;
            address: reexibe o endereço
  genesis   (dev) minera o bloco 0 de um perfil
  docs      abre a documentação web da Zhu no navegador (localhost)
  version   versão do binário (definida no build.conf de quem compilou)
  zen       os princípios da rede, uma frase por vez

Bancada didática (onde se aprende PoW de verdade):
  powdemo   corrida de mineradores com PoW real e recompensa simulada
  blocks    lista os últimos blocos de uma corrida (-db ou -peer)
  ranking   placar por minerador de uma corrida (-db ou -peer)

Exemplo — dois nodes de verdade na mesma máquina:
  zhu run -profile devnet -datadir ~/.zhu/n1 -listen :9551 -rpc 127.0.0.1:8551
  zhu run -profile devnet -datadir ~/.zhu/n2 -listen :9552 -rpc 127.0.0.1:8552 -peers 127.0.0.1:9551
  zhu info    -rpc 127.0.0.1:8552      # alturas convergem => sync ok
  zhu balance -rpc 127.0.0.1:8551      # cresce conforme coinbases maturam
  zhu send    -rpc 127.0.0.1:8551 -to P... -amount 1.5

Configuração por arquivo (menos flags repetidos):
  copie zhu.conf.example para zhu.conf no diretório onde roda o node —
  powdemo/blocks/ranking o encontram sozinhos (ou use -config caminho).
  As chaves são os nomes dos flags (name=David, db=david.db, listen=:9551,
  peer=..., spacing=1m, zeros=10, ...); flag na linha de comando vence.
  Com o arquivo no lugar, "zhu powdemo" e "zhu blocks" bastam.

Exemplos:
  zhu powdemo                                  # 3 blocos, perfil devnet (64 MiB/hash)
  zhu powdemo -blocks 0 -spacing 10m           # madrugada estilo Bitcoin: blocos de ~10min
  zhu powdemo -zeros 9 -workers 2              # mais difícil, 2 cores (+64 MiB de RAM)
  zhu powdemo -profile test                    # Argon2 de 1 MiB: veja a diferença de H/s

  # corrida de mineradores NA MESMA MÁQUINA (um terminal por minerador, mesmo -db):
  zhu powdemo -db mineracao.db -name Alice -blocks 0 -spacing 1m -zeros 10
  zhu powdemo -db mineracao.db -name Bob   -blocks 0
  zhu blocks  -db mineracao.db -last 10
  zhu ranking -db mineracao.db

  # corrida entre DUAS MÁQUINAS na mesma rede (ver SINCRONIZACAO.md):
  # na máquina A (abre a corrida e expõe pra rede):
  zhu powdemo -db alice.db -name Alice -listen :9551 -blocks 0 -spacing 1m -zeros 10
  # na máquina B (SEMPRE com -db próprio — ela sincroniza uma cópia completa,
  # não fica refém da máquina A continuar ligada; descubra o IP da A com
  # "ipconfig getifaddr en0"):
  zhu powdemo -db bob.db -peer 192.168.1.10:9551 -name Bob -blocks 0
  zhu blocks  -db bob.db -last 10

  # Bob agora tem uma cópia completa e pode servir uma TERCEIRA máquina, mesmo
  # que a máquina A da Alice já tenha sido desligada:
  zhu powdemo -db bob.db -peer 192.168.1.10:9551 -listen :9552 -name Bob -blocks 0
  zhu powdemo -db carol.db -peer 192.168.1.11:9552 -name Carol -blocks 0
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
	case "docs":
		runDocs([]string{"-h"})
	case "wallet":
		fmt.Print(`uso: zhu wallet <subcomando> [-file wallet.json | -datadir DIR]
  new       gera chave nova + 12 palavras de backup (nunca sobrescreve)
  restore   recupera a carteira: zhu wallet restore palavra1 ... palavra12
  words     reexibe as 12 palavras da wallet (só para os SEUS olhos)
  address   reexibe o endereço (alias: show)
`)
	default:
		fmt.Fprintf(os.Stderr, "Zhu não conhece esse comando: %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
}

// runZen é o easter egg da marca: o Zhu cospe os princípios da rede, um por
// vez. Sereno e teimoso num assunto só — descentralização.
func runZen() {
	zen := []string{
		"Rodar um node é o ato que descentraliza a rede.",
		"A segurança vem da quantidade de participantes, não da potência de cada um.",
		"O gargalo é RAM comum, não hardware especializado. Sem corrida armamentista.",
		"Ninguém concede sua luz, ninguém a apaga.",
		"Claro antes de esperto. Coletivo, nunca messiânico.",
		"Nunca uma promessa de preço — só tecnologia que qualquer casa consegue rodar.",
		"Se cada vizinho acender a sua lanterna, nenhuma escuridão apaga a rede toda.",
    "Descentralizados e unidos!",
	}
	for _, line := range zen {
		fmt.Printf("%s\n", line)
	}
}
