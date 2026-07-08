// O binário desktop é a janela do node Zhu: o mesmo que a CLI faz no
// terminal (info/balance/send), numa interface nativa (Fyne — renderização
// em Go, sem Electron). Híbrido: usa um node já em execução via RPC local
// ou, se não houver, embute o node no próprio processo. Ver SPEC.md.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"zhu/internal/node"
)

// version é injetada por scripts/build-desktop.sh (-X main.version=...).
var version = "dev"

func main() {
	fs := flag.NewFlagSet("zhu-desktop", flag.ExitOnError)
	cfg, peersCSV := node.RegisterFlags(fs)
	peerSingle := fs.String("peer", "", "alias de -peers para um único peer")
	configPath := fs.String("config", "", "arquivo de configuração chave=valor (default: zhu.conf, se existir)")
	fs.Parse(os.Args[1:])

	// O desktop ancora o zhu.conf num caminho fixo (~/.zhu/zhu.conf,
	// salvo -config ou um zhu.conf no diretório atual). Sem o arquivo =
	// primeira vez: tela de pré-configuração antes de ligar qualquer coisa.
	confPath := desktopConfigPath(*configPath)
	firstRun := false
	if _, err := os.Stat(confPath); err == nil {
		if _, err := node.ApplyConfigFile(fs, confPath); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(2)
		}
	} else {
		firstRun = true
	}
	node.FinishFlags(cfg, *peersCSV)
	if *peerSingle != "" {
		cfg.Peers = append(cfg.Peers, *peerSingle)
	}

	// Captura o log ANTES de subir qualquer coisa: a aba Atividade mostra a
	// vida do node (peers, blocos, retarget) e o terminal continua vendo tudo.
	logs := newLogStore(500)
	log.SetOutput(io.MultiWriter(os.Stderr, logs))

	a := app.NewWithID("coin.zhu.desktop")
	a.Settings().SetTheme(zhuTheme{})
	w := a.NewWindow("Zhu")
	w.Resize(fyne.NewSize(960, 660))
	w.CenterOnScreen()

	u := &ui{app: a, win: w, cfg: cfg, confPath: confPath, logs: logs, version: version}

	if firstRun {
		w.SetContent(u.setupScreen())
	} else {
		u.startAndBuild()
	}
	w.ShowAndRun()
}
