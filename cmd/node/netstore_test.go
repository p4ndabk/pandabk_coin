package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"zhu/internal/params"
)

// newServedStore sobe um serveRace de teste sobre um demoStore local em
// loopback e devolve um cliente já conectado — simula o Mac A (-db -listen)
// e o Mac B (-peer) na mesma máquina de testes.
func newServedStore(t *testing.T) *netStore {
	t.Helper()
	local, err := openDemoStore(filepath.Join(t.TempDir(), "demo.db"))
	if err != nil {
		t.Fatalf("openDemoStore: %v", err)
	}
	t.Cleanup(func() { local.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	addr, err := serveRace(ctx, "127.0.0.1:0", local)
	if err != nil {
		t.Fatalf("serveRace: %v", err)
	}

	client := dialRaceStore(addr)
	t.Cleanup(func() { client.Close() })
	return client
}

func TestNetStoreRoundTrip(t *testing.T) {
	client := newServedStore(t)

	// tip em corrida vazia, via rede, tem que bater com o comportamento local.
	tip, err := client.tip()
	if err != nil || tip.height != 0 {
		t.Fatalf("tip remoto em corrida vazia: %+v, %v", tip, err)
	}

	// initMeta pela rede grava do lado do servidor.
	want := demoMeta{profile: "test", spacing: 2 * time.Second, retarget: 5, zeros: 9}
	got, created, err := client.initMeta(want)
	if err != nil || !created || got != want {
		t.Fatalf("initMeta remoto: %+v created=%v err=%v", got, created, err)
	}
	// Um segundo client (nova conexão) tem que ver a MESMA config.
	client2 := newClientTo(t, client.addr)
	got2, created2, err := client2.initMeta(demoMeta{profile: "devnet", spacing: time.Minute, retarget: 1, zeros: 1})
	if err != nil || created2 || got2 != want {
		t.Fatalf("segundo cliente deveria adotar a config do primeiro: %+v created=%v err=%v", got2, created2, err)
	}

	// insertBlock pela rede + corrida perdida traduzida de volta em errRaceLost.
	// id/prev precisam ser hex de 32 bytes: é o que store.tip() espera decodificar.
	idA := "aa" + strings.Repeat("00", 31)
	idB := "bb" + strings.Repeat("00", 31)
	zero := strings.Repeat("00", 32)
	row := demoBlockRow{height: 1, id: idA, prev: zero, bits: 0x20010000, nonce: 1, miner: "alice", reward: 50 * params.CoinUnit, attempts: 10, durationMS: 100, foundAt: 1000}
	if err := client.insertBlock(row); err != nil {
		t.Fatalf("insertBlock remoto: %v", err)
	}
	row.id, row.miner = idB, "bob"
	err = client2.insertBlock(row)
	if !errors.Is(err, errRaceLost) {
		t.Fatalf("segundo insert na altura 1 (por outra conexão) deveria perder a corrida, veio %v", err)
	}

	// tip agora reflete o bloco 1 do lado do servidor, visto pelas duas conexões.
	tip, err = client.tip()
	if err != nil || tip.height != 1 {
		t.Fatalf("tip remoto após insert: %+v, %v", tip, err)
	}

	// blockAt, minerBalance, listBlocks, ranking, epochWindow — todos pela rede.
	b, err := client.blockAt(1)
	if err != nil || b.miner != "alice" {
		t.Fatalf("blockAt remoto: %+v, %v", b, err)
	}
	reward, blocks, err := client.minerBalance("alice")
	if err != nil || blocks != 1 || reward != 50*params.CoinUnit {
		t.Fatalf("minerBalance remoto: %d blocos, %d, %v", blocks, reward, err)
	}
	rows, err := client.listBlocks(10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("listBlocks remoto: %+v, %v", rows, err)
	}
	ranks, err := client.ranking()
	if err != nil || len(ranks) != 1 || ranks[0].miner != "alice" {
		t.Fatalf("ranking remoto: %+v, %v", ranks, err)
	}
	first, last, err := client.epochWindow(1, 1)
	if err != nil || first != 1000 || last != 1000 {
		t.Fatalf("epochWindow remoto: %d, %d, %v", first, last, err)
	}
}

func newClientTo(t *testing.T, addr string) *netStore {
	t.Helper()
	c := dialRaceStore(addr)
	t.Cleanup(func() { c.Close() })
	return c
}

func TestNetStoreUnknownOp(t *testing.T) {
	client := newServedStore(t)
	resp, err := client.call(rpcRequest{Op: "nao-existe"})
	if err != nil {
		t.Fatalf("chamada com op desconhecida não deveria falhar no transporte: %v", err)
	}
	if resp.OK {
		t.Fatal("op desconhecida deveria devolver OK=false")
	}
}
