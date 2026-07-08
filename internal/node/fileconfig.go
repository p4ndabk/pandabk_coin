package node

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/joho/godotenv"
)

// DefaultConfigFile é procurado no diretório atual quando -config não é
// passado; se não existir, o comando segue só com flags/defaults.
const DefaultConfigFile = "zhu.conf"

// ApplyConfigFile carrega um arquivo de configuração (formato chave=valor,
// uma por linha, # comenta — as chaves são os mesmos nomes dos flags) e
// aplica aos flags que NÃO vieram na linha de comando: flag explícito sempre
// vence. configFlag é o valor do flag -config: vazio usa o zhu.conf do
// diretório atual (opcional); explícito exige que o arquivo exista. Cada
// consumidor aproveita apenas as chaves que conhece, então um único
// zhu.conf serve para CLI, demo e app de desktop. Devolve o conjunto de
// flags passados explicitamente na linha de comando.
func ApplyConfigFile(fs *flag.FlagSet, configFlag string) (map[string]bool, error) {
	path, explicit := configFlag, true
	if path == "" {
		path, explicit = DefaultConfigFile, false
	}
	fromCLI := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { fromCLI[f.Name] = true })

	vals, err := godotenv.Read(path)
	if err != nil {
		if !explicit && os.IsNotExist(err) {
			return fromCLI, nil // sem zhu.conf por perto: nada a aplicar
		}
		return fromCLI, fmt.Errorf("lendo config %s: %w", path, err)
	}

	var applied []string
	for k, v := range vals {
		name := strings.ToLower(strings.TrimSpace(k))
		if name == "config" || fromCLI[name] || fs.Lookup(name) == nil {
			continue
		}
		if err := fs.Set(name, v); err != nil {
			return fromCLI, fmt.Errorf("config %s: %s=%q inválido: %w", path, name, v, err)
		}
		applied = append(applied, name+"="+v)
	}
	if len(applied) > 0 {
		sort.Strings(applied)
		fmt.Fprintf(os.Stderr, "⚙️  %s: %s\n", path, strings.Join(applied, "  "))
	}
	return fromCLI, nil
}
