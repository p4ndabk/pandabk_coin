package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"
)

// runBlock é o explorador de bolso: `node block 42` (ou um hash, ou nada =
// a ponta) mostra o bloco por dentro — quem minerou (coinbase), quais
// transações carrega e para onde o dinheiro foi.
func runBlock(args []string) {
	fs := flag.NewFlagSet("block", flag.ExitOnError)
	rpc := rpcFlag(fs)
	configPath := fs.String("config", "", "arquivo de configuração (default: panda.conf, se existir)")
	fs.Parse(args)
	applyConfig(fs, *configPath)

	params := map[string]any{}
	if arg := fs.Arg(0); arg != "" {
		if h, err := strconv.ParseUint(arg, 10, 64); err == nil {
			params["height"] = h
		} else if len(arg) == 64 {
			params["hash"] = arg
		} else {
			fmt.Fprintf(os.Stderr, "uso: node block [altura | hash de 64 hex]  (vazio = ponta atual)\n")
			os.Exit(2)
		}
	}

	var b struct {
		Height        uint64  `json:"height"`
		Hash          string  `json:"hash"`
		Prev          string  `json:"prev"`
		Time          int64   `json:"time"`
		Bits          string  `json:"bits"`
		Difficulty    float64 `json:"difficulty"`
		Nonce         uint64  `json:"nonce"`
		Size          int     `json:"size"`
		Confirmations uint64  `json:"confirmations"`
		Txs           []struct {
			TxID     string `json:"txid"`
			Coinbase bool   `json:"coinbase"`
			Ins      []struct {
				TxID  string `json:"txid"`
				Index uint32 `json:"index"`
			} `json:"ins"`
			Outs []struct {
				ValuePanda string `json:"value_panda"`
				Address    string `json:"address"`
			} `json:"outs"`
		} `json:"txs"`
	}
	if err := rpcClient(*rpc, "getblock", params, &b); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	fmt.Printf("bloco        %d  (%d confirmação(ões))\n", b.Height, b.Confirmations)
	fmt.Printf("hash         %s\n", b.Hash)
	fmt.Printf("anterior     %s\n", b.Prev)
	fmt.Printf("quando       %s\n", time.Unix(b.Time, 0).Format("2006-01-02 15:04:05"))
	fmt.Printf("dificuldade  %.2f (bits %s)  nonce %d\n", b.Difficulty, b.Bits, b.Nonce)
	fmt.Printf("tamanho      %d bytes\ntransações   %d\n", b.Size, len(b.Txs))
	for i, tx := range b.Txs {
		fmt.Println()
		if tx.Coinbase {
			fmt.Printf("tx %d  %s  (coinbase — a recompensa deste bloco)\n", i+1, tx.TxID[:16])
		} else {
			fmt.Printf("tx %d  %s\n", i+1, tx.TxID[:16])
			for _, in := range tx.Ins {
				fmt.Printf("   gasta  %s:%d\n", in.TxID[:16], in.Index)
			}
		}
		for _, out := range tx.Outs {
			fmt.Printf("   →  %s  %s PANDA\n", out.Address, out.ValuePanda)
		}
	}
}
