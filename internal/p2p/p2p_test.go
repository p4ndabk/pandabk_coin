package p2p

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"zhu/internal/chain"
	"zhu/internal/core"
	"zhu/internal/mempool"
	"zhu/internal/params"
	"zhu/internal/pow"
)

// ── codec ───────────────────────────────────────────────────────────────────

func TestCodecRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	want := MsgVersion{Protocol: 1, Genesis: "aa", Height: 7, CumWork: "ff", Nonce: 42}
	if err := WriteMsg(&buf, TypeVersion, want); err != nil {
		t.Fatalf("WriteMsg: %v", err)
	}
	env, err := ReadMsg(&buf)
	if err != nil {
		t.Fatalf("ReadMsg: %v", err)
	}
	if env.Type != TypeVersion {
		t.Fatalf("type = %q", env.Type)
	}
	got, err := decodePayload[MsgVersion](env)
	if err != nil || got != want {
		t.Fatalf("payload = %+v, %v; esperava %+v", got, err, want)
	}
}

func TestOversizeFrameRejectedWithoutReadingBody(t *testing.T) {
	// Frame declara 2 MiB mas o corpo nem existe: ReadMsg tem que falhar só
	// com o prefixo, sem tentar alocar/ler o resto.
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], 2<<20)
	_, err := ReadMsg(bytes.NewReader(hdr[:]))
	if !errors.Is(err, ErrFrameTooBig) {
		t.Fatalf("err = %v, esperava ErrFrameTooBig", err)
	}
}

// ── handshake sobre net.Pipe ────────────────────────────────────────────────

// runHandshake executa os dois lados simultaneamente, como na rede real.
func runHandshake(t *testing.T, va, vb MsgVersion) (ra, rb MsgVersion, ea, eb error) {
	t.Helper()
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	done := make(chan struct{})
	go func() {
		rb, eb = doHandshake(b, vb)
		close(done)
	}()
	ra, ea = doHandshake(a, va)
	<-done
	return ra, rb, ea, eb
}

func TestHandshakeHappyPath(t *testing.T) {
	va := MsgVersion{Protocol: ProtocolVersion, Genesis: "cafe", Height: 10, CumWork: "ff", Nonce: 1}
	vb := MsgVersion{Protocol: ProtocolVersion, Genesis: "cafe", Height: 2, CumWork: "0a", Nonce: 2}
	ra, rb, ea, eb := runHandshake(t, va, vb)
	if ea != nil || eb != nil {
		t.Fatalf("handshake falhou: %v / %v", ea, eb)
	}
	if ra != vb || rb != va {
		t.Fatalf("versions trocadas erradas: %+v / %+v", ra, rb)
	}
}

func TestHandshakeGenesisMismatch(t *testing.T) {
	va := MsgVersion{Protocol: ProtocolVersion, Genesis: "cafe", Nonce: 1}
	vb := MsgVersion{Protocol: ProtocolVersion, Genesis: "beef", Nonce: 2}
	_, _, ea, eb := runHandshake(t, va, vb)
	if !errors.Is(ea, ErrGenesisMismatch) || !errors.Is(eb, ErrGenesisMismatch) {
		t.Fatalf("esperava ErrGenesisMismatch dos dois lados: %v / %v", ea, eb)
	}
}

func TestHandshakeProtocolMismatch(t *testing.T) {
	va := MsgVersion{Protocol: 1, Genesis: "cafe", Nonce: 1}
	vb := MsgVersion{Protocol: 2, Genesis: "cafe", Nonce: 2}
	_, _, ea, eb := runHandshake(t, va, vb)
	if !errors.Is(ea, ErrProtocolMismatch) || !errors.Is(eb, ErrProtocolMismatch) {
		t.Fatalf("esperava ErrProtocolMismatch dos dois lados: %v / %v", ea, eb)
	}
}

func TestHandshakeSelfConnection(t *testing.T) {
	v := MsgVersion{Protocol: ProtocolVersion, Genesis: "cafe", Nonce: 77}
	_, _, ea, eb := runHandshake(t, v, v)
	if !errors.Is(ea, ErrSelfConnection) || !errors.Is(eb, ErrSelfConnection) {
		t.Fatalf("esperava ErrSelfConnection: %v / %v", ea, eb)
	}
}

// ── integração: nodes completos in-process ──────────────────────────────────

type testNode struct {
	t   *testing.T
	c   *chain.Chain
	mp  *mempool.Mempool
	s   *Server
	p   params.Params
	key *ecdsa.PrivateKey
	pub []byte
	pkh [20]byte
	cbs []core.OutPoint
}

func newTestNode(t *testing.T, seeds ...string) *testNode {
	t.Helper()
	p := params.TestNet()
	c, err := chain.Open(filepath.Join(t.TempDir(), "chain.db"), p)
	if err != nil {
		t.Fatalf("chain.Open: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	mp := mempool.New(c, p)
	s := NewServer(Config{
		Listen:         "127.0.0.1:0",
		Seeds:          seeds,
		RedialInterval: 50 * time.Millisecond,
	}, c, mp, p)
	key, err := core.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	pub := core.CompressPubKey(&key.PublicKey)
	return &testNode{t: t, c: c, mp: mp, s: s, p: p, key: key, pub: pub, pkh: core.HashPubKey(pub)}
}

func (n *testNode) start() {
	n.t.Helper()
	if err := n.s.Start(); err != nil {
		n.t.Fatalf("Start: %v", err)
	}
	n.t.Cleanup(func() { n.s.Stop() })
}

func (n *testNode) mine(count int) {
	n.t.Helper()
	for i := 0; i < count; i++ {
		prev, _, _ := n.c.Tip()
		height := prev.Height + 1
		cb := core.NewCoinbase(height, n.p.BlockSubsidy(height), n.pkh, nil)
		b := &core.Block{
			Header: core.Header{
				Version:    1,
				Height:     height,
				PrevHash:   prev.ID(),
				MerkleRoot: core.MerkleRoot([][32]byte{cb.TxID()}),
				Timestamp:  prev.Timestamp + 30,
				Bits:       prev.Bits,
			},
			Txs: []core.Tx{cb},
		}
		target := pow.CompactToTarget(b.Header.Bits)
		for nn := uint64(0); ; nn++ {
			b.Header.Nonce = nn
			hash := pow.PowHash(b.Header.Bytes(), n.p)
			if new(big.Int).SetBytes(hash[:]).Cmp(target) <= 0 {
				break
			}
		}
		if err := n.c.AcceptBlock(b); err != nil {
			n.t.Fatalf("AcceptBlock: %v", err)
		}
		n.cbs = append(n.cbs, core.OutPoint{TxID: b.Txs[0].TxID(), Index: 0})
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout esperando: %s", what)
}

func TestIBDSyncsFiftyBlocks(t *testing.T) {
	a := newTestNode(t)
	a.mine(50)
	a.start()

	b := newTestNode(t, a.s.Addr())
	b.start()

	wantTip, _, _ := a.c.Tip()
	waitFor(t, "B sincronizar os 50 blocos de A", func() bool {
		tip, height, _ := b.c.Tip()
		return height == 50 && tip.ID() == wantTip.ID()
	})
}

func TestForksConvergeToHeavierChain(t *testing.T) {
	a := newTestNode(t)
	a.mine(3)
	b := newTestNode(t) // fork independente: coinbase de outro dono
	b.mine(5)

	a.start()
	// seed adicionado pós-construção: entra direto no address book (é de lá
	// que o maintainLoop disca — cfg.Seeds só é lido no NewServer)
	b.s.mu.Lock()
	b.s.addrBook[a.s.Addr()] = &addrEntry{Seed: true}
	b.s.mu.Unlock()
	b.start()

	wantTip, _, _ := b.c.Tip() // B tem mais trabalho: A converge para B
	waitFor(t, "A adotar a chain mais pesada de B", func() bool {
		tip, height, _ := a.c.Tip()
		return height == 5 && tip.ID() == wantTip.ID()
	})
	// e B continua na própria chain (não regrediu)
	tip, height, _ := b.c.Tip()
	if height != 5 || tip.ID() != wantTip.ID() {
		t.Fatalf("B não deveria ter mudado de chain: altura %d", height)
	}
}

func TestTxGossip(t *testing.T) {
	a := newTestNode(t)
	a.mine(12) // coinbase do bloco 1 madura na próxima altura
	a.start()

	b := newTestNode(t, a.s.Addr())
	b.start()
	waitFor(t, "B sincronizar antes do gossip", func() bool {
		_, height, _ := b.c.Tip()
		return height == 12
	})

	// tx gastando a coinbase 1 de A, com taxa
	subsidy := 50 * params.CoinUnit
	tx := core.Tx{
		Version: 1,
		Ins:     []core.TxIn{{Prev: a.cbs[0], PubKey: a.pub}},
		Outs:    []core.TxOut{{Value: subsidy - 1000, PubKeyHash: [20]byte{9}}},
	}
	sigHash, err := tx.SigHash(0, a.pkh)
	if err != nil {
		t.Fatal(err)
	}
	tx.Ins[0].Sig, err = core.SignHash(a.key, sigHash)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.mp.Add(&tx); err != nil {
		t.Fatalf("mempool A rejeitou a tx: %v", err)
	}
	a.s.BroadcastTx(&tx)

	txid := tx.TxID()
	waitFor(t, "tx aparecer no mempool de B", func() bool {
		return b.mp.Has(txid)
	})
}

// ── proxy SOCKS5 (o caminho do Tor) ─────────────────────────────────────────

// startSocks5 sobe um proxy SOCKS5 mínimo (sem auth, só CONNECT) — o
// suficiente para provar que o node disca através dele, como faria com o
// Tor local em 127.0.0.1:9050. Devolve o endereço e o contador de túneis
// abertos.
func startSocks5(t *testing.T) (addr string, hits *atomic.Int32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("socks5: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	hits = new(atomic.Int32)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				dst, ok := socks5Handshake(c)
				if !ok {
					return
				}
				up, err := net.Dial("tcp", dst)
				if err != nil {
					return
				}
				defer up.Close()
				hits.Add(1)
				go func() { _, _ = io.Copy(up, c) }()
				_, _ = io.Copy(c, up)
			}(conn)
		}
	}()
	return ln.Addr().String(), hits
}

// socks5Handshake fala o lado servidor do protocolo: greeting sem auth,
// pedido CONNECT (IPv4 ou domínio — a forma como um .onion chega) e a
// resposta de sucesso. Devolve o destino pedido.
func socks5Handshake(c net.Conn) (dst string, ok bool) {
	buf := make([]byte, 260)
	if _, err := io.ReadFull(c, buf[:2]); err != nil || buf[0] != 5 {
		return "", false
	}
	if _, err := io.ReadFull(c, buf[:int(buf[1])]); err != nil {
		return "", false
	}
	if _, err := c.Write([]byte{5, 0}); err != nil { // sem autenticação
		return "", false
	}
	if _, err := io.ReadFull(c, buf[:4]); err != nil || buf[1] != 1 {
		return "", false
	}
	var host string
	switch buf[3] {
	case 1: // IPv4
		if _, err := io.ReadFull(c, buf[:4]); err != nil {
			return "", false
		}
		host = net.IP(buf[:4]).String()
	case 3: // domínio — é o proxy quem resolve (o caso .onion)
		if _, err := io.ReadFull(c, buf[:1]); err != nil {
			return "", false
		}
		l := int(buf[0])
		if _, err := io.ReadFull(c, buf[:l]); err != nil {
			return "", false
		}
		host = string(buf[:l])
	default:
		return "", false
	}
	if _, err := io.ReadFull(c, buf[:2]); err != nil {
		return "", false
	}
	port := binary.BigEndian.Uint16(buf[:2])
	if _, err := c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return "", false
	}
	return net.JoinHostPort(host, strconv.Itoa(int(port))), true
}

func TestSyncViaSocks5Proxy(t *testing.T) {
	a := newTestNode(t)
	a.mine(5)
	a.start()

	proxyAddr, hits := startSocks5(t)
	// Seed por hostname (não IP) obriga o dialer a entregar o destino como
	// domínio para o proxy resolver — exatamente o que acontece com .onion.
	_, port, err := net.SplitHostPort(a.s.Addr())
	if err != nil {
		t.Fatal(err)
	}
	b := newTestNode(t, "localhost:"+port)
	b.s.cfg.Proxy = proxyAddr
	b.start()

	wantTip, _, _ := a.c.Tip()
	waitFor(t, "B sincronizar os 5 blocos de A através do proxy", func() bool {
		tip, height, _ := b.c.Tip()
		return height == 5 && tip.ID() == wantTip.ID()
	})
	if hits.Load() == 0 {
		t.Fatal("a conexão não passou pelo proxy")
	}
}

func TestAdvertiseGossipedToPeers(t *testing.T) {
	a := newTestNode(t)
	a.start()

	b := newTestNode(t, a.s.Addr())
	b.s.cfg.Advertise = "zhuxyz.onion:9551" // hidden service anuncia o onion, não o 127.0.0.1
	b.start()
	if got := b.s.Addr(); got != "zhuxyz.onion:9551" {
		t.Fatalf("Addr() = %q, esperava o advertise", got)
	}

	waitFor(t, "A aprender o endereço anunciado por B", func() bool {
		a.s.mu.Lock()
		defer a.s.mu.Unlock()
		_, ok := a.s.addrBook["zhuxyz.onion:9551"]
		return ok
	})
}

// ── higiene do address book: backoff, evicção, cap ──────────────────────────

func TestDialFailureBacksOff(t *testing.T) {
	// porta reservada e fechada na hora: todo dial falha rápido
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dead := ln.Addr().String()
	ln.Close()

	n := newTestNode(t, dead)
	n.start()

	waitFor(t, "primeira falha de dial registrada", func() bool {
		n.s.mu.Lock()
		defer n.s.mu.Unlock()
		e, ok := n.s.addrBook[dead]
		return ok && e.Fails >= 1
	})
	n.s.mu.Lock()
	e := n.s.addrBook[dead]
	fails, next, seed := e.Fails, e.NextTry, e.Seed
	n.s.mu.Unlock()
	if !seed {
		t.Fatal("endereço de -peers deveria estar marcado como seed")
	}
	if !next.After(time.Now().Add(-time.Second)) {
		t.Fatalf("NextTry deveria estar no futuro após falha, veio %v", next)
	}
	// backoff cresce: espera algumas falhas e confere que o intervalo entre
	// tentativas não é mais o tick base (a 3ª falha só vem após ~4× o tick)
	if fails > 3 {
		t.Fatalf("com backoff, %d falhas não cabem tão cedo", fails)
	}
}

func TestDialSuccessResetsBackoff(t *testing.T) {
	a := newTestNode(t)
	a.start()

	b := newTestNode(t, a.s.Addr())
	// simula histórico ruim antes de ligar: falhas acumuladas no seed
	b.s.mu.Lock()
	e := b.s.addrBook[a.s.Addr()]
	e.Fails = 5
	b.s.mu.Unlock()
	b.start()

	waitFor(t, "sucesso zerar as falhas do endereço", func() bool {
		b.s.mu.Lock()
		defer b.s.mu.Unlock()
		e, ok := b.s.addrBook[a.s.Addr()]
		return ok && e.Fails == 0 && !e.LastSeen.IsZero()
	})
}

func TestDeadAddrEvictedSeedStays(t *testing.T) {
	n := newTestNode(t)
	s := n.s

	s.mu.Lock()
	s.addrBook["morto.onion:1"] = &addrEntry{Fails: evictAfterFails - 1}
	s.addrBook["seed-morto.onion:1"] = &addrEntry{Seed: true, Fails: evictAfterFails + 3}
	s.mu.Unlock()

	s.noteDialResult("morto.onion:1", false)
	s.noteDialResult("seed-morto.onion:1", false)

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.addrBook["morto.onion:1"]; ok {
		t.Fatal("endereço morto não-seed deveria ter sido evicto")
	}
	e, ok := s.addrBook["seed-morto.onion:1"]
	if !ok {
		t.Fatal("seed nunca deve ser evicto, só esperar o backoff")
	}
	if !e.NextTry.After(time.Now()) {
		t.Fatal("seed morto deveria estar em backoff")
	}
}

func TestFullBookEvictsWorst(t *testing.T) {
	n := newTestNode(t)
	s := n.s

	s.mu.Lock()
	for i := 0; len(s.addrBook) < maxAddrBook-1; i++ {
		s.addrBook[fmt.Sprintf("lixo%d.onion:1", i)] = &addrEntry{Fails: 3}
	}
	// a pior de todas: mais falhas que o resto — book fica exatamente cheio
	s.addrBook["pior.onion:1"] = &addrEntry{Fails: 9}
	s.mu.Unlock()

	env := Envelope{Type: TypeAddr, Payload: mustJSON(t, MsgAddr{Addrs: []string{"novo.onion:1"}})}
	if err := s.handleAddr(nil, env); err != nil {
		t.Fatalf("handleAddr: %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.addrBook["novo.onion:1"]; !ok {
		t.Fatal("endereço novo deveria entrar no book cheio (evictando a pior entrada)")
	}
	if len(s.addrBook) > maxAddrBook {
		t.Fatalf("book estourou o cap: %d", len(s.addrBook))
	}
}

// ── addr relay: descoberta e auto-recuperação sem reconectar ────────────────

func TestNewPeerAddrRelayedToNeighbors(t *testing.T) {
	s := newTestNode(t)
	s.start()

	b := newTestNode(t, s.s.Addr())
	b.start()
	waitFor(t, "B conectar ao seed", func() bool { return b.s.PeerCount() >= 1 })

	// C entra na rede DEPOIS de B já estar conectado: sem relay, B só saberia
	// de C num futuro getaddr — com relay, o seed anuncia C na hora.
	c := newTestNode(t, s.s.Addr())
	c.start()

	waitFor(t, "B aprender o endereço de C via relay", func() bool {
		for _, a := range b.s.KnownAddrs() {
			if a == c.s.Addr() {
				return true
			}
		}
		return false
	})
}

func TestNetworkSurvivesSeedDeath(t *testing.T) {
	seed := newTestNode(t)
	seed.start()

	a := newTestNode(t, seed.s.Addr())
	a.start()
	b := newTestNode(t, seed.s.Addr())
	b.start()

	waitFor(t, "A aprender o endereço de B", func() bool {
		for _, addr := range a.s.KnownAddrs() {
			if addr == b.s.Addr() {
				return true
			}
		}
		return false
	})

	// morre o único peer configurado — o cenário que motivou o address book
	if err := seed.s.Stop(); err != nil {
		t.Fatalf("Stop do seed: %v", err)
	}

	// A e B se conectam entre si a partir do que aprenderam: a rede sobrevive
	waitFor(t, "A e B conectados entre si sem o seed", func() bool {
		aToB := false
		for _, p := range a.s.Peers() {
			if p.Addr == b.s.Addr() || p.ListenAddr == b.s.Addr() {
				aToB = true
			}
		}
		return aToB && b.s.PeerCount() >= 1
	})
}

func TestKnownAddrNotRerelayed(t *testing.T) {
	n := newTestNode(t)
	s := n.s

	// vizinho falso sobre net.Pipe para observar o que o relay envia
	ours, theirs := net.Pipe()
	t.Cleanup(func() { ours.Close(); theirs.Close() })
	s.mu.Lock()
	s.peers["pipe"] = &peerConn{conn: ours, addr: "pipe", ver: MsgVersion{ListenAddr: "vizinho.onion:1"}}
	s.mu.Unlock()

	got := make(chan Envelope, 1)
	readOne := func() {
		go func() {
			if env, err := ReadMsg(theirs); err == nil {
				got <- env
			}
		}()
	}

	env := Envelope{Type: TypeAddr, Payload: mustJSON(t, MsgAddr{Addrs: []string{"novato.onion:1"}})}
	readOne()
	if err := s.handleAddr(nil, env); err != nil {
		t.Fatalf("handleAddr: %v", err)
	}
	select {
	case e := <-got:
		msg, err := decodePayload[MsgAddr](e)
		if e.Type != TypeAddr || err != nil || len(msg.Addrs) != 1 || msg.Addrs[0] != "novato.onion:1" {
			t.Fatalf("relay inesperado: type=%q %+v, %v", e.Type, msg, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("endereço novo deveria ter sido relayado ao vizinho")
	}

	// o mesmo endereço de novo (o eco do gossip): já conhecido, morre em silêncio
	readOne()
	if err := s.handleAddr(nil, env); err != nil {
		t.Fatalf("handleAddr: %v", err)
	}
	select {
	case e := <-got:
		t.Fatalf("eco relayado de volta: type=%q", e.Type)
	case <-time.After(300 * time.Millisecond):
		// silêncio esperado: o gossip converge em vez de circular
	}
}

// ── validação de endereço: lixo não entra no book nem é relayado ───────────

func TestIsDialableAddr(t *testing.T) {
	cases := map[string]bool{
		"192.168.68.101:9551": true,
		"exemplo.onion:9551":  true,
		"[::1]:9551":          true, // loopback IPv6 é válido como host, só não é alcançável de fora
		"[::]:9054":           false, // bind coringa — o bug real observado em produção
		"0.0.0.0:9551":        false, // bind coringa IPv4
		"exemplo.onion":       false, // sem porta (advertise mal configurado)
		"":                    false,
		":9551":               false, // sem host
	}
	for addr, want := range cases {
		if got := isDialableAddr(addr); got != want {
			t.Errorf("isDialableAddr(%q) = %v, esperava %v", addr, got, want)
		}
	}
}

func TestHandleAddrRejectsGarbage(t *testing.T) {
	n := newTestNode(t)
	s := n.s

	env := Envelope{Type: TypeAddr, Payload: mustJSON(t, MsgAddr{
		Addrs: []string{"[::]:9054", "sem-porta.onion", "bom.onion:9551"},
	})}
	if err := s.handleAddr(nil, env); err != nil {
		t.Fatalf("handleAddr: %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.addrBook["[::]:9054"]; ok {
		t.Fatal("endereço coringa não deveria entrar no book")
	}
	if _, ok := s.addrBook["sem-porta.onion"]; ok {
		t.Fatal("endereço sem porta não deveria entrar no book")
	}
	if _, ok := s.addrBook["bom.onion:9551"]; !ok {
		t.Fatal("endereço válido deveria ter entrado no book")
	}
}

func TestPersistedGarbageFilteredOnLoad(t *testing.T) {
	p := params.TestNet()
	c, err := chain.Open(filepath.Join(t.TempDir(), "chain.db"), p)
	if err != nil {
		t.Fatalf("chain.Open: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	// simula um book poluído por versões anteriores, sem a validação de hoje
	if err := c.SaveAddrBook([]string{"[::]:9054", "sem-porta.onion", "bom.onion:9551"}); err != nil {
		t.Fatalf("SaveAddrBook: %v", err)
	}

	mp := mempool.New(c, p)
	s := NewServer(Config{Listen: "127.0.0.1:0"}, c, mp, p)

	if _, ok := s.addrBook["[::]:9054"]; ok {
		t.Fatal("boot deveria ter descartado o endereço coringa persistido")
	}
	if _, ok := s.addrBook["sem-porta.onion"]; ok {
		t.Fatal("boot deveria ter descartado o endereço sem porta persistido")
	}
	if _, ok := s.addrBook["bom.onion:9551"]; !ok {
		t.Fatal("endereço válido persistido deveria ter sobrevivido ao boot")
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestNewBlockPropagatesViaInv(t *testing.T) {
	a := newTestNode(t)
	a.mine(5)
	a.start()
	b := newTestNode(t, a.s.Addr())
	b.start()
	waitFor(t, "sync inicial", func() bool {
		_, height, _ := b.c.Tip()
		return height == 5
	})

	// A minera mais um e anuncia (o que o miner fará no M5)
	a.mine(1)
	tip, _, _ := a.c.Tip()
	a.s.BroadcastBlock(tip.ID())
	waitFor(t, "bloco novo chegar em B via inv/getdata", func() bool {
		btip, height, _ := b.c.Tip()
		return height == 6 && btip.ID() == tip.ID()
	})
}
