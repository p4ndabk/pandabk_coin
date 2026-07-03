package core

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// CoinbaseIndex marca o OutPoint de uma coinbase (não referencia output real).
const CoinbaseIndex = 0xffffffff

var zeroHash [32]byte

type OutPoint struct {
	TxID  [32]byte
	Index uint32
}

type TxIn struct {
	Prev OutPoint
	// Sig é a assinatura ECDSA em ASN.1 DER sobre o SigHash do input.
	Sig []byte
	// PubKey é a chave pública comprimida (33 bytes) cujo hash deve bater
	// com o PubKeyHash do output gasto. Na coinbase carrega height +
	// extranonce (unicidade do txid, estilo BIP34).
	PubKey []byte
}

type TxOut struct {
	Value      uint64
	PubKeyHash [20]byte
}

type Tx struct {
	Version uint32
	Ins     []TxIn
	Outs    []TxOut
}

func (t *Tx) TxID() [32]byte {
	return sha256d(t.Bytes())
}

func (t *Tx) IsCoinbase() bool {
	return len(t.Ins) == 1 &&
		t.Ins[0].Prev.TxID == zeroHash &&
		t.Ins[0].Prev.Index == CoinbaseIndex
}

// NewCoinbase monta a transação que abre um bloco: cria value subunidades
// (subsídio + taxas) para pkh. Height e extra entram na PubKey do input para
// que cada coinbase tenha txid único.
func NewCoinbase(height uint64, value uint64, pkh [20]byte, extra []byte) Tx {
	tag := make([]byte, 8, 8+len(extra))
	binary.BigEndian.PutUint64(tag, height)
	tag = append(tag, extra...)
	return Tx{
		Version: 1,
		Ins: []TxIn{{
			Prev:   OutPoint{TxID: zeroHash, Index: CoinbaseIndex},
			PubKey: tag,
		}},
		Outs: []TxOut{{Value: value, PubKeyHash: pkh}},
	}
}

var ErrInputIndexOutOfRange = errors.New("core: índice de input fora da transação")

// SigHash é o resumo que o dono do input i assina (SIGHASH_ALL simplificado):
// SHA-256d da transação serializada com todos os campos Sig vazios e, no
// lugar da PubKey, o PubKeyHash do output gasto no input i (vazio nos
// demais). Compromete a assinatura com todos os inputs e outputs.
func (t *Tx) SigHash(i int, prevPKH [20]byte) ([32]byte, error) {
	if i < 0 || i >= len(t.Ins) {
		return [32]byte{}, ErrInputIndexOutOfRange
	}
	stripped := Tx{Version: t.Version, Outs: t.Outs, Ins: make([]TxIn, len(t.Ins))}
	for j := range t.Ins {
		stripped.Ins[j].Prev = t.Ins[j].Prev
		if j == i {
			stripped.Ins[j].PubKey = bytes.Clone(prevPKH[:])
		}
	}
	return sha256d(stripped.Bytes()), nil
}
