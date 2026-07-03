package chain

import (
	"fmt"

	bolt "go.etcd.io/bbolt"

	"pandabk_coin/internal/core"
)

// reorgTo troca a cadeia ativa pelo ramo que termina em newTip (que já
// provou ter mais trabalho acumulado): desconecta a ponta atual até o ponto
// de bifurcação restaurando os UTXOs via undo sets, e reconecta o ramo novo
// validando cada bloco POR COMPLETO (txs, assinaturas, coinbase — só o
// Argon2 é pulado, já checado quando o bloco lateral foi guardado). Tudo
// numa única transação bbolt: se qualquer bloco do ramo novo for podre, o
// rollback restaura a cadeia antiga intacta, e o culpado (e seus
// descendentes) são marcados como inválidos.
func (c *Chain) reorgTo(newTip [32]byte) error {
	var bad [][32]byte // blocos a marcar como inválidos se o reorg abortar
	err := c.db.Update(func(btx *bolt.Tx) error {
		// Caminho do ramo novo: anda de newTip para trás até encostar na
		// cadeia ativa (o ponto de bifurcação).
		var path [][32]byte
		cur := newTip
		var forkE indexEntry
		for {
			e, ok, err := getIndexEntry(btx, cur)
			if err != nil {
				return err
			}
			if !ok {
				return ErrBlockNotFound
			}
			if e.Status == statusActive {
				forkE = e
				break
			}
			if e.Status == statusInvalid {
				bad = path // tudo que depende de um inválido é inválido
				return ErrKnownInvalid
			}
			path = append(path, cur)
			hdr, err := readHeader(btx, cur)
			if err != nil {
				return err
			}
			cur = hdr.PrevHash
		}
		for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
			path[i], path[j] = path[j], path[i]
		}

		// Desconecta a cadeia ativa de cima para baixo até a bifurcação.
		_, tipE, err := tipState(btx)
		if err != nil {
			return err
		}
		for h := tipE.Height; h > forkE.Height; h-- {
			if err := disconnectBlock(btx, h); err != nil {
				return err
			}
		}
		if err := btx.Bucket(bucketMeta).Put(keyTip, cur[:]); err != nil {
			return err
		}

		// Reconecta o ramo novo do mais antigo ao mais novo.
		for k, bid := range path {
			blk, raw, err := getBlock(btx, bid)
			if err != nil {
				return err
			}
			e, ok, err := getIndexEntry(btx, bid)
			if err != nil || !ok {
				return fmt.Errorf("chain: bloco %x do ramo novo sem índice", bid[:8])
			}
			if err := connectBlock(btx, blk, raw, bid, e.CumWork, c.p, true); err != nil {
				bad = path[k:] // o podre e todos que constroem sobre ele
				return fmt.Errorf("chain: reorg abortado, bloco lateral inválido: %w", err)
			}
		}
		return nil
	})
	if len(bad) > 0 {
		c.markInvalid(bad)
	}
	return err
}

// disconnectBlock desfaz o bloco ativo da altura h: remove os outputs que
// ele criou, restaura via undo os que ele gastou e o rebaixa a lateral.
func disconnectBlock(btx *bolt.Tx, h uint64) error {
	hb := btx.Bucket(bucketHeight)
	idRaw := hb.Get(heightKey(h))
	if len(idRaw) != 32 {
		return fmt.Errorf("chain: heightIndex sem a altura %d", h)
	}
	var id [32]byte
	copy(id[:], idRaw)
	blk, _, err := getBlock(btx, id)
	if err != nil {
		return err
	}
	utxoB := btx.Bucket(bucketUTXO)
	for ti := range blk.Txs {
		txid := blk.Txs[ti].TxID()
		for oi := range blk.Txs[ti].Outs {
			if err := utxoB.Delete(outPointKey(core.OutPoint{TxID: txid, Index: uint32(oi)})); err != nil {
				return err
			}
		}
	}
	undoB := btx.Bucket(bucketUndo)
	if undoRaw := undoB.Get(id[:]); undoRaw != nil {
		spent, err := decodeUndo(undoRaw)
		if err != nil {
			return err
		}
		for _, s := range spent {
			if err := utxoB.Put(outPointKey(s.Prev), s.Entry.encode()); err != nil {
				return err
			}
		}
		if err := undoB.Delete(id[:]); err != nil {
			return err
		}
	}
	if err := hb.Delete(heightKey(h)); err != nil {
		return err
	}
	e, ok, err := getIndexEntry(btx, id)
	if err != nil || !ok {
		return fmt.Errorf("chain: bloco ativo %x sem índice", id[:8])
	}
	e.Status = statusSide
	return putIndexEntry(btx, id, e)
}

// markInvalid rebaixa blocos a inválidos em transação própria — chamado
// depois do rollback de um reorg abortado, para nunca revalidar o ramo.
func (c *Chain) markInvalid(ids [][32]byte) {
	_ = c.db.Update(func(btx *bolt.Tx) error {
		for _, id := range ids {
			e, ok, err := getIndexEntry(btx, id)
			if err != nil || !ok {
				continue
			}
			e.Status = statusInvalid
			if err := putIndexEntry(btx, id, e); err != nil {
				return err
			}
		}
		return nil
	})
}
