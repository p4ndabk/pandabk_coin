// Package mempool é a sala de espera das transações: valida ao receber,
// guarda em memória ordenável por taxa e acompanha os eventos da chain
// (connect remove confirmadas e conflitantes; reorg devolve as desconectadas
// revalidando). Não sobrevive a restart — comportamento padrão de node, a
// rede re-propaga. Ver internal/mempool/SPEC.md.
package mempool

import (
	"errors"
	"sort"
	"sync"

	"pandabk_coin/internal/chain"
	"pandabk_coin/internal/core"
	"pandabk_coin/internal/params"
)

// MaxPoolSize limita o pool (evict da menor fee rate quando cheio) — teto de
// memória de um node caseiro, não regra de consenso.
const MaxPoolSize = 5000

var (
	ErrCoinbase = errors.New("mempool: coinbase não entra no pool (só nasce dentro de bloco)")
	ErrKnown    = errors.New("mempool: transação já está no pool")
	ErrPoolFull = errors.New("mempool: pool cheio e fee rate abaixo da menor presente")
)

type entry struct {
	tx      *core.Tx
	size    int
	value   uint64 // Σ outputs — o que a tx movimenta (sem a taxa)
	fee     uint64
	feeRate float64
	seq     uint64 // ordem de chegada: desempate FIFO na ordenação
}

// Mempool valida contra o UTXO set da chain e reserva outpoints para
// impedir double-spend entre transações ainda não confirmadas.
type Mempool struct {
	mu      sync.Mutex
	c       *chain.Chain
	p       params.Params
	pool    map[[32]byte]*entry
	spends  map[core.OutPoint][32]byte
	nextSeq uint64
	maxSize int // = MaxPoolSize; reduzível em teste
}

func New(c *chain.Chain, p params.Params) *Mempool {
	return &Mempool{
		c:       c,
		p:       p,
		pool:    make(map[[32]byte]*entry),
		spends:  make(map[core.OutPoint][32]byte),
		maxSize: MaxPoolSize,
	}
}

// Add valida a transação com as MESMAS regras que a chain aplicará no bloco
// (UTXOs existem, maturidade, dono e assinatura, Σout ≤ Σin) mais as regras
// próprias do pool (sem coinbase, sem gastar outpoint já reservado por outra
// pendente, sem encadear em tx não confirmada). Taxa = Σin − Σout.
func (m *Mempool) Add(tx *core.Tx) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if tx.IsCoinbase() {
		return ErrCoinbase
	}
	if len(tx.Ins) == 0 || len(tx.Outs) == 0 {
		return chain.ErrBadTxStructure
	}
	txid := tx.TxID()
	if _, ok := m.pool[txid]; ok {
		return ErrKnown
	}

	_, tipHeight, _ := m.c.Tip()
	maxMoney := m.p.MaxSupply()
	seen := make(map[core.OutPoint]bool, len(tx.Ins))
	var sumIn uint64
	for i := range tx.Ins {
		in := &tx.Ins[i]
		if seen[in.Prev] {
			return chain.ErrDoubleSpend
		}
		seen[in.Prev] = true
		if _, reserved := m.spends[in.Prev]; reserved {
			return chain.ErrDoubleSpend
		}
		u, ok, err := m.c.LookupUTXO(in.Prev)
		if err != nil {
			return err
		}
		if !ok {
			// inclui o caso "output de outra tx ainda no mempool": sem
			// encadeamento de não-confirmadas nesta versão (SPEC)
			return chain.ErrMissingUTXO
		}
		// maturidade contra a PRÓXIMA altura — o melhor caso de confirmação
		if u.Coinbase && tipHeight+1-u.Height < m.p.CoinbaseMaturity {
			return chain.ErrImmatureCoinbase
		}
		if core.HashPubKey(in.PubKey) != u.PKH {
			return chain.ErrPubKeyMismatch
		}
		sigHash, err := tx.SigHash(i, u.PKH)
		if err != nil {
			return err
		}
		if !core.VerifySignature(in.PubKey, sigHash, in.Sig) {
			return chain.ErrBadSignature
		}
		sumIn += u.Value
		if sumIn > maxMoney {
			return chain.ErrValueOverflow
		}
	}
	var sumOut uint64
	for i := range tx.Outs {
		v := tx.Outs[i].Value
		if v > maxMoney {
			return chain.ErrValueOverflow
		}
		sumOut += v
		if sumOut > maxMoney {
			return chain.ErrValueOverflow
		}
	}
	if sumOut > sumIn {
		return chain.ErrOutputsExceedIns
	}

	size := len(tx.Bytes())
	e := &entry{
		tx:      tx,
		size:    size,
		value:   sumOut,
		fee:     sumIn - sumOut,
		feeRate: float64(sumIn-sumOut) / float64(size),
		seq:     m.nextSeq,
	}

	if len(m.pool) >= m.maxSize {
		lowID, low := m.lowestFeeRate()
		if low == nil || e.feeRate <= low.feeRate {
			return ErrPoolFull
		}
		m.drop(lowID)
	}

	m.nextSeq++
	m.pool[txid] = e
	for i := range tx.Ins {
		m.spends[tx.Ins[i].Prev] = txid
	}
	return nil
}

func (m *Mempool) Has(txid [32]byte) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.pool[txid]
	return ok
}

func (m *Mempool) Get(txid [32]byte) (*core.Tx, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.pool[txid]
	if !ok {
		return nil, false
	}
	return e.tx, true
}

// Entry é a visão externa de uma transação pendente (para RPC/explorador).
type Entry struct {
	TxID    [32]byte
	Size    int
	Value   uint64 // Σ outputs (o que a tx movimenta, sem a taxa)
	Fee     uint64
	FeeRate float64
	Seq     uint64
}

// Entries devolve um snapshot das transações esperando bloco, da maior fee
// rate para a menor (a ordem em que um minerador as pegaria).
func (m *Mempool) Entries() []Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Entry, 0, len(m.pool))
	for txid, e := range m.pool {
		out = append(out, Entry{TxID: txid, Size: e.size, Value: e.value, Fee: e.fee, FeeRate: e.feeRate, Seq: e.seq})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FeeRate != out[j].FeeRate {
			return out[i].FeeRate > out[j].FeeRate
		}
		return out[i].Seq < out[j].Seq
	})
	return out
}

// AllTxs devolve um snapshot das transações pendentes em ordem de chegada —
// para persistir o mempool no shutdown e recarregá-lo no boot.
func (m *Mempool) AllTxs() []*core.Tx {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries := make([]*entry, 0, len(m.pool))
	for _, e := range m.pool {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].seq < entries[j].seq })
	out := make([]*core.Tx, len(entries))
	for i, e := range entries {
		out[i] = e.tx
	}
	return out
}

func (m *Mempool) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.pool)
}

// TopByFeeRate devolve transações em ordem de fee rate decrescente (empate:
// quem chegou primeiro) até encher o orçamento de bytes — é assim que o
// miner escolhe o que entra no template do próximo bloco. Uma tx que não
// cabe é pulada, as menores seguintes ainda podem caber.
func (m *Mempool) TopByFeeRate(maxBytes int) []*core.Tx {
	txs, _ := m.Template(maxBytes)
	return txs
}

// Template é o TopByFeeRate que o miner usa: as txs escolhidas E a soma das
// taxas delas — o valor que a coinbase pode adicionar ao subsídio.
func (m *Mempool) Template(maxBytes int) ([]*core.Tx, uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries := make([]*entry, 0, len(m.pool))
	for _, e := range m.pool {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].feeRate != entries[j].feeRate {
			return entries[i].feeRate > entries[j].feeRate
		}
		return entries[i].seq < entries[j].seq
	})
	var out []*core.Tx
	var fees uint64
	budget := maxBytes
	for _, e := range entries {
		if e.size > budget {
			continue
		}
		out = append(out, e.tx)
		fees += e.fee
		budget -= e.size
	}
	return out, fees
}

// Reserved diz se um outpoint já está comprometido por uma tx pendente —
// a wallet do node usa para não gastar duas vezes o próprio dinheiro
// enquanto a primeira tx ainda não confirmou.
func (m *Mempool) Reserved(op core.OutPoint) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.spends[op]
	return ok
}

// RemoveConfirmed sincroniza o pool com um bloco conectado: as transações
// incluídas saem, e qualquer pendente que gastava um outpoint consumido pelo
// bloco ficou inválida — sai também.
func (m *Mempool) RemoveConfirmed(b *core.Block) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for ti := range b.Txs {
		tx := &b.Txs[ti]
		m.drop(tx.TxID())
		if tx.IsCoinbase() {
			continue
		}
		for i := range tx.Ins {
			if victim, ok := m.spends[tx.Ins[i].Prev]; ok {
				m.drop(victim)
			}
		}
	}
}

// Readd devolve ao pool transações desconectadas por um reorg, passando pela
// validação completa de Add — o ramo novo pode tê-las confirmado ou
// invalidado; as que não passam são simplesmente descartadas.
func (m *Mempool) Readd(txs []*core.Tx) {
	for _, tx := range txs {
		_ = m.Add(tx)
	}
}

// drop remove uma entrada e libera suas reservas. Chamar com o lock.
func (m *Mempool) drop(txid [32]byte) {
	e, ok := m.pool[txid]
	if !ok {
		return
	}
	for i := range e.tx.Ins {
		if owner, ok := m.spends[e.tx.Ins[i].Prev]; ok && owner == txid {
			delete(m.spends, e.tx.Ins[i].Prev)
		}
	}
	delete(m.pool, txid)
}

// lowestFeeRate acha a candidata a evict. Chamar com o lock.
func (m *Mempool) lowestFeeRate() ([32]byte, *entry) {
	var lowID [32]byte
	var low *entry
	for id, e := range m.pool {
		if low == nil || e.feeRate < low.feeRate || (e.feeRate == low.feeRate && e.seq > low.seq) {
			lowID, low = id, e
		}
	}
	return lowID, low
}
