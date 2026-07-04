package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pandabk_coin/internal/node"
)

// runRun é o `node run`: o full node de verdade (M5) — chain validada em
// bbolt, mempool, p2p e miner LIGADO por padrão (1 worker), com RPC local
// para os comandos info/balance/send.
func runRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	cfg, peersCSV := node.RegisterFlags(fs)
	peerSingle := fs.String("peer", "", "alias de -peers para um único peer (mesma chave do panda.conf da demo)")
	configPath := fs.String("config", "", "arquivo de configuração chave=valor (default: panda.conf, se existir)")
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
	fmt.Printf(`🐼 PANDA node no ar

   perfil       %s
   datadir      %s
   p2p          %s
   rpc          %s   (info/balance/send falam aqui)
   mineração    %s

Ctrl+C para encerrar com segurança.

`, cfg.Profile, cfg.DataDir, listen, n.RPCAddr(), mining)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	fmt.Println("\nencerrando (p2p → miner → banco)...")
	if err := n.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "encerrando: %v\n", err)
		os.Exit(1)
	}
}

// ── cliente RPC dos subcomandos ─────────────────────────────────────────────

func rpcClient(addr, method string, params any, out any) error {
	req := struct {
		Method string `json:"method"`
		Params any    `json:"params,omitempty"`
	}{Method: method, Params: params}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post("http://"+addr+"/rpc", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("não consegui falar com o node em %s — ele está rodando? (%v)", addr, err)
	}
	defer resp.Body.Close()
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("resposta inválida do node: %w", err)
	}
	if envelope.Error != nil {
		return errors.New(envelope.Error.Message)
	}
	if out != nil {
		return json.Unmarshal(envelope.Result, out)
	}
	return nil
}

func rpcFlag(fs *flag.FlagSet) *string {
	def := os.Getenv("NODE_RPC")
	if def == "" {
		def = "127.0.0.1:8555"
	}
	return fs.String("rpc", def, "endereço RPC do node em execução")
}

func runInfo(args []string) {
	fs := flag.NewFlagSet("info", flag.ExitOnError)
	rpc := rpcFlag(fs)
	configPath := fs.String("config", "", "arquivo de configuração (default: panda.conf, se existir)")
	fs.Parse(args)
	applyConfig(fs, *configPath)

	var info struct {
		Profile  string  `json:"profile"`
		Height   uint64  `json:"height"`
		Tip      string  `json:"tip"`
		Peers    int     `json:"peers"`
		Mempool  int     `json:"mempool"`
		Mining   bool    `json:"mining"`
		HashRate float64 `json:"hashrate"`
		Address  string  `json:"address"`
	}
	if err := rpcClient(*rpc, "getinfo", nil, &info); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	fmt.Printf("perfil     %s\naltura     %d\ntip        %s\npeers      %d\nmempool    %d tx(s)\n",
		info.Profile, info.Height, info.Tip, info.Peers, info.Mempool)
	if info.Mining {
		fmt.Printf("minerando  sim (%.1f H/s)\n", info.HashRate)
	} else {
		fmt.Println("minerando  não")
	}
	if info.Address != "" {
		fmt.Printf("endereço   %s\n", info.Address)
	}
}

func runBalance(args []string) {
	fs := flag.NewFlagSet("balance", flag.ExitOnError)
	rpc := rpcFlag(fs)
	address := fs.String("address", "", "endereço a consultar (default: a wallet do node)")
	configPath := fs.String("config", "", "arquivo de configuração (default: panda.conf, se existir)")
	fs.Parse(args)
	applyConfig(fs, *configPath)

	var bal struct {
		Address        string `json:"address"`
		BalancePanda   string `json:"balance_panda"`
		SpendablePanda string `json:"spendable_panda"`
		UTXOs          int    `json:"utxos"`
	}
	params := map[string]string{}
	if *address != "" {
		params["address"] = *address
	}
	if err := rpcClient(*rpc, "getbalance", params, &bal); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	fmt.Printf("endereço   %s\nsaldo      %s PANDA (%d UTXOs)\ngastável   %s PANDA (coinbases maduras, nada pendente)\n",
		bal.Address, bal.BalancePanda, bal.UTXOs, bal.SpendablePanda)
}

func runSend(args []string) {
	fs := flag.NewFlagSet("send", flag.ExitOnError)
	rpc := rpcFlag(fs)
	to := fs.String("to", "", "endereço de destino (P...)")
	amount := fs.String("amount", "", "valor em PANDA, ex.: 1.5")
	feeRate := fs.Uint64("fee-rate", 0, "taxa em subunidades/byte (0 = default do node)")
	configPath := fs.String("config", "", "arquivo de configuração (default: panda.conf, se existir)")
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
