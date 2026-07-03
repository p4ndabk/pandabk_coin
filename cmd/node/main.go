// O binário node é o full node PANDA. Nesta fase (M1.5) ele expõe a bancada
// de mineração powdemo — proof of work real (Argon2id) com retarget,
// recompensa simulada e, com -db, uma corrida entre vários mineradores na
// mesma máquina usando um SQLite compartilhado como "rede" — além dos
// comandos de consulta blocks e ranking. Os subcomandos definitivos
// (run, wallet, send, ...) chegam nos milestones seguintes; ver
// internal/node/SPEC.md.
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
            retarget de dificuldade e recompensa simulada; com -db, vários
            mineradores competem pela mesma chain (SQLite compartilhado)
  blocks    lista os últimos blocos de um banco de demo (-db)
  ranking   placar por minerador de um banco de demo (-db)

Exemplos:
  node powdemo                                  # 3 blocos, perfil devnet (64 MiB/hash)
  node powdemo -blocks 0 -spacing 10m           # madrugada estilo Bitcoin: blocos de ~10min
  node powdemo -zeros 9 -workers 2              # mais difícil, 2 cores (+64 MiB de RAM)
  node powdemo -profile test                    # Argon2 de 1 MiB: veja a diferença de H/s

  # corrida de mineradores (um por terminal, mesmo -db):
  node powdemo -db mineracao.db -name Alice -blocks 0 -spacing 1m -zeros 10
  node powdemo -db mineracao.db -name Bob   -blocks 0
  node blocks  -db mineracao.db -last 10
  node ranking -db mineracao.db
`)
}
