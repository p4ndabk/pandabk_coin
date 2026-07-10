package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

// runPeers é o `zhu peers`: os vizinhos do node em execução (via RPC) — quem
// está conectado agora (direção, altura declarada, tempo de conexão) e os
// endereços que o node conhece por peer exchange, mesmo desconectados.
func runPeers(args []string) {
	fs := flag.NewFlagSet("peers", flag.ExitOnError)
	rpc := rpcFlag(fs)
	known := fs.Bool("known", false, "listar também o address book (endereços conhecidos, mesmo desconectados)")
	configPath := fs.String("config", "", "arquivo de configuração (default: zhu.conf, se existir)")
	fs.Parse(args)
	applyConfig(fs, *configPath)

	var res struct {
		Peers []struct {
			Addr          string `json:"addr"`
			Direction     string `json:"direction"`
			ListenAddr    string `json:"listen_addr"`
			Height        uint64 `json:"height"`
			ConnectedSecs int64  `json:"connected_seconds"`
		} `json:"peers"`
		KnownAddrs []string `json:"known_addrs"`
	}
	if err := rpcClient(*rpc, "getpeers", nil, &res); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	if len(res.Peers) == 0 {
		fmt.Println("nenhum peer conectado — a lanterna está acesa, mas sozinha.")
		fmt.Println("aponte vizinhos com -peers IP:PORTA no run (ou peers= no zhu.conf).")
	} else {
		fmt.Printf("%d peer(s) conectado(s):\n\n", len(res.Peers))
		fmt.Printf("  %-28s %-8s %-10s %-10s %s\n", "ENDEREÇO", "DIREÇÃO", "ALTURA*", "HÁ", "ANUNCIA")
		for _, p := range res.Peers {
			dir := "entrada"
			if p.Direction == "out" {
				dir = "saída"
			}
			ann := p.ListenAddr
			if ann == "" {
				ann = "—"
			}
			fmt.Printf("  %-28s %-8s %-10d %-10s %s\n",
				p.Addr, dir, p.Height, formatConnected(p.ConnectedSecs), ann)
		}
		fmt.Println("\n  * altura declarada no handshake (quando o peer conectou)")
	}

	if *known {
		fmt.Printf("\naddress book — %d endereço(s) conhecido(s) por peer exchange:\n", len(res.KnownAddrs))
		for _, a := range res.KnownAddrs {
			fmt.Printf("  %s\n", a)
		}
	} else if len(res.KnownAddrs) > 0 {
		fmt.Printf("\naddress book: %d endereço(s) conhecido(s) — veja com zhu peers -known\n", len(res.KnownAddrs))
	}
}

// formatConnected mostra o tempo de conexão sem ruído: 42s, 5m2s, 3h07m.
func formatConnected(secs int64) string {
	d := time.Duration(secs) * time.Second
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", secs)
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", secs/60, secs%60)
	default:
		return fmt.Sprintf("%dh%02dm", secs/3600, (secs%3600)/60)
	}
}
