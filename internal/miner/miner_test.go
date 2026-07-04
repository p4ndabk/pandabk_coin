package miner

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"pandabk_coin/internal/chain"
	"pandabk_coin/internal/core"
	"pandabk_coin/internal/mempool"
	"pandabk_coin/internal/params"
	"pandabk_coin/internal/pow"
)

type harness struct {
	t   *testing.T
	c   *chain.Chain
	mp  *mempool.Mempool
	p   params.Params
	key *ecdsa.PrivateKey
	pub []byte
	pkh [20]byte
	cbs []core.OutPoint
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	p := params.TestNet()
	c, err := chain.Open(filepath.Join(t.TempDir(), "chain.db"), p)
	if err != nil {
		t.Fatalf("chain.Open: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	key, err := core.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	pub := core.CompressPubKey(&key.PublicKey)
	return &harness{
		t: t, c: c, mp: mempool.New(c, p), p: p,
		key: key, pub: pub, pkh: core.HashPubKey(pub),
	}
}

// mineExternal simula um bloco vindo "da rede" (fora do miner).
func (h *harness) mineExternal() {
	h.t.Helper()
	prev, _, _ := h.c.Tip()
	height := prev.Height + 1
	cb := core.NewCoinbase(height, h.p.BlockSubsidy(height), h.pkh, []byte{0xee})
	b := &core.Block{
		Header: core.Header{
			Version:    1,
			Height:     height,
			PrevHash:   prev.ID(),
			MerkleRoot: core.MerkleRoot([][32]byte{cb.TxID()}),
			Timestamp:  prev.Timestamp + 30,
			Bits:       prev.Bits,
		},
		Txs: []core.Tx{cb},
	}
	target := pow.CompactToTarget(b.Header.Bits)
	for n := uint64(0); ; n++ {
		b.Header.Nonce = n
		hash := pow.PowHash(b.Header.Bytes(), h.p)
		if new(big.Int).SetBytes(hash[:]).Cmp(target) <= 0 {
			break
		}
	}
	if err := h.c.AcceptBlock(b); err != nil {
		h.t.Fatalf("AcceptBlock externo: %v", err)
	}
	h.cbs = append(h.cbs, core.OutPoint{TxID: cb.TxID(), Index: 0})
}

func TestMinesValidBlocksAndCallsOnBlock(t *testing.T) {
	h := newHarness(t)
	mn := New(h.c, h.mp, h.p, h.pkh, 1)

	var mined atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mn.Start(ctx, func(b *core.Block) { mined.Add(1) })
	defer mn.Stop()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, height, _ := h.c.Tip(); height >= 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	_, height, _ := h.c.Tip()
	if height < 3 {
		t.Fatalf("miner deveria ter minerado ≥3 blocos, altura = %d", height)
	}
	if mined.Load() < 3 {
		t.Fatalf("onBlock chamado %d vezes, esperava ≥3", mined.Load())
	}
	if mn.HashRate() <= 0 {
		t.Fatal("hashrate deveria ser > 0 enquanto minera")
	}
}

func TestTemplateIncludesTxsAndFees(t *testing.T) {
	h := newHarness(t)
	for i := 0; i < 11; i++ {
		h.mineExternal() // coinbase 1 madura na altura 12
	}
	subsidy := 50 * params.CoinUnit
	fee := uint64(7_000)
	tx := core.Tx{
		Version: 1,
		Ins:     []core.TxIn{{Prev: h.cbs[0], PubKey: h.pub}},
		Outs:    []core.TxOut{{Value: subsidy - fee, PubKeyHash: [20]byte{5}}},
	}
	sigHash, _ := tx.SigHash(0, h.pkh)
	tx.Ins[0].Sig, _ = core.SignHash(h.key, sigHash)
	if err := h.mp.Add(&tx); err != nil {
		t.Fatalf("mempool.Add: %v", err)
	}

	mn := New(h.c, h.mp, h.p, h.pkh, 1)
	tmpl, err := mn.buildTemplate(0)
	if err != nil {
		t.Fatalf("buildTemplate: %v", err)
	}
	if len(tmpl.Txs) != 2 || tmpl.Txs[1].TxID() != tx.TxID() {
		t.Fatalf("template deveria incluir a tx do mempool: %d txs", len(tmpl.Txs))
	}
	wantCB := h.p.BlockSubsidy(12) + fee
	if got := tmpl.Txs[0].Outs[0].Value; got != wantCB {
		t.Fatalf("coinbase = %d, esperava subsídio+taxa = %d", got, wantCB)
	}
	// e o bloco do template, minerado, é aceito pela chain
	target := pow.CompactToTarget(tmpl.Header.Bits)
	for n := uint64(0); ; n++ {
		tmpl.Header.Nonce = n
		hash := pow.PowHash(tmpl.Header.Bytes(), h.p)
		if new(big.Int).SetBytes(hash[:]).Cmp(target) <= 0 {
			break
		}
	}
	if err := h.c.AcceptBlock(tmpl); err != nil {
		t.Fatalf("bloco do template rejeitado pela chain: %v", err)
	}
}

func TestTemplateFollowsNewTip(t *testing.T) {
	h := newHarness(t)
	mn := New(h.c, h.mp, h.p, h.pkh, 1)

	t1, err := mn.buildTemplate(0)
	if err != nil {
		t.Fatal(err)
	}
	if t1.Header.Height != 1 {
		t.Fatalf("primeiro template na altura %d, esperava 1", t1.Header.Height)
	}
	h.mineExternal() // "a rede" achou o bloco 1
	mn.TipChanged()  // o node avisa (o poll de 500ms também pegaria)
	t2, err := mn.buildTemplate(0)
	if err != nil {
		t.Fatal(err)
	}
	tip, _, _ := h.c.Tip()
	if t2.Header.Height != 2 || t2.Header.PrevHash != tip.ID() {
		t.Fatalf("template não seguiu o novo tip: altura %d", t2.Header.Height)
	}
}

func TestWorkersUseDistinctCoinbases(t *testing.T) {
	h := newHarness(t)
	mn := New(h.c, h.mp, h.p, h.pkh, 2)
	a, _ := mn.buildTemplate(0)
	b, _ := mn.buildTemplate(1)
	if a.Txs[0].TxID() == b.Txs[0].TxID() {
		t.Fatal("extranonce deveria diferenciar as coinbases dos workers")
	}
	if a.Header.MerkleRoot == b.Header.MerkleRoot {
		t.Fatal("merkle roots dos workers deveriam divergir")
	}
}
