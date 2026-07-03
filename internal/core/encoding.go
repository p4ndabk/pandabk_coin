package core

import (
	"encoding/binary"
	"errors"
	"math"
)

// Serialização binária canônica: big-endian, contagens e comprimentos em
// uint32. Canônica = a mesma struct sempre produz os mesmos bytes, e o decode
// rejeita bytes sobrando — obrigatório para que hashes e assinaturas sejam
// determinísticos (gob/JSON não garantem isso).

var (
	ErrTruncated = errors.New("core: bytes truncados")
	ErrTrailing  = errors.New("core: bytes sobrando após decode")
	ErrOversized = errors.New("core: comprimento declarado maior que os dados")
)

func appendU32(b []byte, v uint32) []byte {
	return binary.BigEndian.AppendUint32(b, v)
}

func appendU64(b []byte, v uint64) []byte {
	return binary.BigEndian.AppendUint64(b, v)
}

func appendBlob(b, blob []byte) []byte {
	b = appendU32(b, uint32(len(blob)))
	return append(b, blob...)
}

type reader struct {
	b   []byte
	off int
	err error
}

func (r *reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || r.off+n > len(r.b) {
		r.err = ErrTruncated
		return nil
	}
	out := r.b[r.off : r.off+n]
	r.off += n
	return out
}

func (r *reader) u32() uint32 {
	b := r.take(4)
	if r.err != nil {
		return 0
	}
	return binary.BigEndian.Uint32(b)
}

func (r *reader) u64() uint64 {
	b := r.take(8)
	if r.err != nil {
		return 0
	}
	return binary.BigEndian.Uint64(b)
}

func (r *reader) hash() (h [32]byte) {
	copy(h[:], r.take(32))
	return h
}

// blob lê um comprimento u32 + bytes; o comprimento é validado contra o que
// resta no buffer, então entrada maliciosa não força alocação gigante.
func (r *reader) blob() []byte {
	n := r.u32()
	if r.err != nil {
		return nil
	}
	if int(n) > len(r.b)-r.off {
		r.err = ErrOversized
		return nil
	}
	if n == 0 {
		return nil
	}
	out := make([]byte, n)
	copy(out, r.take(int(n)))
	return out
}

func (r *reader) done() error {
	if r.err != nil {
		return r.err
	}
	if r.off != len(r.b) {
		return ErrTrailing
	}
	return nil
}

func (h *Header) Bytes() []byte {
	b := make([]byte, 0, HeaderSize)
	b = appendU32(b, h.Version)
	b = appendU64(b, h.Height)
	b = append(b, h.PrevHash[:]...)
	b = append(b, h.MerkleRoot[:]...)
	b = appendU64(b, uint64(h.Timestamp))
	b = appendU32(b, h.Bits)
	b = appendU64(b, h.Nonce)
	return b
}

func DecodeHeader(data []byte) (Header, error) {
	if len(data) != HeaderSize {
		return Header{}, ErrTruncated
	}
	r := &reader{b: data}
	h := Header{
		Version: r.u32(),
		Height:  r.u64(),
	}
	h.PrevHash = r.hash()
	h.MerkleRoot = r.hash()
	h.Timestamp = int64(r.u64())
	h.Bits = r.u32()
	h.Nonce = r.u64()
	return h, r.done()
}

func (t *Tx) Bytes() []byte {
	b := appendU32(nil, t.Version)
	b = appendU32(b, uint32(len(t.Ins)))
	for i := range t.Ins {
		in := &t.Ins[i]
		b = append(b, in.Prev.TxID[:]...)
		b = appendU32(b, in.Prev.Index)
		b = appendBlob(b, in.Sig)
		b = appendBlob(b, in.PubKey)
	}
	b = appendU32(b, uint32(len(t.Outs)))
	for i := range t.Outs {
		out := &t.Outs[i]
		b = appendU64(b, out.Value)
		b = append(b, out.PubKeyHash[:]...)
	}
	return b
}

func DecodeTx(data []byte) (Tx, error) {
	r := &reader{b: data}
	t := decodeTx(r)
	return t, r.done()
}

func decodeTx(r *reader) Tx {
	t := Tx{Version: r.u32()}
	nIns := r.u32()
	for i := uint32(0); i < nIns && r.err == nil; i++ {
		var in TxIn
		in.Prev.TxID = r.hash()
		in.Prev.Index = r.u32()
		in.Sig = r.blob()
		in.PubKey = r.blob()
		t.Ins = append(t.Ins, in)
	}
	nOuts := r.u32()
	for i := uint32(0); i < nOuts && r.err == nil; i++ {
		var out TxOut
		out.Value = r.u64()
		copy(out.PubKeyHash[:], r.take(20))
		t.Outs = append(t.Outs, out)
	}
	return t
}

func (b *Block) Bytes() []byte {
	buf := b.Header.Bytes()
	buf = appendU32(buf, uint32(len(b.Txs)))
	for i := range b.Txs {
		buf = appendBlob(buf, b.Txs[i].Bytes())
	}
	return buf
}

func DecodeBlock(data []byte) (Block, error) {
	if len(data) > math.MaxInt32 {
		return Block{}, ErrOversized
	}
	r := &reader{b: data}
	var blk Block
	hdr, err := DecodeHeader(r.take(HeaderSize))
	if err != nil {
		return Block{}, err
	}
	blk.Header = hdr
	nTxs := r.u32()
	for i := uint32(0); i < nTxs && r.err == nil; i++ {
		txBytes := r.blob()
		if r.err != nil {
			break
		}
		tx, err := DecodeTx(txBytes)
		if err != nil {
			return Block{}, err
		}
		blk.Txs = append(blk.Txs, tx)
	}
	return blk, r.done()
}
