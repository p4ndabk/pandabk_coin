// Package node orquestra o full node PANDA: chain + mempool + p2p + miner
// num processo só, com RPC JSON em localhost para a CLI (o bbolt é
// single-writer — `node balance` pergunta ao processo em vez de abrir o
// banco). Wiring manual, sem DI. Ver internal/node/SPEC.md.
package node

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"pandabk_coin/internal/params"
)

// DefaultProfile é o perfil de consenso default do binário. O build.conf do
// desenvolvedor pode trocá-lo via -ldflags -X (chave profile=) — quem recebe
// o binário pronto roda `node run` sem pensar em perfil.
var DefaultProfile = "devnet"

// Config do node. Flags primeiro, env NODE_* como fallback (mesmo padrão
// getEnv de internal/config, sem tocá-lo).
type Config struct {
	DataDir   string
	Listen    string
	RPC       string
	Peers     []string
	Proxy     string // SOCKS5 para conexões de saída (Tor); vazio = direto
	Advertise string // endereço anunciado aos peers (.onion de um hidden service)
	Mine      bool
	Miners    int
	Profile   string
}

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".panda"
	}
	return filepath.Join(home, ".panda")
}

// DefaultDataDir é o datadir padrão (~/.panda) — exportado para o app de
// desktop ancorar o panda.conf dele num lugar fixo, independente do
// diretório de onde foi aberto.
func DefaultDataDir() string { return defaultDataDir() }

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

// RegisterFlags registra os flags do `node run` num FlagSet, com os
// defaults vindos do ambiente — flag explícito vence env, env vence default.
// peersCSV é lido depois do Parse via FinishFlags.
func RegisterFlags(fs *flag.FlagSet) (*Config, *string) {
	cfg := &Config{}
	fs.StringVar(&cfg.DataDir, "datadir", getEnv("NODE_DATADIR", defaultDataDir()), "diretório de dados do node (chain.db, wallet.json)")
	fs.StringVar(&cfg.Listen, "listen", getEnv("NODE_LISTEN", ":9551"), "endereço P2P (vazio = só conexões de saída, funciona atrás de NAT)")
	fs.StringVar(&cfg.RPC, "rpc", getEnv("NODE_RPC", "127.0.0.1:8555"), "endereço da RPC local (obrigatoriamente loopback)")
	fs.BoolVar(&cfg.Mine, "mine", getEnvBool("NODE_MINE", true), "minerar (padrão LIGADO, 1 worker — todo node contribui; desligue com -mine=false)")
	fs.IntVar(&cfg.Miners, "miners", getEnvInt("NODE_MINERS", 1), "workers de mineração (1 core e ~64 MiB cada)")
	fs.StringVar(&cfg.Profile, "profile", getEnv("NODE_PROFILE", DefaultProfile), "perfil de consenso: devnet ou test")
	fs.StringVar(&cfg.Proxy, "proxy", getEnv("NODE_PROXY", ""), "proxy SOCKS5 para TODA conexão de saída (ex.: 127.0.0.1:9050 do Tor — permite discar peers .onion; vazio = direto)")
	fs.StringVar(&cfg.Advertise, "advertise", getEnv("NODE_ADVERTISE", ""), "endereço anunciado aos peers (ex.: seuendereco.onion:9551 de um hidden service; vazio = o do -listen)")
	peersCSV := fs.String("peers", getEnv("NODE_PEERS", ""), "peers iniciais host:porta, separados por vírgula")
	return cfg, peersCSV
}

// FinishFlags completa a config após o Parse (peers CSV → slice).
func FinishFlags(cfg *Config, peersCSV string) {
	for _, p := range strings.Split(peersCSV, ",") {
		if p = strings.TrimSpace(p); p != "" {
			cfg.Peers = append(cfg.Peers, p)
		}
	}
}

// Params resolve o perfil de consenso da config.
func (c Config) Params() (params.Params, error) {
	switch c.Profile {
	case "devnet":
		return params.DevNet(), nil
	case "test":
		return params.TestNet(), nil
	default:
		return params.Params{}, fmt.Errorf("node: perfil desconhecido %q (use devnet ou test)", c.Profile)
	}
}

// WalletPath é onde a wallet do node mora dentro do datadir.
func (c Config) WalletPath() string { return filepath.Join(c.DataDir, "wallet.json") }

// ChainPath é o arquivo bbolt da chain dentro do datadir.
func (c Config) ChainPath() string { return filepath.Join(c.DataDir, "chain.db") }

// MempoolPath é onde as transações pendentes hibernam entre um Stop e o
// próximo Start (o mempool.dat da PANDA).
func (c Config) MempoolPath() string { return filepath.Join(c.DataDir, "mempool.json") }
