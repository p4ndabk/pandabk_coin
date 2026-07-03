package mempool

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
	"path/filepath"
	"sync"
	"testing"

	"pandabk_coin/internal/chain"
	"pandabk_coin/internal/core"
	"pandabk_coin/internal/params"
	"pandabk_coin/internal/pow"
)

// Harness do M3: uma chain de verdade (perfil de teste) com blocos minerados
// para um dono, e o mempool apontado para ela — Add valida contra UTXOs
// reais, RemoveConfirmed/Readd reagem a blocos reais.
type harness struct {
	t   *testing.T
	c   *chain.Chain
	p   params.Params
	m   *Mempool
	key *ecdsa.PrivateKey
	pub []byte
	pkh [20]byte
	cbs []core.OutPoint // outpoint da coinbase de cada bloco minerado (1-based)
}

func newHarness(t *testing.T, blocks int) *harness {
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
	h := &harness{t: t, c: c, p: p, m: New(c, p), key: key, pub: pub, pkh: core.HashPubKey(pub)}
	for i := 0; i < blocks; i++ {
		h.mineBlock(nil, 0)
	}
	return h
}

// mineBlock estende a ponta com as txs dadas; extraFee soma à coinbase (tem
// que ser ≤ taxas reais das txs, senão a chain rejeita).
func (h *harness) mineBlock(txs []core.Tx, extraFee uint64) *core.Block {
	h.t.Helper()
	prev, _, _ := h.c.Tip()
	height := prev.Height + 1
	cb := core.NewCoinbase(height, h.p.BlockSubsidy(height)+extraFee, h.pkh, nil)
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
	target := pow.CompactToTarget(b.Header.Bits)
	for n := uint64(0); ; n++ {
		b.Header.Nonce = n
		hash := pow.PowHash(b.Header.Bytes(), h.p)
		if new(big.Int).SetBytes(hash[:]).Cmp(target) <= 0 {
			break
		}
	}
	if err := h.c.AcceptBlock(b); err != nil {
		h.t.Fatalf("AcceptBlock(altura %d): %v", height, err)
	}
	h.cbs = append(h.cbs, core.OutPoint{TxID: cb.TxID(), Index: 0})
	return b
}

// spend gasta um outpoint da harness deixando fee de taxa.
func (h *harness) spend(op core.OutPoint, prevValue, fee uint64, to [20]byte) core.Tx {
	h.t.Helper()
	tx := core.Tx{
		Version: 1,
		Ins:     []core.TxIn{{Prev: op, PubKey: h.pub}},
		Outs:    []core.TxOut{{Value: prevValue - fee, PubKeyHash: to}},
	}
	sigHash, err := tx.SigHash(0, h.pkh)
	if err != nil {
		h.t.Fatal(err)
	}
	tx.Ins[0].Sig, err = core.SignHash(h.key, sigHash)
	if err != nil {
		h.t.Fatal(err)
	}
	return tx
}

func TestAddValidAndTopByFeeRate(t *testing.T) {
	h := newHarness(t, 13) // coinbases 1..3 maduras
	subsidy := 50 * params.CoinUnit

	cheap := h.spend(h.cbs[0], subsidy, 1_000, [20]byte{1})
	rich := h.spend(h.cbs[1], subsidy, 500_000, [20]byte{2})
	if err := h.m.Add(&cheap); err != nil {
		t.Fatalf("Add(cheap): %v", err)
	}
	if err := h.m.Add(&rich); err != nil {
		t.Fatalf("Add(rich): %v", err)
	}
	if !h.m.Has(cheap.TxID()) || !h.m.Has(rich.TxID()) {
		t.Fatal("txs válidas deveriam estar no pool")
	}

	top := h.m.TopByFeeRate(1 << 20)
	if len(top) != 2 || top[0].TxID() != rich.TxID() {
		t.Fatalf("maior taxa deveria vir primeiro: %d txs", len(top))
	}
	// orçamento apertado: só cabe a primeira
	small := h.m.TopByFeeRate(len(rich.Bytes()))
	if len(small) != 1 || small[0].TxID() != rich.TxID() {
		t.Fatalf("orçamento de bytes deveria cortar para 1 tx: %d", len(small))
	}
}

func TestAddRejections(t *testing.T) {
	h := newHarness(t, 13)
	subsidy := 50 * params.CoinUnit

	cb := core.NewCoinbase(99, subsidy, h.pkh, nil)
	if err := h.m.Add(&cb); !errors.Is(err, ErrCoinbase) {
		t.Fatalf("coinbase: err = %v", err)
	}

	ok := h.spend(h.cbs[0], subsidy, 1_000, [20]byte{1})
	if err := h.m.Add(&ok); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := h.m.Add(&ok); !errors.Is(err, ErrKnown) {
		t.Fatalf("duplicada: err = %v", err)
	}

	// double-spend contra pendente: outra tx gastando a MESMA coinbase
	conflict := h.spend(h.cbs[0], subsidy, 2_000, [20]byte{3})
	if err := h.m.Add(&conflict); !errors.Is(err, chain.ErrDoubleSpend) {
		t.Fatalf("double-spend: err = %v", err)
	}

	ghost := h.spend(core.OutPoint{TxID: [32]byte{0xaa}}, subsidy, 1_000, [20]byte{1})
	if err := h.m.Add(&ghost); !errors.Is(err, chain.ErrMissingUTXO) {
		t.Fatalf("UTXO inexistente: err = %v", err)
	}

	immature := h.spend(h.cbs[12], subsidy, 1_000, [20]byte{1}) // coinbase do tip
	if err := h.m.Add(&immature); !errors.Is(err, chain.ErrImmatureCoinbase) {
		t.Fatalf("coinbase imatura: err = %v", err)
	}

	bad := h.spend(h.cbs[1], subsidy, 1_000, [20]byte{1})
	bad.Ins[0].Sig[4] ^= 0xff
	if err := h.m.Add(&bad); !errors.Is(err, chain.ErrBadSignature) {
		t.Fatalf("assinatura inválida: err = %v", err)
	}

	greedy := h.spend(h.cbs[1], subsidy, 1_000, [20]byte{1})
	greedy.Outs[0].Value = subsidy + 1 // cria moedas do nada
	sigHash, _ := greedy.SigHash(0, h.pkh)
	greedy.Ins[0].Sig, _ = core.SignHash(h.key, sigHash)
	if err := h.m.Add(&greedy); !errors.Is(err, chain.ErrOutputsExceedIns) {
		t.Fatalf("outputs > inputs: err = %v", err)
	}
}

func TestRemoveConfirmedAndConflicts(t *testing.T) {
	h := newHarness(t, 13)
	subsidy := 50 * params.CoinUnit

	pendA := h.spend(h.cbs[0], subsidy, 1_000, [20]byte{1})
	if err := h.m.Add(&pendA); err != nil {
		t.Fatal(err)
	}
	// o bloco confirma uma tx DIFERENTE gastando a mesma coinbase: pendA
	// virou double-spend e tem que sair junto
	winner := h.spend(h.cbs[0], subsidy, 2_000, [20]byte{9})
	blk := h.mineBlock([]core.Tx{winner}, 2_000)

	h.m.RemoveConfirmed(blk)
	if h.m.Has(pendA.TxID()) {
		t.Fatal("pendente conflitante deveria ter saído do pool")
	}
	if h.m.Len() != 0 {
		t.Fatalf("pool deveria estar vazio, tem %d", h.m.Len())
	}
}

func TestReaddRevalidates(t *testing.T) {
	h := newHarness(t, 13)
	subsidy := 50 * params.CoinUnit

	txA := h.spend(h.cbs[0], subsidy, 1_000, [20]byte{1})
	txB := h.spend(h.cbs[1], subsidy, 1_000, [20]byte{2})

	// txB ainda é válida → volta; txA foi invalidada por um bloco que gastou
	// a mesma coinbase → descartada em silêncio.
	spender := h.spend(h.cbs[0], subsidy, 3_000, [20]byte{7})
	h.mineBlock([]core.Tx{spender}, 3_000)

	h.m.Readd([]*core.Tx{&txA, &txB})
	if h.m.Has(txA.TxID()) {
		t.Fatal("txA gasta no novo ramo não deveria voltar")
	}
	if !h.m.Has(txB.TxID()) {
		t.Fatal("txB ainda válida deveria voltar ao pool")
	}
}

func TestEvictLowestFeeRateWhenFull(t *testing.T) {
	h := newHarness(t, 14) // coinbases 1..4 maduras
	subsidy := 50 * params.CoinUnit
	h.m.maxSize = 2

	low := h.spend(h.cbs[0], subsidy, 1_000, [20]byte{1})
	mid := h.spend(h.cbs[1], subsidy, 10_000, [20]byte{2})
	high := h.spend(h.cbs[2], subsidy, 100_000, [20]byte{3})
	if err := h.m.Add(&low); err != nil {
		t.Fatal(err)
	}
	if err := h.m.Add(&mid); err != nil {
		t.Fatal(err)
	}
	if err := h.m.Add(&high); err != nil {
		t.Fatalf("tx de taxa maior deveria entrar com evict: %v", err)
	}
	if h.m.Has(low.TxID()) || h.m.Len() != 2 {
		t.Fatalf("a de menor taxa deveria ter sido evictada (len %d)", h.m.Len())
	}
	// pool cheio e taxa menor que todas → rejeitada (e o outpoint liberado
	// pelo evict de low pode ser reusado)
	verylow := h.spend(h.cbs[0], subsidy, 500, [20]byte{4})
	if err := h.m.Add(&verylow); !errors.Is(err, ErrPoolFull) {
		t.Fatalf("err = %v, esperava ErrPoolFull", err)
	}
}

func TestConcurrentAdd(t *testing.T) {
	h := newHarness(t, 14)
	subsidy := 50 * params.CoinUnit
	txs := []core.Tx{
		h.spend(h.cbs[0], subsidy, 1_000, [20]byte{1}),
		h.spend(h.cbs[1], subsidy, 2_000, [20]byte{2}),
		h.spend(h.cbs[2], subsidy, 3_000, [20]byte{3}),
	}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range txs {
				_ = h.m.Add(&txs[i])
				h.m.Has(txs[i].TxID())
				h.m.TopByFeeRate(1 << 20)
			}
		}()
	}
	wg.Wait()
	if h.m.Len() != 3 {
		t.Fatalf("pool deveria ter as 3 txs, tem %d", h.m.Len())
	}
}
