package core

import (
	"bytes"
	"testing"
)

func sampleTx() Tx {
	var txid [32]byte
	copy(txid[:], bytes.Repeat([]byte{0xab}, 32))
	var pkh [20]byte
	copy(pkh[:], bytes.Repeat([]byte{0xcd}, 20))
	return Tx{
		Version: 1,
		Ins: []TxIn{{
			Prev:   OutPoint{TxID: txid, Index: 7},
			Sig:    []byte{1, 2, 3},
			PubKey: bytes.Repeat([]byte{4}, PubKeySize),
		}},
		Outs: []TxOut{
			{Value: 5_000_000_000, PubKeyHash: pkh},
			{Value: 42, PubKeyHash: pkh},
		},
	}
}

func sampleHeader() Header {
	var prev, merkle [32]byte
	prev[0], merkle[31] = 0x11, 0x22
	return Header{
		Version:    1,
		Height:     123,
		PrevHash:   prev,
		MerkleRoot: merkle,
		Timestamp:  1783036800,
		Bits:       0x20010000,
		Nonce:      987654321,
	}
}

func TestHeaderRoundTripAndSize(t *testing.T) {
	h := sampleHeader()
	b := h.Bytes()
	if len(b) != HeaderSize {
		t.Fatalf("header serializado tem %d bytes, want %d", len(b), HeaderSize)
	}
	got, err := DecodeHeader(b)
	if err != nil {
		t.Fatal(err)
	}
	if got != h {
		t.Fatalf("round-trip divergiu: %+v != %+v", got, h)
	}
	if _, err := DecodeHeader(b[:HeaderSize-1]); err == nil {
		t.Fatal("header truncado deveria falhar")
	}
}

func TestTxRoundTrip(t *testing.T) {
	tx := sampleTx()
	got, err := DecodeTx(tx.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), tx.Bytes()) {
		t.Fatal("round-trip da tx divergiu")
	}
	if got.TxID() != tx.TxID() {
		t.Fatal("txid mudou no round-trip")
	}
}

func TestBlockRoundTrip(t *testing.T) {
	cb := NewCoinbase(1, 50_0000_0000, [20]byte{9}, []byte("extra"))
	tx := sampleTx()
	blk := Block{Header: sampleHeader(), Txs: []Tx{cb, tx}}
	got, err := DecodeBlock(blk.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), blk.Bytes()) {
		t.Fatal("round-trip do bloco divergiu")
	}
}

func TestDecodeRejectsMalformed(t *testing.T) {
	tx := sampleTx()
	b := tx.Bytes()

	if _, err := DecodeTx(append(b, 0x00)); err == nil {
		t.Fatal("bytes sobrando deveriam falhar")
	}
	if _, err := DecodeTx(b[:len(b)-1]); err == nil {
		t.Fatal("tx truncada deveria falhar")
	}

	// Comprimento de blob mentiroso (maior que os dados) não pode alocar nem
	// passar: corrompe o tamanho da Sig, que vem depois de version(4)+
	// nIns(4)+txid(32)+index(4).
	evil := bytes.Clone(b)
	evil[44], evil[45], evil[46], evil[47] = 0xff, 0xff, 0xff, 0xff
	if _, err := DecodeTx(evil); err == nil {
		t.Fatal("blob com comprimento mentiroso deveria falhar")
	}

	blk := Block{Header: sampleHeader(), Txs: []Tx{tx}}
	bb := blk.Bytes()
	if _, err := DecodeBlock(bb[:len(bb)-2]); err == nil {
		t.Fatal("bloco truncado deveria falhar")
	}
}
