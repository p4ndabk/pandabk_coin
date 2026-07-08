package main

import (
	"flag"
	"fmt"
	"os"

	"zhu/internal/node"
)

// applyConfig é o node.ApplyConfigFile com o comportamento de CLI: erro de
// config é fatal (mensagem + exit 2). O app de desktop usa a versão com
// erro para mostrar um diálogo em vez de morrer.
func applyConfig(fs *flag.FlagSet, configFlag string) map[string]bool {
	fromCLI, err := node.ApplyConfigFile(fs, configFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(2)
	}
	return fromCLI
}
