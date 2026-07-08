// Package core define os tipos fundamentais da Zhu (bloco, transação UTXO,
// merkle, endereço) e sua serialização binária canônica — a base
// determinística sobre a qual todo o consenso é calculado. Sem I/O, sem
// estado. Ver internal/core/SPEC.md.
package core

import "crypto/sha256"

// HeaderSize é o tamanho fixo do header serializado.
const HeaderSize = 96

type Header struct {
	Version    uint32
	Height     uint64
	PrevHash   [32]byte
	MerkleRoot [32]byte
	Timestamp  int64
	Bits       uint32
	Nonce      uint64
}

type Block struct {
	Header Header
	Txs    []Tx
}

// ID é o identificador do bloco: SHA-256d do header. Barato — é o hash usado
// para indexar e encadear. O hash caro de proof of work (Argon2id) vive em
// internal/pow e só é calculado na checagem de PoW.
func (h *Header) ID() [32]byte {
	return sha256d(h.Bytes())
}

func sha256d(b []byte) [32]byte {
	first := sha256.Sum256(b)
	return sha256.Sum256(first[:])
}
