package chain

import (
	"fmt"

	bolt "go.etcd.io/bbolt"

	"zhu/internal/core"
	"zhu/internal/params"
	"zhu/internal/pow"
)

// GenesisBlock constrói o bloco 0 de um perfil deterministicamente a partir
// da GenesisSpec — todo nó da rede monta exatamente estes bytes, e o ID
// resultante serve de identificador da rede no handshake P2P. A coinbase
// paga o subsídio inicial para o PubKeyHash zero, do qual ninguém tem a
// chave: o prêmio do gênesis é simbólico, como no Bitcoin.
func GenesisBlock(p params.Params) *core.Block {
	cb := core.NewCoinbase(0, p.BlockSubsidy(0), [20]byte{}, []byte(p.Genesis.Message))
	return &core.Block{
		Header: core.Header{
			Version:    1,
			Height:     0,
			MerkleRoot: core.MerkleRoot([][32]byte{cb.TxID()}),
			Timestamp:  p.Genesis.Timestamp,
			Bits:       p.Genesis.Bits,
			Nonce:      p.Genesis.Nonce,
		},
		Txs: []core.Tx{cb},
	}
}

// insertGenesis grava o bloco 0 num banco recém-criado. O gênesis é validado
// por igualdade exata de hash contra params (regra 9 da SPEC) — isento das
// demais regras, que pressupõem um pai. Exceção: num build com regras
// próprias (CustomBuild, via build.conf) não há hash hardcoded para conferir
// — o gênesis é derivado das regras e o ID resultante identifica a nova rede
// no handshake (que já o calcula dinamicamente).
func insertGenesis(btx *bolt.Tx, p params.Params) error {
	g := GenesisBlock(p)
	id := g.Header.ID()
	if !p.CustomBuild && (p.Genesis.Hash == ([32]byte{}) || id != p.Genesis.Hash) {
		return fmt.Errorf("%w: perfil %s (gênesis não minerado ou params divergentes)", ErrBadGenesis, p.Name)
	}
	if err := btx.Bucket(bucketBlocks).Put(id[:], g.Bytes()); err != nil {
		return err
	}
	entry := indexEntry{Height: 0, Status: statusActive, CumWork: pow.BlockWork(g.Header.Bits)}
	if err := putIndexEntry(btx, id, entry); err != nil {
		return err
	}
	if err := btx.Bucket(bucketHeight).Put(heightKey(0), id[:]); err != nil {
		return err
	}
	cbID := g.Txs[0].TxID()
	for oi := range g.Txs[0].Outs {
		e := utxoEntry{
			Value:    g.Txs[0].Outs[oi].Value,
			PKH:      g.Txs[0].Outs[oi].PubKeyHash,
			Height:   0,
			Coinbase: true,
		}
		if err := btx.Bucket(bucketUTXO).Put(outPointKey(core.OutPoint{TxID: cbID, Index: uint32(oi)}), e.encode()); err != nil {
			return err
		}
	}
	return btx.Bucket(bucketMeta).Put(keyTip, id[:])
}
