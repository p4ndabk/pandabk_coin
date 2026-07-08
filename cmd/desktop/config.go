package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"zhu/internal/node"
)

// O desktop ancora o zhu.conf num lugar FIXO (~/.zhu/zhu.conf) para
// não depender do diretório de onde o app foi aberto — clique duplo no
// Finder funciona. Compatibilidade: -config explícito vence, e um
// zhu.conf no diretório atual (fluxo antigo/CLI) ainda é respeitado.

func desktopConfigPath(configFlag string) string {
	if configFlag != "" {
		return configFlag
	}
	if _, err := os.Stat(node.DefaultConfigFile); err == nil {
		return node.DefaultConfigFile // zhu.conf do diretório atual (CLI)
	}
	return filepath.Join(node.DefaultDataDir(), "zhu.conf")
}

// confValues é o que a interface edita — o subconjunto do node.Config que
// faz sentido mexer numa tela (o profile fica fora: é decisão de build).
type confValues struct {
	Peers     string // host:porta separados por vírgula
	Proxy     string // SOCKS5 (Tor) para conexões de saída; vazio = direto
	Advertise string // endereço anunciado aos peers (ex.: seu .onion); vazio = o do Listen
	Listen    string
	RPC       string
	DataDir   string
	Mine      bool
	Miners    int
}

func confFromConfig(cfg *node.Config) confValues {
	return confValues{
		Peers:     strings.Join(cfg.Peers, ","),
		Proxy:     cfg.Proxy,
		Advertise: cfg.Advertise,
		Listen:    cfg.Listen,
		RPC:       cfg.RPC,
		DataDir:   cfg.DataDir,
		Mine:      cfg.Mine,
		Miners:    cfg.Miners,
	}
}

// apply projeta os valores da tela de volta num node.Config.
func (v confValues) apply(cfg node.Config) node.Config {
	cfg.Peers = nil
	for _, p := range strings.Split(v.Peers, ",") {
		if p = strings.TrimSpace(p); p != "" {
			cfg.Peers = append(cfg.Peers, p)
		}
	}
	cfg.Proxy = strings.TrimSpace(v.Proxy)
	cfg.Advertise = strings.TrimSpace(v.Advertise)
	cfg.Listen = strings.TrimSpace(v.Listen)
	cfg.RPC = strings.TrimSpace(v.RPC)
	cfg.DataDir = strings.TrimSpace(v.DataDir)
	cfg.Mine = v.Mine
	cfg.Miners = v.Miners
	return cfg
}

func (v confValues) validate() error {
	if strings.TrimSpace(v.DataDir) == "" {
		return fmt.Errorf("datadir não pode ficar vazio")
	}
	if v.Miners < 1 || v.Miners > 32 {
		return fmt.Errorf("miners precisa estar entre 1 e 32 (cada worker usa 1 core e ~64 MiB)")
	}
	if strings.TrimSpace(v.RPC) == "" {
		return fmt.Errorf("rpc não pode ficar vazio (padrão 127.0.0.1:8555)")
	}
	return nil
}

// saveConf grava o zhu.conf gerado pela interface (chave=valor, as mesmas
// chaves dos flags — o arquivo continua legível pela CLI).
func saveConf(path string, v confValues) error {
	if err := v.validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# zhu.conf — gerado pela aba Ajustes do Zhu Desktop.\n")
	b.WriteString("# Editar à mão também vale: chave=valor, as chaves são os nomes dos flags.\n")
	if v.Peers != "" {
		b.WriteString("peers=" + v.Peers + "\n")
	}
	if v.Proxy != "" {
		b.WriteString("proxy=" + v.Proxy + "\n")
	}
	if v.Advertise != "" {
		b.WriteString("advertise=" + v.Advertise + "\n")
	}
	b.WriteString("listen=" + v.Listen + "\n")
	b.WriteString("rpc=" + v.RPC + "\n")
	b.WriteString("datadir=" + v.DataDir + "\n")
	b.WriteString("mine=" + strconv.FormatBool(v.Mine) + "\n")
	b.WriteString("miners=" + strconv.Itoa(v.Miners) + "\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
