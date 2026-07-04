package p2p

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/binary"
	"errors"
	"math/big"
	"net"
	"path/filepath"
	"testing"
	"time"

	"pandabk_coin/internal/chain"
	"pandabk_coin/internal/core"
	"pandabk_coin/internal/mempool"
	"pandabk_coin/internal/params"
	"pandabk_coin/internal/pow"
)

// ── codec ───────────────────────────────────────────────────────────────────

func TestCodecRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	want := MsgVersion{Protocol: 1, Genesis: "aa", Height: 7, CumWork: "ff", Nonce: 42}
	if err := WriteMsg(&buf, TypeVersion, want); err != nil {
		t.Fatalf("WriteMsg: %v", err)
	}
	env, err := ReadMsg(&buf)
	if err != nil {
		t.Fatalf("ReadMsg: %v", err)
	}
	if env.Type != TypeVersion {
		t.Fatalf("type = %q", env.Type)
	}
	got, err := decodePayload[MsgVersion](env)
	if err != nil || got != want {
		t.Fatalf("payload = %+v, %v; esperava %+v", got, err, want)
	}
}

func TestOversizeFrameRejectedWithoutReadingBody(t *testing.T) {
	// Frame declara 2 MiB mas o corpo nem existe: ReadMsg tem que falhar só
	// com o prefixo, sem tentar alocar/ler o resto.
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], 2<<20)
	_, err := ReadMsg(bytes.NewReader(hdr[:]))
	if !errors.Is(err, ErrFrameTooBig) {
		t.Fatalf("err = %v, esperava ErrFrameTooBig", err)
	}
}

// ── handshake sobre net.Pipe ────────────────────────────────────────────────

// runHandshake executa os dois lados simultaneamente, como na rede real.
func runHandshake(t *testing.T, va, vb MsgVersion) (ra, rb MsgVersion, ea, eb error) {
	t.Helper()
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	done := make(chan struct{})
	go func() {
		rb, eb = doHandshake(b, vb)
		close(done)
	}()
	ra, ea = doHandshake(a, va)
	<-done
	return ra, rb, ea, eb
}

func TestHandshakeHappyPath(t *testing.T) {
	va := MsgVersion{Protocol: ProtocolVersion, Genesis: "cafe", Height: 10, CumWork: "ff", Nonce: 1}
	vb := MsgVersion{Protocol: ProtocolVersion, Genesis: "cafe", Height: 2, CumWork: "0a", Nonce: 2}
	ra, rb, ea, eb := runHandshake(t, va, vb)
	if ea != nil || eb != nil {
		t.Fatalf("handshake falhou: %v / %v", ea, eb)
	}
	if ra != vb || rb != va {
		t.Fatalf("versions trocadas erradas: %+v / %+v", ra, rb)
	}
}

func TestHandshakeGenesisMismatch(t *testing.T) {
	va := MsgVersion{Protocol: ProtocolVersion, Genesis: "cafe", Nonce: 1}
	vb := MsgVersion{Protocol: ProtocolVersion, Genesis: "beef", Nonce: 2}
	_, _, ea, eb := runHandshake(t, va, vb)
	if !errors.Is(ea, ErrGenesisMismatch) || !errors.Is(eb, ErrGenesisMismatch) {
		t.Fatalf("esperava ErrGenesisMismatch dos dois lados: %v / %v", ea, eb)
	}
}

func TestHandshakeProtocolMismatch(t *testing.T) {
	va := MsgVersion{Protocol: 1, Genesis: "cafe", Nonce: 1}
	vb := MsgVersion{Protocol: 2, Genesis: "cafe", Nonce: 2}
	_, _, ea, eb := runHandshake(t, va, vb)
	if !errors.Is(ea, ErrProtocolMismatch) || !errors.Is(eb, ErrProtocolMismatch) {
		t.Fatalf("esperava ErrProtocolMismatch dos dois lados: %v / %v", ea, eb)
	}
}

func TestHandshakeSelfConnection(t *testing.T) {
	v := MsgVersion{Protocol: ProtocolVersion, Genesis: "cafe", Nonce: 77}
	_, _, ea, eb := runHandshake(t, v, v)
	if !errors.Is(ea, ErrSelfConnection) || !errors.Is(eb, ErrSelfConnection) {
		t.Fatalf("esperava ErrSelfConnection: %v / %v", ea, eb)
	}
}

// ── integração: nodes completos in-process ──────────────────────────────────

type testNode struct {
	t   *testing.T
	c   *chain.Chain
	mp  *mempool.Mempool
	s   *Server
	p   params.Params
	key *ecdsa.PrivateKey
	pub []byte
	pkh [20]byte
	cbs []core.OutPoint
}

func newTestNode(t *testing.T, seeds ...string) *testNode {
	t.Helper()
	p := params.TestNet()
	c, err := chain.Open(filepath.Join(t.TempDir(), "chain.db"), p)
	if err != nil {
		t.Fatalf("chain.Open: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	mp := mempool.New(c, p)
	s := NewServer(Config{
		Listen:         "127.0.0.1:0",
		Seeds:          seeds,
		RedialInterval: 50 * time.Millisecond,
	}, c, mp, p)
	key, err := core.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	pub := core.CompressPubKey(&key.PublicKey)
	return &testNode{t: t, c: c, mp: mp, s: s, p: p, key: key, pub: pub, pkh: core.HashPubKey(pub)}
}

func (n *testNode) start() {
	n.t.Helper()
	if err := n.s.Start(); err != nil {
		n.t.Fatalf("Start: %v", err)
	}
	n.t.Cleanup(func() { n.s.Stop() })
}

func (n *testNode) mine(count int) {
	n.t.Helper()
	for i := 0; i < count; i++ {
		prev, _, _ := n.c.Tip()
		height := prev.Height + 1
		cb := core.NewCoinbase(height, n.p.BlockSubsidy(height), n.pkh, nil)
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
		for nn := uint64(0); ; nn++ {
			b.Header.Nonce = nn
			hash := pow.PowHash(b.Header.Bytes(), n.p)
			if new(big.Int).SetBytes(hash[:]).Cmp(target) <= 0 {
				break
			}
		}
		if err := n.c.AcceptBlock(b); err != nil {
			n.t.Fatalf("AcceptBlock: %v", err)
		}
		n.cbs = append(n.cbs, core.OutPoint{TxID: b.Txs[0].TxID(), Index: 0})
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout esperando: %s", what)
}

func TestIBDSyncsFiftyBlocks(t *testing.T) {
	a := newTestNode(t)
	a.mine(50)
	a.start()

	b := newTestNode(t, a.s.Addr())
	b.start()

	wantTip, _, _ := a.c.Tip()
	waitFor(t, "B sincronizar os 50 blocos de A", func() bool {
		tip, height, _ := b.c.Tip()
		return height == 50 && tip.ID() == wantTip.ID()
	})
}

func TestForksConvergeToHeavierChain(t *testing.T) {
	a := newTestNode(t)
	a.mine(3)
	b := newTestNode(t) // fork independente: coinbase de outro dono
	b.mine(5)

	a.start()
	b.s.cfg.Seeds = []string{a.s.Addr()}
	b.start()

	wantTip, _, _ := b.c.Tip() // B tem mais trabalho: A converge para B
	waitFor(t, "A adotar a chain mais pesada de B", func() bool {
		tip, height, _ := a.c.Tip()
		return height == 5 && tip.ID() == wantTip.ID()
	})
	// e B continua na própria chain (não regrediu)
	tip, height, _ := b.c.Tip()
	if height != 5 || tip.ID() != wantTip.ID() {
		t.Fatalf("B não deveria ter mudado de chain: altura %d", height)
	}
}

func TestTxGossip(t *testing.T) {
	a := newTestNode(t)
	a.mine(12) // coinbase do bloco 1 madura na próxima altura
	a.start()

	b := newTestNode(t, a.s.Addr())
	b.start()
	waitFor(t, "B sincronizar antes do gossip", func() bool {
		_, height, _ := b.c.Tip()
		return height == 12
	})

	// tx gastando a coinbase 1 de A, com taxa
	subsidy := 50 * params.CoinUnit
	tx := core.Tx{
		Version: 1,
		Ins:     []core.TxIn{{Prev: a.cbs[0], PubKey: a.pub}},
		Outs:    []core.TxOut{{Value: subsidy - 1000, PubKeyHash: [20]byte{9}}},
	}
	sigHash, err := tx.SigHash(0, a.pkh)
	if err != nil {
		t.Fatal(err)
	}
	tx.Ins[0].Sig, err = core.SignHash(a.key, sigHash)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.mp.Add(&tx); err != nil {
		t.Fatalf("mempool A rejeitou a tx: %v", err)
	}
	a.s.BroadcastTx(&tx)

	txid := tx.TxID()
	waitFor(t, "tx aparecer no mempool de B", func() bool {
		return b.mp.Has(txid)
	})
}

func TestNewBlockPropagatesViaInv(t *testing.T) {
	a := newTestNode(t)
	a.mine(5)
	a.start()
	b := newTestNode(t, a.s.Addr())
	b.start()
	waitFor(t, "sync inicial", func() bool {
		_, height, _ := b.c.Tip()
		return height == 5
	})

	// A minera mais um e anuncia (o que o miner fará no M5)
	a.mine(1)
	tip, _, _ := a.c.Tip()
	a.s.BroadcastBlock(tip.ID())
	waitFor(t, "bloco novo chegar em B via inv/getdata", func() bool {
		btip, height, _ := b.c.Tip()
		return height == 6 && btip.ID() == tip.ID()
	})
}
