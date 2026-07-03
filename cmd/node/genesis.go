package main

import (
	"flag"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"pandabk_coin/internal/chain"
	"pandabk_coin/internal/params"
	"pandabk_coin/internal/pow"
)

// runGenesis é o comando dev do M2: minera o bloco 0 de um perfil (procura o
// nonce cujo Argon2id fica abaixo do target) e imprime Nonce e Hash prontos
// para colar em internal/params/genesis.go. Roda uma única vez por perfil —
// depois do hardcode, todo nó reconstrói o gênesis deterministicamente e só
// confere o hash.
func runGenesis(args []string) {
	fs := flag.NewFlagSet("genesis", flag.ExitOnError)
	profile := fs.String("profile", "all", "perfil a minerar: devnet, test ou all")
	fs.Parse(args)

	profiles := []params.Params{}
	switch *profile {
	case "devnet":
		profiles = append(profiles, params.DevNet())
	case "test":
		profiles = append(profiles, params.TestNet())
	case "all":
		profiles = append(profiles, params.TestNet(), params.DevNet())
	default:
		fmt.Fprintf(os.Stderr, "perfil desconhecido: %q\n", *profile)
		os.Exit(2)
	}

	for _, p := range profiles {
		b := chain.GenesisBlock(p)
		hdr := b.Header
		target := pow.CompactToTarget(hdr.Bits)
		start := time.Now()
		var tries uint64
		for nonce := uint64(0); ; nonce++ {
			hdr.Nonce = nonce
			tries++
			h := pow.PowHash(hdr.Bytes(), p)
			if new(big.Int).SetBytes(h[:]).Cmp(target) <= 0 {
				break
			}
		}
		id := hdr.ID()
		fmt.Printf("perfil %s: nonce %d em %s (%d hashes Argon2id)\n",
			p.Name, hdr.Nonce, time.Since(start).Round(time.Millisecond), tries)
		fmt.Printf("  cole em internal/params/genesis.go:\n")
		fmt.Printf("    Nonce: %d,\n", hdr.Nonce)
		fmt.Printf("    Hash:  %s,\n\n", goHashLiteral(id))
	}
}

func goHashLiteral(h [32]byte) string {
	var sb strings.Builder
	sb.WriteString("[32]byte{")
	for i, v := range h {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(&sb, "0x%02x", v)
	}
	sb.WriteString("}")
	return sb.String()
}
