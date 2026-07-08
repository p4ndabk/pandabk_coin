package wallet

import (
	"errors"
	"sort"

	"zhu/internal/core"
)

// UTXO é a visão mínima que o BuildTx precisa de um output não gasto do
// PRÓPRIO dono — quem chama (o RPC do node, M5) consulta a chain e converte.
// Definida aqui para a wallet não depender de chain (direção de dependências
// do PLAN.md).
type UTXO struct {
	Prev  core.OutPoint
	Value uint64
}

// DustLimit: troco abaixo disso não vira output (custaria mais taxa para
// gastar do que vale) — é incorporado à taxa do minerador.
const DustLimit = 1000

// approxSigSize superestima levemente a assinatura DER (70–72 bytes) para a
// taxa nunca ficar abaixo do prometido pelo feeRate.
const approxSigSize = 72

var (
	ErrInsufficientFunds = errors.New("wallet: fundos insuficientes para valor + taxa")
	ErrZeroAmount        = errors.New("wallet: valor precisa ser maior que zero")
)

// BuildTx monta e assina uma transação: seleciona UTXOs (largest-first, o
// jeito mais simples de minimizar o número de inputs), paga amount para to,
// devolve o troco para a própria wallet (se acima da poeira) e deixa
// feeRate × tamanho como taxa. Cada input é assinado com o sighash do
// SIGHASH_ALL simplificado do core.
func (w *Wallet) BuildTx(utxos []UTXO, to [20]byte, amount, feeRate uint64) (*core.Tx, error) {
	if amount == 0 {
		return nil, ErrZeroAmount
	}
	sorted := make([]UTXO, len(utxos))
	copy(sorted, utxos)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Value > sorted[j].Value })

	// Acumula inputs até cobrir amount + taxa — que cresce com cada input
	// adicionado, por isso o alvo é reavaliado a cada volta. A estimativa
	// assume 2 outputs (destino + troco): superestimar 28 bytes quando o
	// troco não existe só arredonda a taxa a favor do minerador.
	var selected []UTXO
	var total, fee uint64
	for {
		fee = feeRate * estimateSize(len(selected), 2)
		need := amount + fee
		if need < amount { // overflow: pedido impagável
			return nil, ErrInsufficientFunds
		}
		if total >= need {
			break
		}
		if len(selected) == len(sorted) {
			return nil, ErrInsufficientFunds
		}
		next := sorted[len(selected)]
		selected = append(selected, next)
		total += next.Value
	}

	change := total - amount - fee
	if change > 0 && change < DustLimit {
		change = 0 // vira taxa
	}

	tx := &core.Tx{Version: 1}
	for _, u := range selected {
		tx.Ins = append(tx.Ins, core.TxIn{Prev: u.Prev, PubKey: w.pub})
	}
	tx.Outs = append(tx.Outs, core.TxOut{Value: amount, PubKeyHash: to})
	if change > 0 {
		tx.Outs = append(tx.Outs, core.TxOut{Value: change, PubKeyHash: w.pkh})
	}

	// Assina por último: o sighash cobre todos os inputs e outputs, então a
	// tx precisa estar completa. Todos os UTXOs são desta wallet, logo o
	// PubKeyHash do output gasto é sempre o nosso.
	for i := range tx.Ins {
		sigHash, err := tx.SigHash(i, w.pkh)
		if err != nil {
			return nil, err
		}
		sig, err := core.SignHash(w.priv, sigHash)
		if err != nil {
			return nil, err
		}
		tx.Ins[i].Sig = sig
	}
	return tx, nil
}

// estimateSize é o tamanho serializado previsto (encoding canônico do core):
// version + contagens + inputs (outpoint 36, sig com prefixo 4+72, pubkey
// 4+33) e outputs de 28 bytes.
func estimateSize(nIns, nOuts int) uint64 {
	return uint64(4 + 4 + nIns*(36+4+approxSigSize+4+core.PubKeySize) + 4 + nOuts*28)
}
