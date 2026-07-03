package main

import (
	"context"
	"math/big"
	"path/filepath"
	"strings"
	"testing"

	"pandabk_coin/internal/params"
	"pandabk_coin/internal/pow"
)

// bits usados nos testes: heavyBits tem MENOS trabalho médio por tentativa
// (target menor) do que lightBits — mesmos valores de internal/pow/pow_test.go.
const (
	heavyBits = 0x20010000 // target 2^248: mais trabalho
	lightBits = 0x207fffff // target ~2^255: menos trabalho
)

func newLocalStore(t *testing.T) *demoStore {
	t.Helper()
	s, err := openDemoStore(filepath.Join(t.TempDir(), "local.db"))
	if err != nil {
		t.Fatalf("openDemoStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// newPeer sobe um demoStore servido por TCP e devolve tanto o banco (pra
// popular blocos diretamente nos testes) quanto o cliente netStore que o
// reconcile vai usar — o mesmo papel de -listen/-peer entre dois Macs.
func newPeer(t *testing.T) (*demoStore, *netStore) {
	t.Helper()
	store := newLocalStore(t) // reusa o helper, é só outro arquivo
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	addr, err := serveRace(ctx, "127.0.0.1:0", store)
	if err != nil {
		t.Fatalf("serveRace: %v", err)
	}
	client := dialRaceStore(addr)
	t.Cleanup(func() { client.Close() })
	return store, client
}

func idHex(b byte) string {
	return strings.Repeat(string(rune(b)), 2) + strings.Repeat("0", 62)
}

func block(height uint64, id string, bits uint32, foundAt int64) demoBlockRow {
	return demoBlockRow{
		height: height, id: id, prev: strings.Repeat("0", 64), bits: bits,
		nonce: height, miner: "x", reward: 50 * params.CoinUnit, attempts: 1, durationMS: 1, foundAt: foundAt,
	}
}

func TestChainWork(t *testing.T) {
	s := newLocalStore(t)
	for _, b := range []demoBlockRow{
		block(1, idHex('1'), heavyBits, 100),
		block(2, idHex('2'), lightBits, 101),
	} {
		if err := s.insertBlock(b); err != nil {
			t.Fatalf("insertBlock: %v", err)
		}
	}
	want := new(big.Int).Add(pow.BlockWork(heavyBits), pow.BlockWork(lightBits))

	got, err := chainWork(s, 1, 2)
	if err != nil {
		t.Fatalf("chainWork: %v", err)
	}
	if got.Cmp(want) != 0 {
		t.Fatalf("chainWork = %s, want %s", got, want)
	}
	// Intervalo vazio (from > to) soma zero.
	zero, err := chainWork(s, 5, 4)
	if err != nil || zero.Sign() != 0 {
		t.Fatalf("intervalo vazio deveria somar zero: %v, %v", zero, err)
	}
}

func TestReconcileEmptyLocalAdoptsWholePeerChain(t *testing.T) {
	local := newLocalStore(t)
	peerStore, peerClient := newPeer(t)
	for _, b := range []demoBlockRow{
		block(1, idHex('1'), heavyBits, 100),
		block(2, idHex('2'), heavyBits, 101),
		block(3, idHex('3'), heavyBits, 102),
	} {
		if err := peerStore.insertBlock(b); err != nil {
			t.Fatalf("populando peer: %v", err)
		}
	}

	if err := reconcile(local, peerClient, "bob"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	tip, err := local.tip()
	if err != nil || tip.height != 3 {
		t.Fatalf("local deveria ter adotado a chain inteira do peer: altura %d, %v", tip.height, err)
	}
	for h := uint64(1); h <= 3; h++ {
		row, err := local.blockAt(h)
		if err != nil {
			t.Fatalf("blockAt(%d): %v", h, err)
		}
		want, _ := peerStore.blockAt(h)
		if row.id != want.id {
			t.Fatalf("bloco %d divergiu: local=%s peer=%s", h, row.id, want.id)
		}
	}
}

func TestReconcileKeepsHeavierLocalChain(t *testing.T) {
	local := newLocalStore(t)
	peerStore, peerClient := newPeer(t)

	// Ancestral comum na altura 1; divergem na altura 2.
	common := block(1, idHex('1'), heavyBits, 100)
	if err := local.insertBlock(common); err != nil {
		t.Fatal(err)
	}
	if err := peerStore.insertBlock(common); err != nil {
		t.Fatal(err)
	}
	if err := local.insertBlock(block(2, idHex('a'), heavyBits, 101)); err != nil { // pesado
		t.Fatal(err)
	}
	if err := peerStore.insertBlock(block(2, idHex('b'), lightBits, 101)); err != nil { // leve
		t.Fatal(err)
	}

	if err := reconcile(local, peerClient, "alice"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	tip, err := local.tip()
	if err != nil || tip.height != 2 {
		t.Fatalf("altura local não deveria mudar: %d, %v", tip.height, err)
	}
	row, _ := local.blockAt(2)
	if row.id != idHex('a') {
		t.Fatalf("chain local (mais pesada) deveria ter sido mantida, mas virou %s", row.id)
	}
}

func TestReconcileAdoptsHeavierPeerChainReorg(t *testing.T) {
	local := newLocalStore(t)
	peerStore, peerClient := newPeer(t)

	common := block(1, idHex('1'), heavyBits, 100)
	if err := local.insertBlock(common); err != nil {
		t.Fatal(err)
	}
	if err := peerStore.insertBlock(common); err != nil {
		t.Fatal(err)
	}
	if err := local.insertBlock(block(2, idHex('a'), lightBits, 101)); err != nil { // leve
		t.Fatal(err)
	}
	if err := peerStore.insertBlock(block(2, idHex('b'), heavyBits, 101)); err != nil { // pesado
		t.Fatal(err)
	}

	if err := reconcile(local, peerClient, "alice"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	row, err := local.blockAt(2)
	if err != nil || row.id != idHex('b') {
		t.Fatalf("deveria ter adotado o bloco 2 do peer (mais pesado): %+v, %v", row, err)
	}
	// A altura 1 (ancestral comum) não devia ter sido tocada.
	row1, err := local.blockAt(1)
	if err != nil || row1.id != idHex('1') {
		t.Fatalf("ancestral comum não deveria mudar: %+v, %v", row1, err)
	}
}

func TestReconcileTieBreakIsConsistent(t *testing.T) {
	// Mesmo trabalho dos dois lados (mesmos bits) — o desempate tem que ser
	// determinístico: quem tem o tip id lexicograficamente MENOR vence.
	local := newLocalStore(t)
	peerStore, peerClient := newPeer(t)

	if err := local.insertBlock(block(1, idHex('a'), heavyBits, 100)); err != nil { // "aa...", menor
		t.Fatal(err)
	}
	if err := peerStore.insertBlock(block(1, idHex('b'), heavyBits, 100)); err != nil { // "bb...", maior
		t.Fatal(err)
	}
	if err := reconcile(local, peerClient, "x"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	row, _ := local.blockAt(1)
	if row.id != idHex('a') {
		t.Fatalf("id local menor deveria vencer o empate, mas virou %s", row.id)
	}

	// Invertendo: agora o peer tem o id menor — ele deveria vencer.
	local2 := newLocalStore(t)
	peerStore2, peerClient2 := newPeer(t)
	if err := local2.insertBlock(block(1, idHex('b'), heavyBits, 100)); err != nil {
		t.Fatal(err)
	}
	if err := peerStore2.insertBlock(block(1, idHex('a'), heavyBits, 100)); err != nil {
		t.Fatal(err)
	}
	if err := reconcile(local2, peerClient2, "x"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	row2, _ := local2.blockAt(1)
	if row2.id != idHex('a') {
		t.Fatalf("id do peer era o menor, deveria ter sido adotado, mas virou %s", row2.id)
	}
}

func TestReconcileNoOpWhenAlreadyInSync(t *testing.T) {
	local := newLocalStore(t)
	peerStore, peerClient := newPeer(t)
	b := block(1, idHex('1'), heavyBits, 100)
	if err := local.insertBlock(b); err != nil {
		t.Fatal(err)
	}
	if err := peerStore.insertBlock(b); err != nil {
		t.Fatal(err)
	}
	if err := reconcile(local, peerClient, "x"); err != nil {
		t.Fatalf("reconcile em chains já iguais não deveria falhar: %v", err)
	}
	tip, _ := local.tip()
	if tip.height != 1 {
		t.Fatalf("altura não deveria mudar: %d", tip.height)
	}
}
