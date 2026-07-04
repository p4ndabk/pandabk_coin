package node

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"pandabk_coin/internal/chain"
	"pandabk_coin/internal/core"
	"pandabk_coin/internal/params"
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
	Profile  string  `json:"profile"`
	Height   uint64  `json:"height"`
	Tip      string  `json:"tip"`
	Work     string  `json:"work"`
	Peers    int     `json:"peers"`
	Mempool  int     `json:"mempool"`
	Mining   bool    `json:"mining"`
	HashRate float64 `json:"hashrate"`
	Address  string  `json:"address,omitempty"`
}

func (n *Node) rpcGetInfo() (any, error) {
	tip, height, work := n.chain.Tip()
	id := tip.ID()
	info := infoResult{
		Profile: n.p.Name,
		Height:  height,
		Tip:     hex.EncodeToString(id[:]),
		Work:    work.Text(16),
		Peers:   n.srv.PeerCount(),
		Mempool: n.mp.Len(),
		Mining:  n.miner != nil,
	}
	if n.miner != nil {
		info.HashRate = n.miner.HashRate()
	}
	if n.wallet != nil {
		info.Address = n.wallet.Address()
	}
	return info, nil
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
