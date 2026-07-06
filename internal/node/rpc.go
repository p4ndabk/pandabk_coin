package node

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"pandabk_coin/internal/chain"
	"pandabk_coin/internal/core"
	"pandabk_coin/internal/params"
	"pandabk_coin/internal/pow"
	"pandabk_coin/internal/wallet"
)

// RPC JSON local: POST /rpc {"method": "...", "params": {...}} →
// {"result": ...} ou {"error": {"code", "message"}} — mesmo espírito de
// envelope do apierror, sem importá-lo (o node não usa Gin).

type rpcRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	Result any       `json:"result,omitempty"`
	Error  *rpcError `json:"error,omitempty"`
}

func (n *Node) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeRPC(w, http.StatusMethodNotAllowed, rpcResponse{Error: &rpcError{Code: "method_not_allowed", Message: "use POST"}})
		return
	}
	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRPC(w, http.StatusBadRequest, rpcResponse{Error: &rpcError{Code: "bad_request", Message: "JSON inválido"}})
		return
	}
	result, err := n.dispatch(req)
	if err != nil {
		writeRPC(w, http.StatusOK, rpcResponse{Error: &rpcError{Code: "error", Message: err.Error()}})
		return
	}
	writeRPC(w, http.StatusOK, rpcResponse{Result: result})
}

func writeRPC(w http.ResponseWriter, status int, resp rpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

func (n *Node) dispatch(req rpcRequest) (any, error) {
	switch req.Method {
	case "getinfo":
		return n.rpcGetInfo()
	case "getbalance":
		return n.rpcGetBalance(req.Params)
	case "getblock":
		return n.rpcGetBlock(req.Params)
	case "getstats":
		return n.rpcGetStats()
	case "getrecentblocks":
		return n.rpcGetRecentBlocks(req.Params)
	case "getmempool":
		return n.rpcGetMempool()
	case "getactivity":
		return n.rpcGetActivity(req.Params)
	case "getnewutxos":
		return n.rpcGetNewUTXOs(req.Params)
	case "sendrawtx":
		return n.rpcSendRawTx(req.Params)
	case "sendtoaddress":
		return n.rpcSendToAddress(req.Params)
	default:
		return nil, fmt.Errorf("método desconhecido: %q", req.Method)
	}
}

// ── getinfo ─────────────────────────────────────────────────────────────────

type infoResult struct {
	Profile     string  `json:"profile"`
	Height      uint64  `json:"height"`
	Tip         string  `json:"tip"`
	Work        string  `json:"work"`
	Bits        string  `json:"bits"`
	Difficulty  float64 `json:"difficulty"`
	SpacingSecs int64   `json:"target_spacing_seconds"`
	RewardPanda string  `json:"reward_panda"`
	NextHalving uint64  `json:"next_halving"`
	Peers       int     `json:"peers"`
	Mempool     int     `json:"mempool"`
	Mining      bool    `json:"mining"`
	HashRate    float64 `json:"hashrate"`
	Address     string  `json:"address,omitempty"`
}

func (n *Node) rpcGetInfo() (any, error) {
	tip, height, work := n.chain.Tip()
	id := tip.ID()
	info := infoResult{
		Profile:     n.p.Name,
		Height:      height,
		Tip:         hex.EncodeToString(id[:]),
		Work:        work.Text(16),
		Bits:        fmt.Sprintf("%08x", tip.Bits),
		Difficulty:  pow.Difficulty(tip.Bits, n.p),
		SpacingSecs: int64(n.p.TargetSpacing / time.Second),
		RewardPanda: FormatPanda(n.p.BlockSubsidy(height + 1)),
		NextHalving: (height/n.p.HalvingInterval + 1) * n.p.HalvingInterval,
		Peers:       n.srv.PeerCount(),
		Mempool:     n.mp.Len(),
		Mining:      n.miner != nil,
	}
	if n.miner != nil {
		info.HashRate = n.miner.HashRate()
	}
	if n.wallet != nil {
		info.Address = n.wallet.Address()
	}
	return info, nil
}

// ── getblock (o explorador: um bloco e suas transações por dentro) ─────────

type blockParams struct {
	Height *uint64 `json:"height,omitempty"` // ponteiro: 0 é altura válida
	Hash   string  `json:"hash,omitempty"`
	// ambos vazios = a ponta atual
}

type txInResult struct {
	TxID  string `json:"txid"`
	Index uint32 `json:"index"`
}

type txOutResult struct {
	ValuePanda string `json:"value_panda"`
	Address    string `json:"address"`
}

type txResult struct {
	TxID     string        `json:"txid"`
	Coinbase bool          `json:"coinbase"`
	Ins      []txInResult  `json:"ins,omitempty"`
	Outs     []txOutResult `json:"outs"`
}

type blockResult struct {
	Height        uint64     `json:"height"`
	Hash          string     `json:"hash"`
	Prev          string     `json:"prev"`
	Time          int64      `json:"time"`
	Bits          string     `json:"bits"`
	Difficulty    float64    `json:"difficulty"`
	Nonce         uint64     `json:"nonce"`
	Size          int        `json:"size"`
	Confirmations uint64     `json:"confirmations"`
	Txs           []txResult `json:"txs"`
}

func (n *Node) rpcGetBlock(raw json.RawMessage) (any, error) {
	var p blockParams
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
	}
	_, tipHeight, _ := n.chain.Tip()

	var id [32]byte
	switch {
	case p.Hash != "":
		b, err := hex.DecodeString(p.Hash)
		if err != nil || len(b) != 32 {
			return nil, fmt.Errorf("hash inválido: %q (esperado 64 caracteres hex)", p.Hash)
		}
		copy(id[:], b)
	case p.Height != nil:
		var err error
		if id, err = n.chain.BlockIDByHeight(*p.Height); err != nil {
			return nil, fmt.Errorf("sem bloco na altura %d (a chain está na %d)", *p.Height, tipHeight)
		}
	default:
		tip, _, _ := n.chain.Tip()
		id = tip.ID()
	}

	blk, err := n.chain.GetBlock(id)
	if err != nil {
		return nil, fmt.Errorf("bloco %x não encontrado", id[:8])
	}

	res := blockResult{
		Height:     blk.Header.Height,
		Hash:       hex.EncodeToString(id[:]),
		Prev:       hex.EncodeToString(blk.Header.PrevHash[:]),
		Time:       blk.Header.Timestamp,
		Bits:       fmt.Sprintf("%08x", blk.Header.Bits),
		Difficulty: pow.Difficulty(blk.Header.Bits, n.p),
		Nonce:      blk.Header.Nonce,
		Size:       len(blk.Bytes()),
	}
	if tipHeight >= blk.Header.Height {
		res.Confirmations = tipHeight - blk.Header.Height + 1
	}
	for i := range blk.Txs {
		tx := &blk.Txs[i]
		txid := tx.TxID()
		tr := txResult{TxID: hex.EncodeToString(txid[:]), Coinbase: tx.IsCoinbase()}
		if !tr.Coinbase {
			for _, in := range tx.Ins {
				tr.Ins = append(tr.Ins, txInResult{
					TxID: hex.EncodeToString(in.Prev.TxID[:]), Index: in.Prev.Index,
				})
			}
		}
		for _, out := range tx.Outs {
			tr.Outs = append(tr.Outs, txOutResult{
				ValuePanda: FormatPanda(out.Value),
				Address:    core.AddressFromPKH(out.PubKeyHash),
			})
		}
		res.Txs = append(res.Txs, tr)
	}
	return res, nil
}

// ── getstats / getrecentblocks / getmempool (o painel mempool.space-de-casa) ─

type statsResult struct {
	AvgBlockSecs    float64 `json:"avg_block_seconds"` // média dos últimos até 20 intervalos (0 = sem dados)
	AvgWindow       int     `json:"avg_window"`        // quantos intervalos entraram na média
	TargetSecs      int64   `json:"target_seconds"`
	Difficulty      float64 `json:"difficulty"`
	NextRetarget    uint64  `json:"next_retarget_height"`
	BlocksToRetgt   uint64  `json:"blocks_to_retarget"`
	RetargetFactor  float64 `json:"retarget_factor"` // estimativa: >1 dificuldade sobe (0 = sem dados)
	NextHalving     uint64  `json:"next_halving_height"`
	BlocksToHalve   uint64  `json:"blocks_to_halving"`
	RewardPanda     string  `json:"reward_panda"`
	NextRewardPanda string  `json:"next_reward_panda"`
}

func (n *Node) rpcGetStats() (any, error) {
	tip, height, _ := n.chain.Tip()
	s := statsResult{
		TargetSecs:  int64(n.p.TargetSpacing / time.Second),
		Difficulty:  pow.Difficulty(tip.Bits, n.p),
		NextHalving: (height/n.p.HalvingInterval + 1) * n.p.HalvingInterval,
		RewardPanda: FormatPanda(n.p.BlockSubsidy(height + 1)),
	}
	s.BlocksToHalve = s.NextHalving - height
	s.NextRewardPanda = FormatPanda(n.p.BlockSubsidy(s.NextHalving))
	s.NextRetarget = (height/n.p.RetargetInterval + 1) * n.p.RetargetInterval
	s.BlocksToRetgt = s.NextRetarget - height

	// Tempo médio: últimos até 20 intervalos (tip.ts - ancestral.ts)/n.
	if k := min(height, 20); k > 0 {
		old, err := n.chain.HeaderByHeight(height - k)
		if err == nil && tip.Timestamp > old.Timestamp {
			s.AvgBlockSecs = float64(tip.Timestamp-old.Timestamp) / float64(k)
			s.AvgWindow = int(k)
		}
	}

	// Previsão do retarget: o ritmo da JANELA ATUAL (do último ajuste até a
	// ponta) projetado na conta real do consenso — >1 = dificuldade sobe.
	windowStart := s.NextRetarget - n.p.RetargetInterval
	if blocksIn := height - windowStart; blocksIn > 0 {
		first, err := n.chain.HeaderByHeight(windowStart)
		if err == nil && tip.Timestamp > first.Timestamp {
			actualPerBlock := float64(tip.Timestamp-first.Timestamp) / float64(blocksIn)
			factor := float64(s.TargetSecs) / actualPerBlock
			clamp := float64(n.p.MaxClamp)
			if factor > clamp {
				factor = clamp
			}
			if factor < 1/clamp {
				factor = 1 / clamp
			}
			s.RetargetFactor = factor
		}
	}
	return s, nil
}

type recentParams struct {
	Count int `json:"count,omitempty"` // default 15, máx 50
}

type recentBlock struct {
	Height  uint64 `json:"height"`
	Hash    string `json:"hash"`
	Time    int64  `json:"time"`
	Txs     int    `json:"txs"`
	Size    int    `json:"size"`
	Miner   string `json:"miner"`   // endereço da coinbase
	Elapsed int64  `json:"elapsed"` // segundos desde o bloco anterior (0 no gênesis)
}

func (n *Node) rpcGetRecentBlocks(raw json.RawMessage) (any, error) {
	var p recentParams
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
	}
	count := uint64(15)
	if p.Count > 0 && p.Count <= 50 {
		count = uint64(p.Count)
	}
	_, height, _ := n.chain.Tip()
	start := uint64(0)
	if height+1 > count {
		start = height + 1 - count
	}
	var prevTS int64
	if start > 0 {
		if h, err := n.chain.HeaderByHeight(start - 1); err == nil {
			prevTS = h.Timestamp
		}
	}
	out := make([]recentBlock, 0, count)
	for h := start; h <= height; h++ {
		id, err := n.chain.BlockIDByHeight(h)
		if err != nil {
			return nil, err
		}
		blk, err := n.chain.GetBlock(id)
		if err != nil {
			return nil, err
		}
		rb := recentBlock{
			Height: h,
			Hash:   hex.EncodeToString(id[:]),
			Time:   blk.Header.Timestamp,
			Txs:    len(blk.Txs),
			Size:   len(blk.Bytes()),
		}
		if len(blk.Txs) > 0 && len(blk.Txs[0].Outs) > 0 {
			rb.Miner = core.AddressFromPKH(blk.Txs[0].Outs[0].PubKeyHash)
		}
		if prevTS > 0 && blk.Header.Timestamp > prevTS {
			rb.Elapsed = blk.Header.Timestamp - prevTS
		}
		prevTS = blk.Header.Timestamp
		out = append(out, rb)
	}
	// mais novo primeiro, como todo explorador
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

type mempoolTx struct {
	TxID       string  `json:"txid"`
	Size       int     `json:"size"`
	ValuePanda string  `json:"value_panda"` // Σ outputs — o que a tx movimenta
	FeePanda   string  `json:"fee_panda"`
	FeeRate    float64 `json:"fee_rate"`
}

func (n *Node) rpcGetMempool() (any, error) {
	entries := n.mp.Entries()
	out := make([]mempoolTx, 0, len(entries))
	for _, e := range entries {
		out = append(out, mempoolTx{
			TxID:       hex.EncodeToString(e.TxID[:]),
			Size:       e.Size,
			ValuePanda: FormatPanda(e.Value),
			FeePanda:   FormatPanda(e.Fee),
			FeeRate:    e.FeeRate,
		})
	}
	return out, nil
}

// ── getactivity (o extrato da wallet: entradas e saídas confirmadas) ───────

type activityParams struct {
	Address string `json:"address,omitempty"` // vazio = a wallet do node
	Count   int    `json:"count,omitempty"`   // default 25, máx 100
	Offset  int    `json:"offset,omitempty"`  // pula os N primeiros lançamentos (paginação)
	Filter  string `json:"filter,omitempty"`  // "all" (default) | "tx" | "mined"
}

type activityEntry struct {
	Height       uint64 `json:"height"`
	Time         int64  `json:"time"`
	TxID         string `json:"txid"`
	Direction    string `json:"direction"` // "in" (entrada) | "out" (saída)
	AmountPanda  string `json:"amount_panda"`
	FeePanda     string `json:"fee_panda,omitempty"` // só em saídas
	Coinbase     bool   `json:"coinbase"`
	Counterparty string `json:"counterparty,omitempty"` // de quem veio / para quem foi
}

// rpcGetActivity varre a cadeia ativa da ponta para trás e monta o extrato
// do endereço: outputs recebidos são entradas; inputs gastos (resolvidos
// pelo undo do bloco, que guarda valor e dono de cada moeda consumida) são
// saídas. filter separa recompensas de mineração ("mined") de transações
// comuns ("tx"); offset+count paginam do mais novo para o mais velho. Sem
// índice por endereço — é uma varredura, na régua do node doméstico; o
// count limita quantos lançamentos voltam, não o custo de achá-los.
func (n *Node) rpcGetActivity(raw json.RawMessage) (any, error) {
	var p activityParams
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
	}
	pkh, _, err := n.resolvePKH(p.Address)
	if err != nil {
		return nil, err
	}
	count := 25
	if p.Count > 0 && p.Count <= 100 {
		count = p.Count
	}
	offset := max(p.Offset, 0)
	var wantMined, wantTx bool
	switch p.Filter {
	case "", "all":
		wantMined, wantTx = true, true
	case "mined":
		wantMined = true
	case "tx":
		wantTx = true
	default:
		return nil, fmt.Errorf(`filter inválido: %q (use "all", "tx" ou "mined")`, p.Filter)
	}
	_, height, _ := n.chain.Tip()
	skipped := 0
	out := make([]activityEntry, 0, count)
	for h := height; ; h-- {
		id, err := n.chain.BlockIDByHeight(h)
		if err != nil {
			return nil, err
		}
		blk, err := n.chain.GetBlock(id)
		if err != nil {
			return nil, err
		}
		spentList, err := n.chain.SpentByBlock(id)
		if err != nil {
			return nil, err
		}
		spent := make(map[core.OutPoint]chain.UTXO, len(spentList))
		for _, s := range spentList {
			spent[s.Prev] = s
		}
		// dentro do bloco também do mais novo para o mais velho
		for i := len(blk.Txs) - 1; i >= 0; i-- {
			tx := &blk.Txs[i]
			if cb := tx.IsCoinbase(); (cb && !wantMined) || (!cb && !wantTx) {
				continue
			}
			var sent, totalIn uint64
			for _, in := range tx.Ins {
				e, ok := spent[in.Prev]
				if !ok {
					continue // coinbase (outpoint fictício)
				}
				totalIn += e.Value
				if e.PKH == pkh {
					sent += e.Value
				}
			}
			var recv, totalOut uint64
			for _, o := range tx.Outs {
				totalOut += o.Value
				if o.PubKeyHash == pkh {
					recv += o.Value
				}
			}
			if sent == 0 && recv == 0 {
				continue
			}
			if skipped < offset {
				skipped++
				continue
			}
			txid := tx.TxID()
			e := activityEntry{
				Height: h, Time: blk.Header.Timestamp,
				TxID: hex.EncodeToString(txid[:]), Coinbase: tx.IsCoinbase(),
			}
			if sent > 0 {
				// saída: o valor é o que foi para os outros (o troco volta);
				// a taxa aparece separada, não escondida no valor.
				e.Direction = "out"
				e.AmountPanda = FormatPanda(totalOut - recv)
				if totalIn > totalOut {
					e.FeePanda = FormatPanda(totalIn - totalOut)
				}
				for _, o := range tx.Outs {
					if o.PubKeyHash != pkh {
						e.Counterparty = core.AddressFromPKH(o.PubKeyHash)
						break
					}
				}
			} else {
				e.Direction = "in"
				e.AmountPanda = FormatPanda(recv)
				if !e.Coinbase && len(tx.Ins) > 0 {
					if s, ok := spent[tx.Ins[0].Prev]; ok {
						e.Counterparty = core.AddressFromPKH(s.PKH)
					}
				}
			}
			out = append(out, e)
		}
		if len(out) >= count || h == 0 {
			break
		}
	}
	if len(out) > count {
		out = out[:count]
	}
	return out, nil
}

// ── getbalance / getnewutxos ────────────────────────────────────────────────

type balanceParams struct {
	Address string `json:"address,omitempty"`
}

type balanceResult struct {
	Address        string `json:"address"`
	Balance        uint64 `json:"balance"`
	BalancePanda   string `json:"balance_panda"`
	Spendable      uint64 `json:"spendable"`
	SpendablePanda string `json:"spendable_panda"`
	UTXOs          int    `json:"utxos"`
}

func (n *Node) resolvePKH(address string) ([20]byte, string, error) {
	if address == "" {
		if n.wallet == nil {
			return [20]byte{}, "", errors.New("sem wallet no datadir e sem address nos params — rode `node wallet new` ou informe address")
		}
		return n.wallet.PubKeyHash(), n.wallet.Address(), nil
	}
	pkh, err := core.DecodeAddress(address)
	if err != nil {
		return [20]byte{}, "", fmt.Errorf("endereço inválido %q: %w", address, err)
	}
	return pkh, address, nil
}

func (n *Node) rpcGetBalance(raw json.RawMessage) (any, error) {
	var p balanceParams
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
	}
	pkh, addr, err := n.resolvePKH(p.Address)
	if err != nil {
		return nil, err
	}
	utxos, err := n.chain.UTXOsByPKH(pkh)
	if err != nil {
		return nil, err
	}
	_, tipHeight, _ := n.chain.Tip()
	var total, spendable uint64
	for _, u := range utxos {
		total += u.Value
		if isSpendable(u.Coinbase, u.Height, tipHeight, n.p) && !n.mp.Reserved(u.Prev) {
			spendable += u.Value
		}
	}
	return balanceResult{
		Address:        addr,
		Balance:        total,
		BalancePanda:   FormatPanda(total),
		Spendable:      spendable,
		SpendablePanda: FormatPanda(spendable),
		UTXOs:          len(utxos),
	}, nil
}

type utxoResult struct {
	TxID     string `json:"txid"`
	Index    uint32 `json:"index"`
	Value    uint64 `json:"value"`
	Height   uint64 `json:"height"`
	Coinbase bool   `json:"coinbase"`
}

func (n *Node) rpcGetNewUTXOs(raw json.RawMessage) (any, error) {
	var p balanceParams
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
	}
	pkh, _, err := n.resolvePKH(p.Address)
	if err != nil {
		return nil, err
	}
	spend, err := n.spendableUTXOs(pkh)
	if err != nil {
		return nil, err
	}
	out := make([]utxoResult, 0, len(spend))
	for _, u := range spend {
		out = append(out, utxoResult{
			TxID: hex.EncodeToString(u.Prev.TxID[:]), Index: u.Prev.Index,
			Value: u.Value, Height: u.Height, Coinbase: u.Coinbase,
		})
	}
	return out, nil
}

// ── envio ───────────────────────────────────────────────────────────────────

type sendRawParams struct {
	Raw string `json:"raw"` // tx canônica em hex
}

func (n *Node) rpcSendRawTx(raw json.RawMessage) (any, error) {
	var p sendRawParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	data, err := hex.DecodeString(p.Raw)
	if err != nil {
		return nil, fmt.Errorf("raw não é hex: %w", err)
	}
	tx, err := core.DecodeTx(data)
	if err != nil {
		return nil, fmt.Errorf("tx indecodificável: %w", err)
	}
	if err := n.mp.Add(&tx); err != nil {
		return nil, err
	}
	n.srv.BroadcastTx(&tx)
	txid := tx.TxID()
	return map[string]string{"txid": hex.EncodeToString(txid[:])}, nil
}

type sendParams struct {
	To      string `json:"to"`
	Amount  string `json:"amount"`             // em PANDA, ex.: "1.5"
	FeeRate uint64 `json:"fee_rate,omitempty"` // subunidades/byte (default 1)
}

func (n *Node) rpcSendToAddress(raw json.RawMessage) (any, error) {
	if n.wallet == nil {
		return nil, errors.New("sem wallet no datadir — rode `node wallet new` primeiro")
	}
	var p sendParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	toPKH, err := core.DecodeAddress(p.To)
	if err != nil {
		return nil, fmt.Errorf("endereço de destino inválido: %w", err)
	}
	amount, err := ParseAmount(p.Amount)
	if err != nil {
		return nil, err
	}
	feeRate := p.FeeRate
	if feeRate == 0 {
		feeRate = 1
	}
	spend, err := n.spendableUTXOs(n.wallet.PubKeyHash())
	if err != nil {
		return nil, err
	}
	wutxos := make([]wallet.UTXO, len(spend))
	for i, u := range spend {
		wutxos[i] = wallet.UTXO{Prev: u.Prev, Value: u.Value}
	}
	tx, err := n.wallet.BuildTx(wutxos, toPKH, amount, feeRate)
	if err != nil {
		return nil, err
	}
	if err := n.mp.Add(tx); err != nil {
		return nil, fmt.Errorf("tx construída mas recusada pelo mempool: %w", err)
	}
	n.srv.BroadcastTx(tx)
	txid := tx.TxID()
	return map[string]string{"txid": hex.EncodeToString(txid[:])}, nil
}

// spendableUTXOs filtra o que a wallet pode gastar AGORA: coinbase madura
// (contra a próxima altura) e nada já reservado por tx pendente no mempool.
func (n *Node) spendableUTXOs(pkh [20]byte) ([]chain.UTXO, error) {
	utxos, err := n.chain.UTXOsByPKH(pkh)
	if err != nil {
		return nil, err
	}
	_, tipHeight, _ := n.chain.Tip()
	out := utxos[:0]
	for _, u := range utxos {
		if isSpendable(u.Coinbase, u.Height, tipHeight, n.p) && !n.mp.Reserved(u.Prev) {
			out = append(out, u)
		}
	}
	return out, nil
}

func isSpendable(coinbase bool, born, tipHeight uint64, p params.Params) bool {
	return !coinbase || tipHeight+1-born >= p.CoinbaseMaturity
}

// ── unidades ────────────────────────────────────────────────────────────────

// ParseAmount converte "1.5" (PANDA) para subunidades (1 PANDA = 1e8).
func ParseAmount(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("amount vazio")
	}
	intPart, fracPart, _ := strings.Cut(s, ".")
	if intPart == "" {
		intPart = "0"
	}
	var whole uint64
	if _, err := fmt.Sscanf(intPart, "%d", &whole); err != nil || strings.HasPrefix(intPart, "-") {
		return 0, fmt.Errorf("amount inválido: %q", s)
	}
	if len(fracPart) > 8 {
		return 0, fmt.Errorf("amount com mais de 8 casas decimais: %q", s)
	}
	frac := uint64(0)
	if fracPart != "" {
		padded := fracPart + strings.Repeat("0", 8-len(fracPart))
		if _, err := fmt.Sscanf(padded, "%d", &frac); err != nil {
			return 0, fmt.Errorf("amount inválido: %q", s)
		}
	}
	const maxWhole = ^uint64(0) / params.CoinUnit
	if whole > maxWhole {
		return 0, fmt.Errorf("amount grande demais: %q", s)
	}
	return whole*params.CoinUnit + frac, nil
}

// FormatPanda formata subunidades como PANDA com até 8 casas (sem zeros à
// direita desnecessários).
func FormatPanda(v uint64) string {
	whole := v / params.CoinUnit
	frac := v % params.CoinUnit
	if frac == 0 {
		return fmt.Sprintf("%d", whole)
	}
	s := strings.TrimRight(fmt.Sprintf("%08d", frac), "0")
	return fmt.Sprintf("%d.%s", whole, s)
}
