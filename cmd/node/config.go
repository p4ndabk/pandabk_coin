package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/joho/godotenv"
)

// defaultConfigFile é procurado no diretório atual quando -config não é
// passado; se não existir, o comando segue só com flags/defaults.
const defaultConfigFile = "panda.conf"

// applyConfig carrega um arquivo de configuração (formato chave=valor, uma
// por linha, # comenta — as chaves são os mesmos nomes dos flags) e aplica
// aos flags que NÃO vieram na linha de comando: flag explícito sempre vence.
// configFlag é o valor do flag -config: vazio usa o panda.conf do diretório
// atual (opcional); explícito exige que o arquivo exista. Cada subcomando
// aproveita do arquivo apenas as chaves que conhece, então um único
// panda.conf serve para powdemo, blocks e ranking. Devolve o conjunto de
// flags que o usuário passou explicitamente na linha de comando.
func applyConfig(fs *flag.FlagSet, configFlag string) map[string]bool {
	path, explicit := configFlag, true
	if path == "" {
		path, explicit = defaultConfigFile, false
	}
	fromCLI := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { fromCLI[f.Name] = true })

	vals, err := godotenv.Read(path)
	if err != nil {
		if !explicit && os.IsNotExist(err) {
			return fromCLI // sem panda.conf por perto: nada a aplicar
		}
		fmt.Fprintf(os.Stderr, "lendo config %s: %v\n", path, err)
		os.Exit(2)
	}

	var applied []string
	for k, v := range vals {
		name := strings.ToLower(strings.TrimSpace(k))
		if name == "config" || fromCLI[name] || fs.Lookup(name) == nil {
			continue
		}
		if err := fs.Set(name, v); err != nil {
			fmt.Fprintf(os.Stderr, "config %s: %s=%q inválido: %v\n", path, name, v, err)
			os.Exit(2)
		}
		applied = append(applied, name+"="+v)
	}
	if len(applied) > 0 {
		sort.Strings(applied)
		fmt.Fprintf(os.Stderr, "⚙️  %s: %s\n", path, strings.Join(applied, "  "))
	}
	return fromCLI
}
