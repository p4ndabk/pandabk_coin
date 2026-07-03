package chain

import (
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"math/big"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"

	"pandabk_coin/internal/core"
	"pandabk_coin/internal/params"
	"pandabk_coin/internal/pow"
)

// Harness padrão do M2: chain em arquivo temporário com o perfil de teste
// (Argon2 de 1 MiB, target folgado) e um minerador com chave própria.
// O relógio é injetado (bem à frente dos timestamps gerados) para que os
// testes não dependam da hora real da máquina.
type tc struct {
	t   *testing.T
	c   *Chain
	p   params.Params
	key *ecdsa.PrivateKey
	pub []byte
	pkh [20]byte
}

func newTestChain(t *testing.T) *tc {
	t.Helper()
	p := params.TestNet()
	c, err := Open(filepath.Join(t.TempDir(), "chain.db"), p)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	c.now = func() time.Time { return time.Unix(p.Genesis.Timestamp+30*24*3600, 0) }
	key, err := core.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pub := core.CompressPubKey(&key.PublicKey)
	return &tc{t: t, c: c, p: p, key: key, pub: pub, pkh: core.HashPubKey(pub)}
}

// buildBlock monta e minera um bloco filho de prev com a coinbase dada e as
// txs extras. extra diferencia coinbases de mineradores/ramos distintos.
func (h *tc) buildBlock(prev core.Header, txs []core.Tx, cbValue uint64, cbPKH [20]byte, extra byte) *core.Block {
	h.t.Helper()
	height := prev.Height + 1
	cb := core.NewCoinbase(height, cbValue, cbPKH, []byte{extra})
	all := append([]core.Tx{cb}, txs...)
	txids := make([][32]byte, len(all))
	for i := range all {
		txids[i] = all[i].TxID()
	}
	b := &core.Block{
		Header: core.Header{
			Version:    1,
			Height:     height,
			PrevHash:   prev.ID(),
			MerkleRoot: core.MerkleRoot(txids),
			Timestamp:  prev.Timestamp + 30,
			Bits:       prev.Bits,
		},
		Txs: all,
	}
	h.mine(b)
	return b
}

func (h *tc) mine(b *core.Block) {
	target := pow.CompactToTarget(b.Header.Bits)
	for n := uint64(0); ; n++ {
		b.Header.Nonce = n
		hash := pow.PowHash(b.Header.Bytes(), h.p)
		if new(big.Int).SetBytes(hash[:]).Cmp(target) <= 0 {
			return
		}
	}
}

// extend minera e aceita n blocos só-coinbase pagando h.pkh, a partir da ponta.
func (h *tc) extend(n int) []*core.Block {
	h.t.Helper()
	var out []*core.Block
	prev, _, _ := h.c.Tip()
	for i := 0; i < n; i++ {
		b := h.buildBlock(prev, nil, h.p.BlockSubsidy(prev.Height+1), h.pkh, 0)
		if err := h.c.AcceptBlock(b); err != nil {
			h.t.Fatalf("AcceptBlock(altura %d): %v", b.Header.Height, err)
		}
		out = append(out, b)
		prev = b.Header
	}
	return out
}

// signedSpend gasta um outpoint de h.key num tx de um input.
func (h *tc) signedSpend(prevOp core.OutPoint, outs []core.TxOut) core.Tx {
	h.t.Helper()
	tx := core.Tx{Version: 1, Ins: []core.TxIn{{Prev: prevOp, PubKey: h.pub}}, Outs: outs}
	sigHash, err := tx.SigHash(0, h.pkh)
	if err != nil {
		h.t.Fatalf("SigHash: %v", err)
	}
	sig, err := core.SignHash(h.key, sigHash)
	if err != nil {
		h.t.Fatalf("SignHash: %v", err)
	}
	tx.Ins[0].Sig = sig
	return tx
}

func coinbaseOutpoint(b *core.Block) core.OutPoint {
	return core.OutPoint{TxID: b.Txs[0].TxID(), Index: 0}
}

func dumpUTXO(t *testing.T, c *Chain) map[string]string {
	t.Helper()
	m := map[string]string{}
	if err := c.db.View(func(btx *bolt.Tx) error {
		return btx.Bucket(bucketUTXO).ForEach(func(k, v []byte) error {
			m[hex.EncodeToString(k)] = hex.EncodeToString(v)
			return nil
		})
	}); err != nil {
		t.Fatalf("dumpUTXO: %v", err)
	}
	return m
}

// ── sequência válida ────────────────────────────────────────────────────────

func TestAcceptValidChainAndSpend(t *testing.T) {
	h := newTestChain(t)
	blocks := h.extend(11)

	if _, height, _ := h.c.Tip(); height != 11 {
		t.Fatalf("altura da ponta = %d, esperava 11", height)
	}

	// Gasta a coinbase do bloco 1 (madura: 12-1 ≥ 10) no bloco 12, com taxa.
	otherKey, _ := core.GenerateKey()
	otherPKH := core.HashPubKey(core.CompressPubKey(&otherKey.PublicKey))
	subsidy := h.p.BlockSubsidy(12)
	fee := uint64(1 * params.CoinUnit)
	spend := h.signedSpend(coinbaseOutpoint(blocks[0]), []core.TxOut{
		{Value: 30 * params.CoinUnit, PubKeyHash: otherPKH},
		{Value: 19 * params.CoinUnit, PubKeyHash: h.pkh},
	})
	prev, _, _ := h.c.Tip()
	b12 := h.buildBlock(prev, []core.Tx{spend}, subsidy+fee, h.pkh, 0)
	if err := h.c.AcceptBlock(b12); err != nil {
		t.Fatalf("bloco com gasto válido rejeitado: %v", err)
	}

	got, err := h.c.UTXOsByPKH(otherPKH)
	if err != nil || len(got) != 1 || got[0].Value != 30*params.CoinUnit {
		t.Fatalf("UTXO do destinatário = %+v (err %v), esperava 30 PANDA", got, err)
	}
	mine, err := h.c.UTXOsByPKH(h.pkh)
	if err != nil {
		t.Fatalf("UTXOsByPKH: %v", err)
	}
	var total uint64
	for _, u := range mine {
		total += u.Value
	}
	// blocos 2..11 (10 coinbases) + coinbase 12 (subsídio+taxa) + troco
	want := 10*50*params.CoinUnit + (subsidy + fee) + 19*params.CoinUnit
	if total != want {
		t.Fatalf("saldo do minerador = %d, esperava %d", total, want)
	}
}

// ── uma rejeição por regra de consenso ──────────────────────────────────────

func TestRejectBadPoW(t *testing.T) {
	h := newTestChain(t)
	prev, _, _ := h.c.Tip()
	b := h.buildBlock(prev, nil, h.p.BlockSubsidy(1), h.pkh, 0)
	// procura um nonce que FALHA o target (o perfil de teste passa ~50%)
	target := pow.CompactToTarget(b.Header.Bits)
	for n := uint64(0); ; n++ {
		b.Header.Nonce = n
		hash := pow.PowHash(b.Header.Bytes(), h.p)
		if new(big.Int).SetBytes(hash[:]).Cmp(target) > 0 {
			break
		}
	}
	if err := h.c.AcceptBlock(b); !errors.Is(err, pow.ErrHashAboveTarget) {
		t.Fatalf("err = %v, esperava ErrHashAboveTarget", err)
	}
}

func TestRejectUnexpectedBits(t *testing.T) {
	h := newTestChain(t)
	prev, _, _ := h.c.Tip()
	b := h.buildBlock(prev, nil, h.p.BlockSubsidy(1), h.pkh, 0)
	b.Header.Bits = 0x20700000 // válido no formato, mas não é o exigido
	if err := h.c.AcceptBlock(b); !errors.Is(err, ErrUnexpectedBits) {
		t.Fatalf("err = %v, esperava ErrUnexpectedBits", err)
	}
}

func TestRejectBadMerkleRoot(t *testing.T) {
	h := newTestChain(t)
	prev, _, _ := h.c.Tip()
	b := h.buildBlock(prev, nil, h.p.BlockSubsidy(1), h.pkh, 0)
	b.Header.MerkleRoot[0] ^= 0xff
	if err := h.c.AcceptBlock(b); !errors.Is(err, ErrBadMerkleRoot) {
		t.Fatalf("err = %v, esperava ErrBadMerkleRoot", err)
	}
}

func TestRejectTimestamps(t *testing.T) {
	h := newTestChain(t)
	prev, _, _ := h.c.Tip()

	old := h.buildBlock(prev, nil, h.p.BlockSubsidy(1), h.pkh, 0)
	old.Header.Timestamp = prev.Timestamp // == MTP do pai: tem que ser maior
	if err := h.c.AcceptBlock(old); !errors.Is(err, ErrTimestampTooOld) {
		t.Fatalf("err = %v, esperava ErrTimestampTooOld", err)
	}

	future := h.buildBlock(prev, nil, h.p.BlockSubsidy(1), h.pkh, 1)
	future.Header.Timestamp = h.c.now().Unix() + maxFutureDrift + 1
	if err := h.c.AcceptBlock(future); !errors.Is(err, ErrTimestampTooNew) {
		t.Fatalf("err = %v, esperava ErrTimestampTooNew", err)
	}
}

func TestRejectInflatedCoinbase(t *testing.T) {
	h := newTestChain(t)
	prev, _, _ := h.c.Tip()
	b := h.buildBlock(prev, nil, h.p.BlockSubsidy(1)+1, h.pkh, 0)
	if err := h.c.AcceptBlock(b); !errors.Is(err, ErrCoinbaseTooLarge) {
		t.Fatalf("err = %v, esperava ErrCoinbaseTooLarge", err)
	}
}

func TestRejectMissingUTXO(t *testing.T) {
	h := newTestChain(t)
	h.extend(11)
	ghost := core.OutPoint{TxID: [32]byte{0xaa}, Index: 3}
	spend := h.signedSpend(ghost, []core.TxOut{{Value: 1, PubKeyHash: h.pkh}})
	prev, _, _ := h.c.Tip()
	b := h.buildBlock(prev, []core.Tx{spend}, h.p.BlockSubsidy(12), h.pkh, 0)
	if err := h.c.AcceptBlock(b); !errors.Is(err, ErrMissingUTXO) {
		t.Fatalf("err = %v, esperava ErrMissingUTXO", err)
	}
}

func TestRejectDoubleSpend(t *testing.T) {
	h := newTestChain(t)
	blocks := h.extend(11)
	op := coinbaseOutpoint(blocks[0])

	// dentro do mesmo bloco: duas txs gastando o mesmo outpoint
	s1 := h.signedSpend(op, []core.TxOut{{Value: 50 * params.CoinUnit, PubKeyHash: h.pkh}})
	s2 := h.signedSpend(op, []core.TxOut{{Value: 25 * params.CoinUnit, PubKeyHash: h.pkh}})
	prev, _, _ := h.c.Tip()
	b := h.buildBlock(prev, []core.Tx{s1, s2}, h.p.BlockSubsidy(12), h.pkh, 0)
	if err := h.c.AcceptBlock(b); !errors.Is(err, ErrDoubleSpend) {
		t.Fatalf("err = %v, esperava ErrDoubleSpend", err)
	}

	// entre blocos: gastar de novo um outpoint já consumido
	prev, _, _ = h.c.Tip()
	ok := h.buildBlock(prev, []core.Tx{s1}, h.p.BlockSubsidy(12), h.pkh, 0)
	if err := h.c.AcceptBlock(ok); err != nil {
		t.Fatalf("gasto válido rejeitado: %v", err)
	}
	prev, _, _ = h.c.Tip()
	again := h.buildBlock(prev, []core.Tx{s2}, h.p.BlockSubsidy(13), h.pkh, 0)
	if err := h.c.AcceptBlock(again); !errors.Is(err, ErrMissingUTXO) {
		t.Fatalf("err = %v, esperava ErrMissingUTXO", err)
	}
}

func TestRejectImmatureCoinbase(t *testing.T) {
	h := newTestChain(t)
	blocks := h.extend(5)
	spend := h.signedSpend(coinbaseOutpoint(blocks[4]), []core.TxOut{{Value: 1 * params.CoinUnit, PubKeyHash: h.pkh}})
	prev, _, _ := h.c.Tip()
	b := h.buildBlock(prev, []core.Tx{spend}, h.p.BlockSubsidy(6), h.pkh, 0)
	if err := h.c.AcceptBlock(b); !errors.Is(err, ErrImmatureCoinbase) {
		t.Fatalf("err = %v, esperava ErrImmatureCoinbase", err)
	}
}

func TestRejectBadSignatureAndWrongKey(t *testing.T) {
	h := newTestChain(t)
	blocks := h.extend(11)
	op := coinbaseOutpoint(blocks[0])
	outs := []core.TxOut{{Value: 50 * params.CoinUnit, PubKeyHash: h.pkh}}

	// assinatura corrompida
	bad := h.signedSpend(op, outs)
	bad.Ins[0].Sig[4] ^= 0xff
	prev, _, _ := h.c.Tip()
	b := h.buildBlock(prev, []core.Tx{bad}, h.p.BlockSubsidy(12), h.pkh, 0)
	if err := h.c.AcceptBlock(b); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("err = %v, esperava ErrBadSignature", err)
	}

	// chave que não corresponde ao PubKeyHash do output gasto
	otherKey, _ := core.GenerateKey()
	otherPub := core.CompressPubKey(&otherKey.PublicKey)
	wrong := core.Tx{Version: 1, Ins: []core.TxIn{{Prev: op, PubKey: otherPub}}, Outs: outs}
	sigHash, _ := wrong.SigHash(0, h.pkh)
	wrong.Ins[0].Sig, _ = core.SignHash(otherKey, sigHash)
	prev, _, _ = h.c.Tip()
	b2 := h.buildBlock(prev, []core.Tx{wrong}, h.p.BlockSubsidy(12), h.pkh, 1)
	if err := h.c.AcceptBlock(b2); !errors.Is(err, ErrPubKeyMismatch) {
		t.Fatalf("err = %v, esperava ErrPubKeyMismatch", err)
	}
}

// ── duplicados, órfãos, reorg ───────────────────────────────────────────────

func TestDuplicateBlockIsNoop(t *testing.T) {
	h := newTestChain(t)
	blocks := h.extend(2)
	if err := h.c.AcceptBlock(blocks[0]); err != nil {
		t.Fatalf("duplicado deveria ser no-op, err = %v", err)
	}
	if _, height, _ := h.c.Tip(); height != 2 {
		t.Fatalf("altura mudou para %d após duplicado", height)
	}
}

func TestOrphanConnectsWhenParentArrives(t *testing.T) {
	h := newTestChain(t)
	prev, _, _ := h.c.Tip()
	b1 := h.buildBlock(prev, nil, h.p.BlockSubsidy(1), h.pkh, 0)
	b2 := h.buildBlock(b1.Header, nil, h.p.BlockSubsidy(2), h.pkh, 0)

	if err := h.c.AcceptBlock(b2); !errors.Is(err, ErrOrphan) {
		t.Fatalf("err = %v, esperava ErrOrphan", err)
	}
	if err := h.c.AcceptBlock(b1); err != nil {
		t.Fatalf("pai rejeitado: %v", err)
	}
	hdr, height, _ := h.c.Tip()
	if height != 2 || hdr.ID() != b2.Header.ID() {
		t.Fatalf("ponta = altura %d, esperava o órfão conectado na altura 2", height)
	}
}

func TestReorgToHeavierBranch(t *testing.T) {
	h := newTestChain(t)
	genesis, _, _ := h.c.Tip()

	// Ramo A (ativo): 3 blocos para h.pkh.
	h.extend(3)
	tipA, _, _ := h.c.Tip()

	// Ramo B: 4 blocos a partir do gênesis para outro minerador.
	otherKey, _ := core.GenerateKey()
	otherPKH := core.HashPubKey(core.CompressPubKey(&otherKey.PublicKey))
	var branchB []*core.Block
	prev := genesis
	for i := 0; i < 4; i++ {
		b := h.buildBlock(prev, nil, h.p.BlockSubsidy(prev.Height+1), otherPKH, 7)
		branchB = append(branchB, b)
		prev = b.Header
	}

	// b1..b3 empatam ou perdem em trabalho: ficam laterais, a ponta não muda.
	for i := 0; i < 3; i++ {
		if err := h.c.AcceptBlock(branchB[i]); err != nil {
			t.Fatalf("bloco lateral %d rejeitado: %v", i+1, err)
		}
	}
	if hdr, _, _ := h.c.Tip(); hdr.ID() != tipA.ID() {
		t.Fatalf("empate de trabalho não deveria trocar a ponta")
	}

	// b4 acumula mais trabalho: reorg para o ramo B.
	if err := h.c.AcceptBlock(branchB[3]); err != nil {
		t.Fatalf("bloco que dispara o reorg rejeitado: %v", err)
	}
	hdr, height, _ := h.c.Tip()
	if height != 4 || hdr.ID() != branchB[3].Header.ID() {
		t.Fatalf("ponta = altura %d, esperava o ramo B na altura 4", height)
	}

	// As coinbases do ramo A saíram do UTXO set...
	mine, err := h.c.UTXOsByPKH(h.pkh)
	if err != nil || len(mine) != 0 {
		t.Fatalf("UTXOs do ramo A ainda presentes após reorg: %v (err %v)", mine, err)
	}

	// ...e o UTXO set final é idêntico ao replay do ramo B numa chain zerada.
	replay, err := Open(filepath.Join(t.TempDir(), "replay.db"), h.p)
	if err != nil {
		t.Fatalf("Open replay: %v", err)
	}
	defer replay.Close()
	replay.now = h.c.now
	for _, b := range branchB {
		if err := replay.AcceptBlock(b); err != nil {
			t.Fatalf("replay AcceptBlock: %v", err)
		}
	}
	if got, want := dumpUTXO(t, h.c), dumpUTXO(t, replay); !reflect.DeepEqual(got, want) {
		t.Fatalf("UTXO set após reorg difere do replay do zero:\nreorg:  %v\nreplay: %v", got, want)
	}
}

func TestOpenRejectsWrongProfile(t *testing.T) {
	h := newTestChain(t)
	path := filepath.Join(t.TempDir(), "net.db")
	c1, err := Open(path, h.p)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	c1.Close()
	if _, err := Open(path, params.DevNet()); !errors.Is(err, ErrBadGenesis) {
		t.Fatalf("err = %v, esperava ErrBadGenesis ao abrir banco de outra rede", err)
	}
}
