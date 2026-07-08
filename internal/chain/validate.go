package chain

import (
	"encoding/binary"
	"errors"
	"math/big"
	"sort"

	bolt "go.etcd.io/bbolt"

	"zhu/internal/core"
	"zhu/internal/params"
	"zhu/internal/pow"
)

// Erros sentinela — um por regra de consenso, para que cada rejeição seja
// testável e o p2p possa distinguir "bloco inválido" (banir peer) de "bloco
// órfão" (pedir o pai).
var (
	ErrBlockTooBig       = errors.New("chain: bloco maior que MaxBlockSize")
	ErrNoCoinbase        = errors.New("chain: primeira tx não é coinbase (ou bloco sem txs)")
	ErrMultipleCoinbase  = errors.New("chain: mais de uma coinbase no bloco")
	ErrBadTxStructure    = errors.New("chain: transação sem inputs ou sem outputs")
	ErrBadCoinbaseHeight = errors.New("chain: altura embutida na coinbase não bate com o header")
	ErrBadMerkleRoot     = errors.New("chain: merkle root não bate com as transações")
	ErrBadGenesis        = errors.New("chain: bloco gênesis não confere com params")
	ErrBadHeight         = errors.New("chain: altura não é a do pai + 1")
	ErrUnexpectedBits    = errors.New("chain: nBits diferente do exigido pelo retarget")
	ErrTimestampTooOld   = errors.New("chain: timestamp não é maior que a mediana dos últimos 11 blocos")
	ErrTimestampTooNew   = errors.New("chain: timestamp mais de 2 minutos no futuro")
	ErrMissingUTXO       = errors.New("chain: input referencia UTXO inexistente ou já gasto")
	ErrDoubleSpend       = errors.New("chain: mesmo UTXO gasto duas vezes no bloco")
	ErrImmatureCoinbase  = errors.New("chain: coinbase gasta antes da maturidade")
	ErrPubKeyMismatch    = errors.New("chain: pubkey do input não corresponde ao PubKeyHash do output gasto")
	ErrBadSignature      = errors.New("chain: assinatura inválida")
	ErrValueOverflow     = errors.New("chain: soma de valores estoura o teto de emissão")
	ErrOutputsExceedIns  = errors.New("chain: outputs somam mais que os inputs")
	ErrCoinbaseTooLarge  = errors.New("chain: coinbase acima de subsídio + taxas")
	ErrKnownInvalid      = errors.New("chain: bloco (ou ancestral) já marcado como inválido")
	ErrOrphan            = errors.New("chain: bloco órfão — pai desconhecido (guardado no pool)")
	ErrBlockNotFound     = errors.New("chain: bloco não encontrado")
)

// maxFutureDrift é a tolerância de relógio: bloco com timestamp além de
// agora+2min é rejeitado (regra 1 da SPEC).
const maxFutureDrift = 120

// checkSanity valida tudo que não depende de contexto (as checagens mais
// baratas vêm primeiro; nenhuma toca o banco): tamanho, estrutura das txs,
// coinbase única com a altura embutida, merkle root.
func checkSanity(b *core.Block, raw []byte, p params.Params) error {
	if uint32(len(raw)) > p.MaxBlockSize {
		return ErrBlockTooBig
	}
	if len(b.Txs) == 0 || !b.Txs[0].IsCoinbase() {
		return ErrNoCoinbase
	}
	// A altura embutida na coinbase (estilo BIP34) garante txid único por
	// bloco — sem isso, duas coinbases idênticas colidiriam no UTXO set.
	cbTag := b.Txs[0].Ins[0].PubKey
	if len(cbTag) < 8 || binary.BigEndian.Uint64(cbTag[:8]) != b.Header.Height {
		return ErrBadCoinbaseHeight
	}
	txids := make([][32]byte, len(b.Txs))
	txids[0] = b.Txs[0].TxID()
	for i := 1; i < len(b.Txs); i++ {
		tx := &b.Txs[i]
		if tx.IsCoinbase() {
			return ErrMultipleCoinbase
		}
		if len(tx.Ins) == 0 || len(tx.Outs) == 0 {
			return ErrBadTxStructure
		}
		txids[i] = tx.TxID()
	}
	if core.MerkleRoot(txids) != b.Header.MerkleRoot {
		return ErrBadMerkleRoot
	}
	return nil
}

// checkHeaderContext valida o header contra seus ancestrais (que já precisam
// estar no banco): encadeamento de altura, nBits exigido pelo retarget e a
// janela de timestamp (MTP-11 < ts ≤ agora+2min). Vale para qualquer ramo,
// não só a cadeia ativa — a caminhada segue PrevHash, não o heightIndex.
func checkHeaderContext(btx *bolt.Tx, hdr *core.Header, p params.Params, nowUnix int64) error {
	prev, err := readHeader(btx, hdr.PrevHash)
	if err != nil {
		return err
	}
	if hdr.Height != prev.Height+1 {
		return ErrBadHeight
	}
	bits, err := expectedBits(btx, prev, p)
	if err != nil {
		return err
	}
	if hdr.Bits != bits {
		return ErrUnexpectedBits
	}
	if hdr.Timestamp <= medianTimePast(btx, prev) {
		return ErrTimestampTooOld
	}
	if hdr.Timestamp > nowUnix+maxFutureDrift {
		return ErrTimestampTooNew
	}
	return nil
}

// expectedBits é o nBits que um bloco filho de prev DEVE declarar: fora da
// fronteira de retarget, o mesmo do pai; na fronteira, o NextBits calculado
// sobre a janela anterior — determinístico, todo nó chega no mesmo valor.
func expectedBits(btx *bolt.Tx, prev core.Header, p params.Params) (uint32, error) {
	height := prev.Height + 1
	if p.RetargetInterval == 0 || height%p.RetargetInterval != 0 {
		return prev.Bits, nil
	}
	first := prev
	for i := uint64(1); i < p.RetargetInterval; i++ {
		h, err := readHeader(btx, first.PrevHash)
		if err != nil {
			return 0, err
		}
		first = h
	}
	return pow.NextBits(first.Timestamp, prev.Timestamp, prev.Bits, p), nil
}

// medianTimePast é a mediana dos timestamps dos últimos (até) 11 blocos
// terminando em prev — a régua anti-manipulação de relógio do Bitcoin.
func medianTimePast(btx *bolt.Tx, prev core.Header) int64 {
	ts := []int64{prev.Timestamp}
	cur := prev
	for len(ts) < 11 && cur.Height > 0 {
		h, err := readHeader(btx, cur.PrevHash)
		if err != nil {
			break
		}
		ts = append(ts, h.Timestamp)
		cur = h
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i] < ts[j] })
	return ts[len(ts)/2]
}

// connectBlock faz a validação completa das transações contra o UTXO set e,
// se tudo passar, aplica o bloco (gasta/cria UTXOs, grava undo, avança o
// heightIndex e a ponta) — tudo dentro da transação bbolt do chamador, então
// qualquer erro desfaz o bloco inteiro. O PoW (Argon2id, a única checagem
// cara) fica por último: só paga quem passou em todo o resto. skipPow evita
// recomputá-lo no reconnect de um reorg (já foi checado quando o bloco
// lateral foi guardado).
func connectBlock(btx *bolt.Tx, b *core.Block, raw []byte, id [32]byte, cum *big.Int, p params.Params, skipPow bool) error {
	utxoB := btx.Bucket(bucketUTXO)
	height := b.Header.Height
	maxMoney := p.MaxSupply()

	created := map[core.OutPoint]utxoEntry{} // outputs nascidos neste bloco
	spent := map[core.OutPoint]bool{}
	var undo []spentUTXO // só UTXOs que existiam no bucket antes do bloco
	var fees uint64

	// Txs normais primeiro (para somar as taxas); coinbase conferida depois.
	// A ordem dentro do bloco importa: uma tx só pode gastar outputs de txs
	// anteriores a ela (ou do UTXO set) — igual ao Bitcoin.
	for ti := 1; ti < len(b.Txs); ti++ {
		tx := &b.Txs[ti]
		var sumIn uint64
		for i := range tx.Ins {
			in := &tx.Ins[i]
			op := in.Prev
			if spent[op] {
				return ErrDoubleSpend
			}
			var entry utxoEntry
			if ce, ok := created[op]; ok {
				entry = ce
				delete(created, op) // nasceu e morreu no mesmo bloco: sem undo
			} else {
				rawE := utxoB.Get(outPointKey(op))
				if rawE == nil {
					return ErrMissingUTXO
				}
				var err error
				entry, err = decodeUTXOEntry(rawE)
				if err != nil {
					return err
				}
				undo = append(undo, spentUTXO{Prev: op, Entry: entry})
			}
			if entry.Coinbase && height-entry.Height < p.CoinbaseMaturity {
				return ErrImmatureCoinbase
			}
			if core.HashPubKey(in.PubKey) != entry.PKH {
				return ErrPubKeyMismatch
			}
			sigHash, err := tx.SigHash(i, entry.PKH)
			if err != nil {
				return err
			}
			if !core.VerifySignature(in.PubKey, sigHash, in.Sig) {
				return ErrBadSignature
			}
			spent[op] = true
			sumIn += entry.Value
			if sumIn > maxMoney {
				return ErrValueOverflow
			}
		}
		sumOut, err := txOutputSum(tx, maxMoney)
		if err != nil {
			return err
		}
		if sumOut > sumIn {
			return ErrOutputsExceedIns
		}
		fees += sumIn - sumOut
		if fees > maxMoney {
			return ErrValueOverflow
		}
		txid := tx.TxID()
		for oi := range tx.Outs {
			created[core.OutPoint{TxID: txid, Index: uint32(oi)}] = utxoEntry{
				Value:  tx.Outs[oi].Value,
				PKH:    tx.Outs[oi].PubKeyHash,
				Height: height,
			}
		}
	}

	// Coinbase: a única tx que cria moedas, limitada a subsídio + taxas.
	cb := &b.Txs[0]
	cbOut, err := txOutputSum(cb, maxMoney)
	if err != nil {
		return err
	}
	if cbOut > p.BlockSubsidy(height)+fees {
		return ErrCoinbaseTooLarge
	}
	cbID := cb.TxID()
	for oi := range cb.Outs {
		created[core.OutPoint{TxID: cbID, Index: uint32(oi)}] = utxoEntry{
			Value:    cb.Outs[oi].Value,
			PKH:      cb.Outs[oi].PubKeyHash,
			Height:   height,
			Coinbase: true,
		}
	}

	if !skipPow {
		if err := pow.CheckProofOfWork(&b.Header, p); err != nil {
			return err
		}
	}

	// Aplica: gasta, cria, grava undo, avança índice e ponta.
	for _, s := range undo {
		if err := utxoB.Delete(outPointKey(s.Prev)); err != nil {
			return err
		}
	}
	for op, e := range created {
		if err := utxoB.Put(outPointKey(op), e.encode()); err != nil {
			return err
		}
	}
	if err := btx.Bucket(bucketUndo).Put(id[:], encodeUndo(undo)); err != nil {
		return err
	}
	if err := btx.Bucket(bucketBlocks).Put(id[:], raw); err != nil {
		return err
	}
	if err := putIndexEntry(btx, id, indexEntry{Height: height, Status: statusActive, CumWork: cum}); err != nil {
		return err
	}
	if err := btx.Bucket(bucketHeight).Put(heightKey(height), id[:]); err != nil {
		return err
	}
	return btx.Bucket(bucketMeta).Put(keyTip, id[:])
}

// txOutputSum soma os outputs de uma tx com checagem explícita contra o teto
// de emissão — nenhum valor individual nem a soma podem passar de maxMoney.
func txOutputSum(tx *core.Tx, maxMoney uint64) (uint64, error) {
	var sum uint64
	for i := range tx.Outs {
		v := tx.Outs[i].Value
		if v > maxMoney {
			return 0, ErrValueOverflow
		}
		sum += v
		if sum > maxMoney {
			return 0, ErrValueOverflow
		}
	}
	return sum, nil
}
