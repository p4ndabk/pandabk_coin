package chain

import "pandabk_coin/internal/core"

// orphanCap limita o pool de órfãos: blocos cujo pai ainda não chegou ficam
// aqui aguardando (evict FIFO quando enche) — memória limitada mesmo se um
// peer malicioso despejar órfãos sem fim.
const orphanCap = 100

type orphanPool struct {
	blocks map[[32]byte]*core.Block
	byPrev map[[32]byte][][32]byte
	order  [][32]byte // ordem de chegada, para o evict FIFO
}

func newOrphanPool() orphanPool {
	return orphanPool{
		blocks: make(map[[32]byte]*core.Block),
		byPrev: make(map[[32]byte][][32]byte),
	}
}

func (o *orphanPool) add(b *core.Block) {
	id := b.Header.ID()
	if _, ok := o.blocks[id]; ok {
		return
	}
	for len(o.blocks) >= orphanCap && len(o.order) > 0 {
		oldest := o.order[0]
		o.order = o.order[1:]
		o.remove(oldest)
	}
	o.blocks[id] = b
	o.byPrev[b.Header.PrevHash] = append(o.byPrev[b.Header.PrevHash], id)
	o.order = append(o.order, id)
}

// takeChildren remove e devolve os órfãos que apontam para parent — chamado
// quando parent acaba de ser aceito, para reprocessá-los.
func (o *orphanPool) takeChildren(parent [32]byte) []*core.Block {
	ids := o.byPrev[parent]
	if len(ids) == 0 {
		return nil
	}
	out := make([]*core.Block, 0, len(ids))
	for _, id := range ids {
		if b, ok := o.blocks[id]; ok {
			out = append(out, b)
			delete(o.blocks, id)
		}
	}
	delete(o.byPrev, parent)
	return out
}

func (o *orphanPool) remove(id [32]byte) {
	b, ok := o.blocks[id]
	if !ok {
		return
	}
	delete(o.blocks, id)
	siblings := o.byPrev[b.Header.PrevHash]
	for i, sid := range siblings {
		if sid == id {
			o.byPrev[b.Header.PrevHash] = append(siblings[:i], siblings[i+1:]...)
			break
		}
	}
	if len(o.byPrev[b.Header.PrevHash]) == 0 {
		delete(o.byPrev, b.Header.PrevHash)
	}
}
