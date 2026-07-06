package node

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"pandabk_coin/internal/chain"
	"pandabk_coin/internal/params"
	"pandabk_coin/internal/wallet"
)

func testConfig(t *testing.T, mine bool, peers ...string) Config {
	t.Helper()
	return Config{
		DataDir: t.TempDir(),
		Listen:  "127.0.0.1:0",
		RPC:     "127.0.0.1:0",
		Peers:   peers,
		Mine:    mine,
		Miners:  1,
		Profile: "test",
	}
}

func rpcCall(t *testing.T, addr, method string, prms any, out any) error {
	t.Helper()
	body, err := json.Marshal(rpcRequest{Method: method, Params: mustJSON(t, prms)})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post("http://"+addr+"/rpc", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("RPC %s: %v", method, err)
	}
	defer resp.Body.Close()
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decodificando resposta de %s: %v", method, err)
	}
	if envelope.Error != nil {
		return errors.New(envelope.Error.Message)
	}
	if out != nil {
		if err := json.Unmarshal(envelope.Result, out); err != nil {
			t.Fatalf("result de %s: %v", method, err)
		}
	}
	return nil
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	if v == nil {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func waitFor(t *testing.T, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout esperando: %s", what)
}

func TestParseAmountAndFormatPanda(t *testing.T) {
	cases := []struct {
		in   string
		want uint64
		err  bool
	}{
		{"1.5", 150_000_000, false},
		{"0.00000001", 1, false},
		{"50", 50 * params.CoinUnit, false},
		{" 2.25 ", 225_000_000, false},
		{"1.123456789", 0, true}, // 9 casas
		{"-1", 0, true},
		{"abc", 0, true},
		{"", 0, true},
	}
	for _, c := range cases {
		got, err := ParseAmount(c.in)
		if c.err != (err != nil) || got != c.want {
			t.Fatalf("ParseAmount(%q) = %d, %v; esperava %d (err=%v)", c.in, got, err, c.want, c.err)
		}
	}
	if s := FormatPanda(150_000_000); s != "1.5" {
		t.Fatalf("FormatPanda(1.5) = %q", s)
	}
	if s := FormatPanda(50 * params.CoinUnit); s != "50" {
		t.Fatalf("FormatPanda(50) = %q", s)
	}
}

func TestRPCRejectsNonLoopbackBind(t *testing.T) {
	cfg := testConfig(t, false)
	cfg.RPC = "0.0.0.0:38555"
	n, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer n.Stop()
	if err := n.Start(); !errors.Is(err, ErrRPCNotLoopback) {
		t.Fatalf("err = %v, esperava ErrRPCNotLoopback", err)
	}
}

func TestSendWithoutWalletFailsClearly(t *testing.T) {
	cfg := testConfig(t, false) // sem minerar → sem wallet automática
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Start(); err != nil {
		t.Fatal(err)
	}
	defer n.Stop()
	err = rpcCall(t, n.RPCAddr(), "sendtoaddress", sendParams{To: "P123", Amount: "1"}, nil)
	if err == nil || !strings.Contains(err.Error(), "wallet new") {
		t.Fatalf("erro deveria instruir `node wallet new`: %v", err)
	}
}

// TestDemoTwoNodes é a demo do PLAN.md como teste: A minera por padrão,
// B (sem minerar) sincroniza de A, um send de A vira saldo em B depois da
// confirmação — o fio completo wallet→mempool→miner→p2p→chain→balance.
func TestDemoTwoNodes(t *testing.T) {
	// Node A: minerador (a wallet nasce automaticamente com o node)
	a, err := New(testConfig(t, true))
	if err != nil {
		t.Fatalf("New(A): %v", err)
	}
	if err := a.Start(); err != nil {
		t.Fatalf("Start(A): %v", err)
	}
	defer a.Stop()

	// espera A ter coinbase madura para gastar (altura ≥ 11)
	waitFor(t, "A minerar 11 blocos", 30*time.Second, func() bool {
		_, h, _ := a.chain.Tip()
		return h >= 11
	})

	// Node B: seguidor com wallet própria (criada antes, sem minerar)
	bcfg := testConfig(t, false, a.P2PAddr())
	bw, err := wallet.New(bcfg.WalletPath())
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(bcfg)
	if err != nil {
		t.Fatalf("New(B): %v", err)
	}
	if err := b.Start(); err != nil {
		t.Fatalf("Start(B): %v", err)
	}
	defer b.Stop()

	waitFor(t, "B sincronizar com A", 30*time.Second, func() bool {
		_, ha, _ := a.chain.Tip()
		_, hb, _ := b.chain.Tip()
		return hb > 0 && hb+2 >= ha // colado na ponta (A segue minerando)
	})

	// send de A para o endereço de B via RPC de A
	var sendRes map[string]string
	if err := rpcCall(t, a.RPCAddr(), "sendtoaddress",
		sendParams{To: bw.Address(), Amount: "1.5"}, &sendRes); err != nil {
		t.Fatalf("sendtoaddress: %v", err)
	}
	if len(sendRes["txid"]) != 64 {
		t.Fatalf("txid inesperado: %+v", sendRes)
	}

	// a tx confirma num bloco de A e o saldo aparece em B
	waitFor(t, "1.5 PANDA aparecer no saldo de B", 30*time.Second, func() bool {
		var bal balanceResult
		if err := rpcCall(t, b.RPCAddr(), "getbalance", nil, &bal); err != nil {
			return false
		}
		return bal.Balance == 150_000_000
	})

	// getinfo dos dois lados responde coerente
	var info infoResult
	if err := rpcCall(t, a.RPCAddr(), "getinfo", nil, &info); err != nil {
		t.Fatal(err)
	}
	if !info.Mining || info.Address == "" || info.Height == 0 {
		t.Fatalf("getinfo de A estranho: %+v", info)
	}

	// shutdown limpo: fechar B e reabrir a chain sem erro (bbolt íntegro)
	dataDir := bcfg.ChainPath()
	if err := b.Stop(); err != nil {
		t.Fatalf("Stop(B): %v", err)
	}
	p, _ := bcfg.Params()
	reopened, err := chain.Open(dataDir, p)
	if err != nil {
		t.Fatalf("reabrir chain de B após Stop: %v", err)
	}
	reopened.Close()
}

func TestGetInfoAndBalanceHandlers(t *testing.T) {
	cfg := testConfig(t, false)
	if _, err := wallet.New(cfg.WalletPath()); err != nil {
		t.Fatal(err)
	}
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Start(); err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	var info infoResult
	if err := rpcCall(t, n.RPCAddr(), "getinfo", nil, &info); err != nil {
		t.Fatal(err)
	}
	if info.Profile != "test" || info.Height != 0 || info.Mining {
		t.Fatalf("getinfo: %+v", info)
	}
	var bal balanceResult
	if err := rpcCall(t, n.RPCAddr(), "getbalance", nil, &bal); err != nil {
		t.Fatal(err)
	}
	if bal.Balance != 0 || bal.Address == "" {
		t.Fatalf("getbalance: %+v", bal)
	}
	if err := rpcCall(t, n.RPCAddr(), "getbalance", balanceParams{Address: "Pinvalido"}, nil); err == nil {
		t.Fatal("endereço inválido deveria dar erro")
	}
	if err := rpcCall(t, n.RPCAddr(), "nada", nil, nil); err == nil ||
		!strings.Contains(err.Error(), "desconhecido") {
		t.Fatalf("método desconhecido: %v", err)
	}

	// getblock: sem params = ponta (aqui, o gênesis); por altura idem; a
	// coinbase aparece com valor e endereço legíveis.
	var blk blockResult
	if err := rpcCall(t, n.RPCAddr(), "getblock", nil, &blk); err != nil {
		t.Fatal(err)
	}
	if blk.Height != 0 || blk.Confirmations != 1 || len(blk.Txs) != 1 || !blk.Txs[0].Coinbase {
		t.Fatalf("getblock (ponta): %+v", blk)
	}
	h := uint64(0)
	var byH blockResult
	if err := rpcCall(t, n.RPCAddr(), "getblock", blockParams{Height: &h}, &byH); err != nil || byH.Hash != blk.Hash {
		t.Fatalf("getblock por altura: %v (%s != %s)", err, byH.Hash, blk.Hash)
	}
	if len(blk.Txs[0].Outs) != 1 || blk.Txs[0].Outs[0].Address == "" {
		t.Fatalf("coinbase do gênesis sem output legível: %+v", blk.Txs[0])
	}
	bad := uint64(99)
	if err := rpcCall(t, n.RPCAddr(), "getblock", blockParams{Height: &bad}, nil); err == nil {
		t.Fatal("altura inexistente deveria dar erro")
	}

	// O painel: getstats/getrecentblocks/getmempool numa chain só-gênesis.
	var st statsResult
	if err := rpcCall(t, n.RPCAddr(), "getstats", nil, &st); err != nil {
		t.Fatal(err)
	}
	if st.AvgWindow != 0 || st.BlocksToHalve != 1000 || st.BlocksToRetgt != 100 || st.TargetSecs != 60 {
		t.Fatalf("getstats: %+v", st)
	}
	if st.RewardPanda != "50" || st.NextRewardPanda != "25" {
		t.Fatalf("recompensas do halving: %+v", st)
	}
	var recent []recentBlock
	if err := rpcCall(t, n.RPCAddr(), "getrecentblocks", nil, &recent); err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 || recent[0].Height != 0 || recent[0].Miner == "" {
		t.Fatalf("getrecentblocks: %+v", recent)
	}
	var pending []mempoolTx
	if err := rpcCall(t, n.RPCAddr(), "getmempool", nil, &pending); err != nil || len(pending) != 0 {
		t.Fatalf("getmempool: %v %v", pending, err)
	}
}
