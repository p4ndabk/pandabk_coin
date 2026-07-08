// Package miner é o loop de mineração: monta um template (coinbase + txs do
// mempool por taxa), procura o nonce cujo Argon2id fica abaixo do alvo e
// submete o bloco à chain. O orçamento de recursos do projeto vive aqui:
// cada worker é 1 core + ~64 MiB, e o default é 1 worker — minerar não deve
// dominar a máquina de ninguém. Ver internal/miner/SPEC.md.
package miner

import (
	"context"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"zhu/internal/chain"
	"zhu/internal/core"
	"zhu/internal/mempool"
	"zhu/internal/params"
	"zhu/internal/pow"
)

// templateTTL: mesmo sem novo tip, o template é remontado periodicamente
// para atualizar o timestamp e captar txs novas do mempool.
const templateTTL = 30 * time.Second

// coinbaseReserve: folga de bytes reservada no orçamento do template para o
// header + coinbase (o resto de MaxBlockSize vai para txs do mempool).
const coinbaseReserve = 1024

type Miner struct {
	c       *chain.Chain
	mp      *mempool.Mempool
	p       params.Params
	payTo   [20]byte
	workers int

	hashes atomic.Uint64
	cancel context.CancelFunc
	wg     sync.WaitGroup
	tipCh  chan struct{}

	rateMu     sync.Mutex
	rateCount  uint64
	rateSample time.Time
}

// New cria o miner pagando a coinbase para payTo. workers < 1 vira 1 — o
// chamador (node) valida e avisa sobre valores estranhos.
func New(c *chain.Chain, mp *mempool.Mempool, p params.Params, payTo [20]byte, workers int) *Miner {
	if workers < 1 {
		workers = 1
	}
	return &Miner{
		c:       c,
		mp:      mp,
		p:       p,
		payTo:   payTo,
		workers: workers,
		tipCh:   make(chan struct{}, 1),
	}
}

// Start sobe o loop de mineração. onBlock é chamado para cada bloco NOSSO
// aceito na chain (o node usa para anunciar via p2p).
func (mn *Miner) Start(ctx context.Context, onBlock func(*core.Block)) {
	ctx, mn.cancel = context.WithCancel(ctx)
	mn.rateSample = time.Now()
	mn.wg.Add(1)
	go mn.run(ctx, onBlock)
}

// Stop cancela os workers e espera todos drenarem (sem goroutine vazada).
func (mn *Miner) Stop() {
	if mn.cancel != nil {
		mn.cancel()
	}
	mn.wg.Wait()
}

// TipChanged avisa que a ponta mudou (bloco da rede aceito): o template
// atual virou trabalho perdido e é remontado imediatamente.
func (mn *Miner) TipChanged() {
	select {
	case mn.tipCh <- struct{}{}:
	default:
	}
}

// HashRate é o hashes/s aproximado desde a última consulta (para getinfo).
func (mn *Miner) HashRate() float64 {
	mn.rateMu.Lock()
	defer mn.rateMu.Unlock()
	now := time.Now()
	total := mn.hashes.Load()
	dt := now.Sub(mn.rateSample).Seconds()
	if dt <= 0 {
		return 0
	}
	rate := float64(total-mn.rateCount) / dt
	mn.rateCount = total
	mn.rateSample = now
	return rate
}

// run é o ciclo: template → workers procuram → (achou | tip mudou | TTL) →
// recomeça. Trabalho sobre tip velho é descartado na hora — insistir só
// geraria um bloco órfão.
func (mn *Miner) run(ctx context.Context, onBlock func(*core.Block)) {
	defer mn.wg.Done()
	for ctx.Err() == nil {
		prevTip, _, _ := mn.c.Tip()
		round, cancelRound := context.WithCancel(ctx)
		found := make(chan *core.Block, mn.workers)
		var rwg sync.WaitGroup
		buildFailed := false
		for i := 0; i < mn.workers; i++ {
			tmpl, err := mn.buildTemplate(byte(i))
			if err != nil {
				buildFailed = true
				break
			}
			rwg.Add(1)
			go mn.worker(round, &rwg, tmpl, found)
		}
		if buildFailed {
			cancelRound()
			rwg.Wait()
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
			}
			continue
		}

		var winner *core.Block
		tick := time.NewTicker(500 * time.Millisecond)
		roundStart := time.Now()
	wait:
		for {
			select {
			case <-ctx.Done():
				break wait
			case winner = <-found:
				break wait
			case <-mn.tipCh:
				break wait // rede achou um bloco: remonta já
			case <-tick.C:
				if tip, _, _ := mn.c.Tip(); tip.ID() != prevTip.ID() {
					break wait // tip mudou por outro caminho (reorg)
				}
				if time.Since(roundStart) > templateTTL {
					break wait // refresh de timestamp/mempool
				}
			}
		}
		tick.Stop()
		cancelRound()
		rwg.Wait()

		if winner != nil {
			// Corrida benigna: se a rede achou o mesmo height primeiro, o
			// AcceptBlock guarda como lateral ou rejeita — nunca crasha.
			if err := mn.c.AcceptBlock(winner); err == nil {
				mn.mp.RemoveConfirmed(winner)
				if onBlock != nil {
					onBlock(winner)
				}
			}
		}
	}
}

// buildTemplate monta o candidato sobre a ponta atual: bits do retarget via
// chain.NextBits (a MESMA conta da validação), txs por fee rate até o
// orçamento e coinbase = subsídio + taxas. extranonce diferencia a coinbase
// de cada worker (merkle roots distintas → espaços de busca disjuntos).
func (mn *Miner) buildTemplate(extranonce byte) (*core.Block, error) {
	prev, _, _ := mn.c.Tip()
	height := prev.Height + 1
	bits, err := mn.c.NextBits()
	if err != nil {
		return nil, err
	}
	txs, fees := mn.mp.Template(int(mn.p.MaxBlockSize) - coinbaseReserve)
	cb := core.NewCoinbase(height, mn.p.BlockSubsidy(height)+fees, mn.payTo, []byte{extranonce})
	all := make([]core.Tx, 0, 1+len(txs))
	all = append(all, cb)
	for _, tx := range txs {
		all = append(all, *tx)
	}
	txids := make([][32]byte, len(all))
	for i := range all {
		txids[i] = all[i].TxID()
	}
	ts := time.Now().Unix()
	if ts <= prev.Timestamp {
		ts = prev.Timestamp + 1 // relógio atrasado: MTP exige > mediana
	}
	return &core.Block{
		Header: core.Header{
			Version:    1,
			Height:     height,
			PrevHash:   prev.ID(),
			MerkleRoot: core.MerkleRoot(txids),
			Timestamp:  ts,
			Bits:       bits,
		},
		Txs: all,
	}, nil
}

// worker varre nonces sobre o próprio template até achar, ser cancelado ou
// esgotar (na prática o cancel vem primeiro).
func (mn *Miner) worker(ctx context.Context, wg *sync.WaitGroup, b *core.Block, found chan<- *core.Block) {
	defer wg.Done()
	target := pow.CompactToTarget(b.Header.Bits)
	if target == nil {
		return
	}
	hdr := b.Header
	for nonce := uint64(0); ctx.Err() == nil; nonce++ {
		hdr.Nonce = nonce
		h := pow.PowHash(hdr.Bytes(), mn.p)
		mn.hashes.Add(1)
		if new(big.Int).SetBytes(h[:]).Cmp(target) <= 0 {
			b.Header = hdr
			select {
			case found <- b:
			default:
			}
			return
		}
	}
}
