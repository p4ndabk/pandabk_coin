package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"pandabk_coin/internal/wallet"
)

// runWallet expõe a wallet do M3 na CLI: `wallet new` cria a chave (nunca
// sobrescreve) e `wallet show` reexibe o endereço. Os comandos que PRECISAM
// da chain (balance, send) chegam no M5 via RPC do node.
func runWallet(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "uso: node wallet new|show [-file wallet.json]")
		os.Exit(2)
	}
	sub := args[0]
	fs := flag.NewFlagSet("wallet "+sub, flag.ExitOnError)
	file := fs.String("file", "wallet.json", "arquivo da wallet")
	datadir := fs.String("datadir", "", "usa a wallet do datadir de um node (<datadir>/wallet.json)")
	fs.Parse(args[1:])
	if *datadir != "" {
		*file = filepath.Join(*datadir, "wallet.json")
	}

	switch sub {
	case "new":
		w, err := wallet.New(*file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Printf("🔑 wallet nova em %s (permissão 0600)\n", *file)
		fmt.Printf("   endereço: %s\n\n", w.Address())
		fmt.Println("⚠️  faça backup deste arquivo AGORA: quem perde a chave perde os fundos,")
		fmt.Println("   e não existe recuperação — é assim que blockchain funciona.")
	case "show", "address":
		w, err := wallet.Load(*file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Printf("endereço: %s\n", w.Address())
	default:
		fmt.Fprintf(os.Stderr, "subcomando de wallet desconhecido: %q (use new ou address)\n", sub)
		os.Exit(2)
	}
}
