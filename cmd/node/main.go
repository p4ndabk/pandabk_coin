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

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
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
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "subcomando desconhecido: %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`node — full node da PANDA Coin (em construção)

Subcomandos disponíveis:
  powdemo   minera blocos de demonstração com o proof of work real (Argon2id),
            retarget de dificuldade e recompensa simulada; com -db/-peer,
            vários mineradores competem pela mesma chain
  blocks    lista os últimos blocos de uma corrida (-db ou -peer)
  ranking   placar por minerador de uma corrida (-db ou -peer)
  wallet    new: gera sua chave/endereço (wallet.json, 0600); show: reexibe

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
`)
}
