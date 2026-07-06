package main

import (
	"flag"
	"path/filepath"
	"testing"

	"pandabk_coin/internal/node"
)

func TestSaveConfRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panda.conf")
	v := confValues{
		Peers:   "192.168.1.10:9551,10.0.0.2:9551",
		Listen:  ":9551",
		RPC:     "127.0.0.1:8555",
		DataDir: "/tmp/panda-teste",
		Mine:    true,
		Miners:  2,
	}
	if err := saveConf(path, v); err != nil {
		t.Fatalf("saveConf: %v", err)
	}

	// O arquivo salvo tem que ser legível pelo MESMO parser da CLI.
	cfg, loaded := loadConfForTest(t, path)
	if !loaded {
		t.Fatal("panda.conf salvo não foi lido")
	}
	if len(cfg.Peers) != 2 || cfg.Peers[0] != "192.168.1.10:9551" {
		t.Fatalf("peers = %v", cfg.Peers)
	}
	if cfg.Listen != ":9551" || cfg.RPC != "127.0.0.1:8555" || cfg.DataDir != "/tmp/panda-teste" {
		t.Fatalf("campos = %+v", cfg)
	}
	if !cfg.Mine || cfg.Miners != 2 {
		t.Fatalf("mine/miners = %v/%d", cfg.Mine, cfg.Miners)
	}
}

func TestSaveConfRejectsInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panda.conf")
	for _, v := range []confValues{
		{DataDir: "", RPC: "127.0.0.1:8555", Miners: 1},        // datadir vazio
		{DataDir: "/tmp/x", RPC: "", Miners: 1},                // rpc vazio
		{DataDir: "/tmp/x", RPC: "127.0.0.1:8555", Miners: 0},  // miners < 1
		{DataDir: "/tmp/x", RPC: "127.0.0.1:8555", Miners: 99}, // miners > 32
	} {
		if err := saveConf(path, v); err == nil {
			t.Errorf("saveConf(%+v) deveria falhar", v)
		}
	}
}

// loadConfForTest lê o arquivo com o fluxo real (RegisterFlags +
// ApplyConfigFile + FinishFlags) — o mesmo caminho do main.
func loadConfForTest(t *testing.T, path string) (*node.Config, bool) {
	t.Helper()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg, peersCSV := node.RegisterFlags(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := node.ApplyConfigFile(fs, path); err != nil {
		t.Fatalf("ApplyConfigFile: %v", err)
	}
	node.FinishFlags(cfg, *peersCSV)
	return cfg, true
}
