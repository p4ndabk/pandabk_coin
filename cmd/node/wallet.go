package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"zhu/internal/wallet"
)

// runWallet expõe a wallet na CLI: `wallet new` cria a chave com backup de
// 12 palavras (BIP39, nunca sobrescreve), `wallet restore` recupera a
// carteira só com as palavras, e `wallet show` reexibe o endereço. Os
// comandos que PRECISAM da chain (balance, send) falam com o node via RPC.
func runWallet(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, `uso: zhu wallet new|show|restore [-file wallet.json]
       zhu wallet restore -file wallet.json palavra1 palavra2 ... palavra12`)
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
		w, phrase, err := wallet.NewWithMnemonic(*file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Printf("🔑 wallet nova em %s (permissão 0600)\n", *file)
		fmt.Printf("   endereço: %s\n\n", w.Address())
		fmt.Println("📝 SUAS 12 PALAVRAS — anote NUM PAPEL, nesta ordem, e guarde bem:")
		fmt.Println()
		printMnemonic(phrase)
		fmt.Println()
		fmt.Println("⚠️  elas são exibidas UMA única vez e recuperam sua carteira em qualquer")
		fmt.Println("   máquina (zhu wallet restore). Quem lê as palavras leva os fundos;")
		fmt.Println("   quem as perde junto com o arquivo perde tudo — não existe suporte.")
	case "restore":
		phrase := strings.Join(fs.Args(), " ")
		if strings.TrimSpace(phrase) == "" {
			fmt.Fprintln(os.Stderr, "uso: zhu wallet restore [-file wallet.json] palavra1 ... palavra12")
			os.Exit(2)
		}
		w, err := wallet.Restore(*file, phrase)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Printf("🔑 wallet recuperada em %s\n", *file)
		fmt.Printf("   endereço: %s\n", w.Address())
		fmt.Println("   (o saldo aparece quando um node com a chain sincronizada consultar este endereço)")
	case "show", "address":
		w, err := wallet.Load(*file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Printf("endereço: %s\n", w.Address())
	case "words":
		w, err := wallet.Load(*file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		if w.Mnemonic() == "" {
			fmt.Println("esta wallet nasceu de chave aleatória (antes do backup por frase) — não há 12 palavras.")
			fmt.Println("para ter uma com palavras: `wallet new` em outro arquivo e transfira os fundos com `send`.")
			return
		}
		fmt.Println("📝 as 12 palavras desta wallet (só para os SEUS olhos):")
		fmt.Println()
		printMnemonic(w.Mnemonic())
	default:
		fmt.Fprintf(os.Stderr, "subcomando de wallet desconhecido: %q (use new, restore, words ou address)\n", sub)
		os.Exit(2)
	}
}

// printMnemonic imprime as palavras numeradas em 3 colunas — fácil de
// conferir no papel.
func printMnemonic(phrase string) {
	words := strings.Fields(phrase)
	for row := 0; row < 4; row++ {
		for col := 0; col < 3; col++ {
			i := row + col*4
			fmt.Printf("   %2d. %-12s", i+1, words[i])
		}
		fmt.Println()
	}
}
