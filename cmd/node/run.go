package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"zhu/internal/node"
	"zhu/internal/rpcclient"
)

// runRun é o `node run`: o full node de verdade (M5) — chain validada em
// bbolt, mempool, p2p e miner LIGADO por padrão (1 worker), com RPC local
// para os comandos info/balance/send.
func runRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	cfg, peersCSV := node.RegisterFlags(fs)
	peerSingle := fs.String("peer", "", "alias de -peers para um único peer (mesma chave do zhu.conf da demo)")
	configPath := fs.String("config", "", "arquivo de configuração chave=valor (default: zhu.conf, se existir)")
	fs.Parse(args)
	applyConfig(fs, *configPath)
	node.FinishFlags(cfg, *peersCSV)
	if *peerSingle != "" {
		cfg.Peers = append(cfg.Peers, *peerSingle)
	}

	n, err := node.New(*cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if err := n.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	mining := "desligada (-mine=false)"
	if cfg.Mine {
		mining = fmt.Sprintf("LIGADA — %d worker(s), 1 core e ~64 MiB cada", cfg.Miners)
	}
	listen := n.P2PAddr()
	if listen == "" {
		listen = "só saída (funciona atrás de NAT)"
	}
	p, _ := cfg.Params()
	height, diff, subsidy := n.ChainStatus()
	fmt.Print(banner)
	fmt.Printf(`
 🏮 Lanterna acesa. — Zhu no ar (versão %s)

   perfil       %s
   datadir      %s
   p2p          %s
   rpc          %s   (info/balance/send falam aqui)
   mineração    %s
   consenso     1 bloco a cada %s | retarget a cada %d blocos | halving a cada %d
   recompensa   %s ZHU pelo próximo bloco
   chain        altura %d, dificuldade %.2f

Ctrl+C para encerrar com segurança.

`, version, cfg.Profile, cfg.DataDir, listen, n.RPCAddr(), mining,
		p.TargetSpacing, p.RetargetInterval, p.HalvingInterval,
		node.FormatZhu(subsidy), height, diff)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	fmt.Println("\n🏮 Apagando a lanterna com cuidado (p2p → miner → banco)...")
	if err := n.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "encerrando: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Até a próxima, vizinho.")
}

// ── cliente RPC dos subcomandos (internal/rpcclient, compartilhado com o
// app de desktop) ───────────────────────────────────────────────────────────

func rpcClient(addr, method string, params any, out any) error {
	return rpcclient.Call(addr, method, params, out)
}

func rpcFlag(fs *flag.FlagSet) *string {
	def := os.Getenv("ZHU_RPC")
	if def == "" {
		def = "127.0.0.1:8555"
	}
	return fs.String("rpc", def, "endereço RPC do node em execução")
}

func runInfo(args []string) {
	fs := flag.NewFlagSet("info", flag.ExitOnError)
	rpc := rpcFlag(fs)
	configPath := fs.String("config", "", "arquivo de configuração (default: zhu.conf, se existir)")
	fs.Parse(args)
	applyConfig(fs, *configPath)

	var info struct {
		Profile     string  `json:"profile"`
		Height      uint64  `json:"height"`
		Tip         string  `json:"tip"`
		Bits        string  `json:"bits"`
		Difficulty  float64 `json:"difficulty"`
		SpacingSecs int64   `json:"target_spacing_seconds"`
		RewardZhu   string  `json:"reward_zhu"`
		NextHalving uint64  `json:"next_halving"`
		Peers       int     `json:"peers"`
		Mempool     int     `json:"mempool"`
		Mining      bool    `json:"mining"`
		HashRate    float64 `json:"hashrate"`
		Address     string  `json:"address"`
	}
	if err := rpcClient(*rpc, "getinfo", nil, &info); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	fmt.Printf("perfil       %s\naltura       %d\ntip          %s\n", info.Profile, info.Height, info.Tip)
	fmt.Printf("dificuldade  %.2f (bits %s)\n", info.Difficulty, info.Bits)
	fmt.Printf("alvo         1 bloco a cada %ds\n", info.SpacingSecs)

	var st struct {
		AvgBlockSecs   float64 `json:"avg_block_seconds"`
		AvgWindow      int     `json:"avg_window"`
		BlocksToRetgt  uint64  `json:"blocks_to_retarget"`
		RetargetFactor float64 `json:"retarget_factor"`
		BlocksToHalve  uint64  `json:"blocks_to_halving"`
		RewardZhu      string  `json:"reward_zhu"`
		NextReward     string  `json:"next_reward_zhu"`
	}
	if err := rpcClient(*rpc, "getstats", nil, &st); err == nil {
		if st.AvgWindow > 0 {
			fmt.Printf("tempo médio  %.0fs por bloco (últimos %d)\n", st.AvgBlockSecs, st.AvgWindow)
		}
		retarget := fmt.Sprintf("retarget     em %d bloco(s)", st.BlocksToRetgt)
		switch {
		case st.RetargetFactor > 1.02:
			retarget += fmt.Sprintf(" — dificuldade deve SUBIR ~×%.2f", st.RetargetFactor)
		case st.RetargetFactor > 0 && st.RetargetFactor < 0.98:
			retarget += fmt.Sprintf(" — dificuldade deve CAIR ~×%.2f", st.RetargetFactor)
		case st.RetargetFactor > 0:
			retarget += " — ritmo no alvo, dificuldade estável"
		}
		fmt.Println(retarget)
		fmt.Printf("recompensa   %s ZHU — halving em %d bloco(s) (%s → %s)\n",
			st.RewardZhu, st.BlocksToHalve, st.RewardZhu, st.NextReward)
	} else {
		fmt.Printf("recompensa   %s ZHU (próximo halving no bloco %d)\n", info.RewardZhu, info.NextHalving)
	}
	fmt.Printf("peers        %d\nmempool      %d tx(s)\n", info.Peers, info.Mempool)
	if info.Mining {
		fmt.Printf("minerando    sim (%.1f H/s)\n", info.HashRate)
	} else {
		fmt.Println("minerando    não")
	}
	if info.Address != "" {
		fmt.Printf("endereço     %s\n", info.Address)
	}
}

func runBalance(args []string) {
	fs := flag.NewFlagSet("balance", flag.ExitOnError)
	rpc := rpcFlag(fs)
	address := fs.String("address", "", "endereço a consultar (default: a wallet do node)")
	configPath := fs.String("config", "", "arquivo de configuração (default: zhu.conf, se existir)")
	fs.Parse(args)
	applyConfig(fs, *configPath)

	var bal struct {
		Address      string `json:"address"`
		BalanceZhu   string `json:"balance_zhu"`
		SpendableZhu string `json:"spendable_zhu"`
		UTXOs        int    `json:"utxos"`
	}
	params := map[string]string{}
	if *address != "" {
		params["address"] = *address
	}
	if err := rpcClient(*rpc, "getbalance", params, &bal); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	fmt.Printf("endereço   %s\nsaldo      %s ZHU (%d UTXOs)\ngastável   %s ZHU (coinbases maduras, nada pendente)\n",
		bal.Address, bal.BalanceZhu, bal.UTXOs, bal.SpendableZhu)
}

func runSend(args []string) {
	fs := flag.NewFlagSet("send", flag.ExitOnError)
	rpc := rpcFlag(fs)
	to := fs.String("to", "", "endereço de destino (P...)")
	amount := fs.String("amount", "", "valor em ZHU, ex.: 1.5")
	feeRate := fs.Uint64("fee-rate", 0, "taxa em subunidades/byte (0 = default do node)")
	configPath := fs.String("config", "", "arquivo de configuração (default: zhu.conf, se existir)")
	fs.Parse(args)
	applyConfig(fs, *configPath)
	if *to == "" || *amount == "" {
		fmt.Fprintln(os.Stderr, "uso: node send -to P... -amount 1.5 [-rpc 127.0.0.1:8555]")
		os.Exit(2)
	}

	req := map[string]any{"to": *to, "amount": *amount}
	if *feeRate > 0 {
		req["fee_rate"] = *feeRate
	}
	var res map[string]string
	if err := rpcClient(*rpc, "sendtoaddress", req, &res); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	fmt.Printf("📤 enviado! txid %s\n", res["txid"])
	fmt.Println("   a transação está no mempool; confirma quando um minerador incluí-la num bloco.")
}
