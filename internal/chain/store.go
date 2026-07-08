// Package chain é o coração do consenso ZHU: valida blocos contra todas as
// regras, persiste a cadeia e o UTXO set em bbolt e decide qual ramo é a
// cadeia verdadeira (fork choice por trabalho acumulado, com reorg seguro).
// Ver internal/chain/SPEC.md.
package chain

import (
	"encoding/binary"
	"fmt"
	"math/big"

	bolt "go.etcd.io/bbolt"

	"zhu/internal/core"
)

// Buckets bbolt (SPEC.md, "Modelo de dados"). blocks e blockIndex guardam
// todo bloco já visto (ativo, lateral ou inválido); heightIndex e utxo
// descrevem só a cadeia ativa; undo permite desconectar um bloco num reorg
// sem reprocessar a cadeia inteira.
var (
	bucketBlocks = []byte("blocks")
	bucketIndex  = []byte("blockIndex")
	bucketHeight = []byte("heightIndex")
	bucketUTXO   = []byte("utxo")
	bucketUndo   = []byte("undo")
	bucketMeta   = []byte("meta")
)

var allBuckets = [][]byte{bucketBlocks, bucketIndex, bucketHeight, bucketUTXO, bucketUndo, bucketMeta}

var (
	keyTip      = []byte("tip")
	keyAddrBook = []byte("addrbook")
)

// Status de um bloco no blockIndex: ativo (na cadeia principal), lateral
// (ramo que perde em trabalho — pode voltar num reorg) ou inválido (falhou a
// validação completa; nunca é reconsiderado).
const (
	statusActive  byte = 1
	statusSide    byte = 2
	statusInvalid byte = 3
)

type indexEntry struct {
	Height  uint64
	Status  byte
	CumWork *big.Int
}

func (e indexEntry) encode() []byte {
	work := e.CumWork.Bytes()
	b := make([]byte, 0, 9+len(work))
	b = binary.BigEndian.AppendUint64(b, e.Height)
	b = append(b, e.Status)
	return append(b, work...)
}

func decodeIndexEntry(b []byte) (indexEntry, error) {
	if len(b) < 9 {
		return indexEntry{}, fmt.Errorf("chain: indexEntry truncado (%d bytes)", len(b))
	}
	return indexEntry{
		Height:  binary.BigEndian.Uint64(b[:8]),
		Status:  b[8],
		CumWork: new(big.Int).SetBytes(b[9:]),
	}, nil
}

// utxoEntry é o valor do bucket utxo: um output ainda não gasto, a altura em
// que nasceu e se veio de coinbase (para a regra de maturidade).
type utxoEntry struct {
	Value    uint64
	PKH      [20]byte
	Height   uint64
	Coinbase bool
}

const utxoEntrySize = 8 + 20 + 8 + 1

func (e utxoEntry) encode() []byte {
	b := make([]byte, 0, utxoEntrySize)
	b = binary.BigEndian.AppendUint64(b, e.Value)
	b = append(b, e.PKH[:]...)
	b = binary.BigEndian.AppendUint64(b, e.Height)
	flag := byte(0)
	if e.Coinbase {
		flag = 1
	}
	return append(b, flag)
}

func decodeUTXOEntry(b []byte) (utxoEntry, error) {
	if len(b) != utxoEntrySize {
		return utxoEntry{}, fmt.Errorf("chain: utxoEntry com %d bytes", len(b))
	}
	var e utxoEntry
	e.Value = binary.BigEndian.Uint64(b[:8])
	copy(e.PKH[:], b[8:28])
	e.Height = binary.BigEndian.Uint64(b[28:36])
	e.Coinbase = b[36] == 1
	return e, nil
}

// spentUTXO é uma entrada do undo set de um bloco: o outpoint gasto e o UTXO
// que ele destravava, para restaurá-lo se o bloco for desconectado.
type spentUTXO struct {
	Prev  core.OutPoint
	Entry utxoEntry
}

const spentSize = 36 + utxoEntrySize

func encodeUndo(spent []spentUTXO) []byte {
	b := make([]byte, 0, 4+len(spent)*spentSize)
	b = binary.BigEndian.AppendUint32(b, uint32(len(spent)))
	for _, s := range spent {
		b = append(b, outPointKey(s.Prev)...)
		b = append(b, s.Entry.encode()...)
	}
	return b
}

func decodeUndo(b []byte) ([]spentUTXO, error) {
	if len(b) < 4 {
		return nil, fmt.Errorf("chain: undo truncado")
	}
	n := int(binary.BigEndian.Uint32(b[:4]))
	if len(b)-4 != n*spentSize {
		return nil, fmt.Errorf("chain: undo com tamanho inconsistente")
	}
	out := make([]spentUTXO, 0, n)
	for i := 0; i < n; i++ {
		off := 4 + i*spentSize
		var s spentUTXO
		copy(s.Prev.TxID[:], b[off:off+32])
		s.Prev.Index = binary.BigEndian.Uint32(b[off+32 : off+36])
		e, err := decodeUTXOEntry(b[off+36 : off+spentSize])
		if err != nil {
			return nil, err
		}
		s.Entry = e
		out = append(out, s)
	}
	return out, nil
}

func outPointKey(op core.OutPoint) []byte {
	k := make([]byte, 36)
	copy(k, op.TxID[:])
	binary.BigEndian.PutUint32(k[32:], op.Index)
	return k
}

func heightKey(h uint64) []byte {
	k := make([]byte, 8)
	binary.BigEndian.PutUint64(k, h)
	return k
}

// ── helpers sobre uma transação bbolt aberta ────────────────────────────────

func getIndexEntry(btx *bolt.Tx, id [32]byte) (indexEntry, bool, error) {
	raw := btx.Bucket(bucketIndex).Get(id[:])
	if raw == nil {
		return indexEntry{}, false, nil
	}
	e, err := decodeIndexEntry(raw)
	return e, err == nil, err
}

func putIndexEntry(btx *bolt.Tx, id [32]byte, e indexEntry) error {
	return btx.Bucket(bucketIndex).Put(id[:], e.encode())
}

func getBlock(btx *bolt.Tx, id [32]byte) (*core.Block, []byte, error) {
	raw := btx.Bucket(bucketBlocks).Get(id[:])
	if raw == nil {
		return nil, nil, ErrBlockNotFound
	}
	blk, err := core.DecodeBlock(raw)
	if err != nil {
		return nil, nil, err
	}
	return &blk, raw, nil
}

// readHeader decodifica só os 96 bytes de header de um bloco guardado —
// barato, usado nas caminhadas de ancestrais (MTP, retarget, fork walk).
func readHeader(btx *bolt.Tx, id [32]byte) (core.Header, error) {
	raw := btx.Bucket(bucketBlocks).Get(id[:])
	if len(raw) < core.HeaderSize {
		return core.Header{}, ErrBlockNotFound
	}
	return core.DecodeHeader(raw[:core.HeaderSize])
}

func tipState(btx *bolt.Tx) ([32]byte, indexEntry, error) {
	var id [32]byte
	raw := btx.Bucket(bucketMeta).Get(keyTip)
	if len(raw) != 32 {
		return id, indexEntry{}, fmt.Errorf("chain: meta sem tip")
	}
	copy(id[:], raw)
	e, ok, err := getIndexEntry(btx, id)
	if err != nil {
		return id, indexEntry{}, err
	}
	if !ok {
		return id, indexEntry{}, fmt.Errorf("chain: tip %x sem entrada no índice", id[:8])
	}
	return id, e, nil
}
