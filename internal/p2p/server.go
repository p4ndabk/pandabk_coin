package p2p

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"sort"
	"sync"
	"time"

	xproxy "golang.org/x/net/proxy"

	"pandabk_coin/internal/chain"
	"pandabk_coin/internal/core"
	"pandabk_coin/internal/mempool"
	"pandabk_coin/internal/params"
)

// Config do servidor p2p. Listen vazio = node outbound-only (atrás de NAT,
// cidadão pleno: valida, minera e propaga pelas conexões que ele mesmo abre).
type Config struct {
	Listen         string        // ex.: ":9551"; "" = só outbound
	Seeds          []string      // peers iniciais (--peers)
	Proxy          string        // SOCKS5 para TODA saída (ex.: 127.0.0.1:9050 do Tor); "" = direto
	Advertise      string        // endereço anunciado no handshake (ex.: xyz.onion:9551); "" = o do listener
	MaxPeers       int           // default 8
	PingInterval   time.Duration // default 2 min (drop após 2 pings sem pong)
	RedialInterval time.Duration // default 15s (manutenção de conexões)

	// OnBlockAccepted (opcional) é chamado após um bloco da REDE ser aceito
	// na chain — o node usa para reiniciar o template do miner.
	OnBlockAccepted func(b *core.Block)
}

// Server conecta a chain e o mempool locais à rede: gossip de blocos/txs por
// inv/getdata, sync inicial por headers e peer exchange.
type Server struct {
	cfg     Config
	chain   *chain.Chain
	mp      *mempool.Mempool
	p       params.Params
	genesis string
	nonce   uint64

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	ln        net.Listener
	advertise string // endereço anunciado no handshake ("" se outbound-only)
	proxyDial xproxy.ContextDialer // != nil quando cfg.Proxy está setado

	mu        sync.Mutex
	peers     map[string]*peerConn
	dialing   map[string]bool
	requested map[[32]byte]time.Time // dedup de getdata disparados por inv
	addrBook  map[string]bool
}

func NewServer(cfg Config, c *chain.Chain, mp *mempool.Mempool, p params.Params) *Server {
	if cfg.MaxPeers <= 0 {
		cfg.MaxPeers = 8
	}
	if cfg.PingInterval <= 0 {
		cfg.PingInterval = 2 * time.Minute
	}
	if cfg.RedialInterval <= 0 {
		cfg.RedialInterval = 15 * time.Second
	}
	var nb [8]byte
	_, _ = rand.Read(nb[:])
	s := &Server{
		cfg:       cfg,
		chain:     c,
		mp:        mp,
		p:         p,
		genesis:   hashToHex(chain.GenesisBlock(p).Header.ID()),
		nonce:     binary.BigEndian.Uint64(nb[:]),
		peers:     make(map[string]*peerConn),
		dialing:   make(map[string]bool),
		requested: make(map[[32]byte]time.Time),
		addrBook:  make(map[string]bool),
	}
	if saved, err := c.LoadAddrBook(); err == nil {
		for _, a := range saved {
			s.addrBook[a] = true
		}
	}
	return s
}

func (s *Server) Start() error {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	if s.cfg.Proxy != "" {
		d, err := xproxy.SOCKS5("tcp", s.cfg.Proxy, nil, &net.Dialer{Timeout: 10 * time.Second})
		if err != nil {
			return fmt.Errorf("p2p: proxy SOCKS5 %q: %w", s.cfg.Proxy, err)
		}
		cd, ok := d.(xproxy.ContextDialer)
		if !ok {
			return fmt.Errorf("p2p: proxy SOCKS5 %q não suporta DialContext", s.cfg.Proxy)
		}
		s.proxyDial = cd
	}
	if s.cfg.Listen != "" {
		ln, err := net.Listen("tcp", s.cfg.Listen)
		if err != nil {
			return err
		}
		s.ln = ln
		s.advertise = ln.Addr().String()
		s.wg.Add(1)
		go s.acceptLoop()
	}
	if s.cfg.Advertise != "" {
		s.advertise = s.cfg.Advertise // hidden service anuncia o .onion, não o 127.0.0.1 local
	}
	s.wg.Add(2)
	go s.maintainLoop()
	go s.pingLoop()
	return nil
}

func (s *Server) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.ln != nil {
		_ = s.ln.Close()
	}
	s.mu.Lock()
	for _, pc := range s.peers {
		pc.close()
	}
	s.mu.Unlock()
	s.wg.Wait()
	return nil
}

// Addr é o endereço real do listener (útil com ":0" nos testes).
func (s *Server) Addr() string { return s.advertise }

func (s *Server) PeerCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.peers)
}

// BroadcastBlock anuncia um bloco (recém-minerado ou conectado) à rede.
func (s *Server) BroadcastBlock(id [32]byte) {
	s.relayInv(nil, InvItem{Kind: KindBlock, ID: hashToHex(id)})
}

// BroadcastTx anuncia uma tx já aceita no mempool local.
func (s *Server) BroadcastTx(tx *core.Tx) {
	s.relayInv(nil, InvItem{Kind: KindTx, ID: hashToHex(tx.TxID())})
}

// ── ciclo de vida das conexões ──────────────────────────────────────────────

func (s *Server) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return // listener fechado no Stop
		}
		if s.PeerCount() >= s.cfg.MaxPeers {
			_ = conn.Close()
			continue
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.setupPeer(conn, conn.RemoteAddr().String(), false)
		}()
	}
}

// maintainLoop mantém conexões de saída: seeds primeiro, depois o address
// book — rediscando quem caiu a cada tick.
func (s *Server) maintainLoop() {
	defer s.wg.Done()
	s.dialMissing()
	t := time.NewTicker(s.cfg.RedialInterval)
	defer t.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
			s.dialMissing()
		}
	}
}

func (s *Server) dialMissing() {
	targets := append([]string{}, s.cfg.Seeds...)
	s.mu.Lock()
	for a := range s.addrBook {
		targets = append(targets, a)
	}
	outbound := 0
	for _, pc := range s.peers {
		if pc.outbound {
			outbound++
		}
	}
	var toDial []string
	seen := map[string]bool{}
	for _, t := range targets {
		if t == "" || t == s.advertise || seen[t] || s.dialing[t] {
			continue
		}
		seen[t] = true
		if outbound+len(toDial) >= s.cfg.MaxPeers {
			break
		}
		s.dialing[t] = true
		toDial = append(toDial, t)
	}
	s.mu.Unlock()

	for _, target := range toDial {
		s.wg.Add(1)
		go func(addr string) {
			defer s.wg.Done()
			conn, err := s.dial(addr)
			if err != nil {
				s.mu.Lock()
				delete(s.dialing, addr) // tenta de novo no próximo tick
				s.mu.Unlock()
				return
			}
			s.setupPeer(conn, addr, true)
		}(target)
	}
}

// dial abre TODA conexão de saída: direta, ou via SOCKS5 quando -proxy está
// configurado — aí é o proxy quem resolve o destino, então `.onion` funciona
// e nenhum DNS/TCP vaza fora do Tor. O timeout maior cobre a construção do
// circuito Tor, que é bem mais lenta que um dial direto.
func (s *Server) dial(addr string) (net.Conn, error) {
	if s.proxyDial == nil {
		return net.DialTimeout("tcp", addr, 5*time.Second)
	}
	ctx, cancel := context.WithTimeout(s.ctx, 45*time.Second)
	defer cancel()
	return s.proxyDial.DialContext(ctx, "tcp", addr)
}

// setupPeer faz o handshake, registra o peer e roda o read loop até a
// conexão morrer. Bloqueia (chamar em goroutine própria).
func (s *Server) setupPeer(conn net.Conn, addr string, outbound bool) {
	defer func() {
		if outbound {
			s.mu.Lock()
			delete(s.dialing, addr)
			s.mu.Unlock()
		}
	}()
	remote, err := doHandshake(conn, s.localVersion())
	if err != nil {
		if !errors.Is(err, ErrSelfConnection) {
			log.Printf("⚠️  handshake com %s falhou: %v", addr, err)
		}
		_ = conn.Close()
		return
	}
	pc := &peerConn{conn: conn, addr: addr, outbound: outbound, ver: remote, work: peerWork(remote)}

	s.mu.Lock()
	if len(s.peers) >= s.cfg.MaxPeers {
		s.mu.Unlock()
		_ = conn.Close()
		return
	}
	s.peers[addr] = pc
	if remote.ListenAddr != "" && remote.ListenAddr != s.advertise {
		s.addrBook[remote.ListenAddr] = true
	}
	s.mu.Unlock()
	s.persistAddrBook()

	dir := "entrada"
	if outbound {
		dir = "saída"
	}
	log.Printf("🤝 peer %s conectado (%s) — altura declarada %d", addr, dir, remote.Height)

	_ = pc.send(TypeGetAddr, nil)
	// Quem declarou mais trabalho acumulado tem a história que nos falta.
	// Já estamos em dia com este peer? Então as pendentes dele nos
	// interessam agora; se não, só depois do sync (senão as txs chegariam
	// antes dos blocos cujos UTXOs elas gastam e seriam rejeitadas).
	if _, ourHeight, ourWork := s.chain.Tip(); pc.work.Cmp(ourWork) > 0 {
		log.Printf("⛓️  %s tem mais trabalho acumulado (altura %d vs nossa %d) — sincronizando", addr, remote.Height, ourHeight)
		s.startSync(pc)
	} else {
		s.askMempool(pc)
	}

	s.readLoop(pc)
	log.Printf("👋 peer %s desconectado", addr)

	s.mu.Lock()
	delete(s.peers, addr)
	s.mu.Unlock()
	pc.close()
}

func (s *Server) readLoop(pc *peerConn) {
	idle := 3 * s.cfg.PingInterval
	if idle < 30*time.Second {
		idle = 30 * time.Second
	}
	for {
		_ = pc.conn.SetReadDeadline(time.Now().Add(idle))
		env, err := ReadMsg(pc.conn)
		if err != nil {
			return
		}
		if err := s.handle(pc, env); err != nil {
			// violação de protocolo ou bloco inválido: derruba o peer
			log.Printf("🚫 derrubando peer %s: %v", pc.addr, err)
			return
		}
	}
}

func (s *Server) pingLoop() {
	defer s.wg.Done()
	t := time.NewTicker(s.cfg.PingInterval)
	defer t.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
			s.mu.Lock()
			peers := make([]*peerConn, 0, len(s.peers))
			for _, pc := range s.peers {
				peers = append(peers, pc)
			}
			s.mu.Unlock()
			for _, pc := range peers {
				pc.mu.Lock()
				missed := pc.pingsOut
				pc.pingsOut++
				pc.mu.Unlock()
				if missed >= 2 {
					pc.close() // read loop desbloqueia e remove
					continue
				}
				_ = pc.send(TypePing, nil)
			}
		}
	}
}

func (s *Server) localVersion() MsgVersion {
	_, height, work := s.chain.Tip()
	return MsgVersion{
		Protocol:   ProtocolVersion,
		Genesis:    s.genesis,
		Height:     height,
		CumWork:    work.Text(16),
		ListenAddr: s.advertise,
		Nonce:      s.nonce,
	}
}

// ── dispatcher de mensagens (pós-handshake) ─────────────────────────────────

func (s *Server) handle(pc *peerConn, env Envelope) error {
	switch env.Type {
	case TypePing:
		return pc.send(TypePong, nil)
	case TypePong:
		pc.mu.Lock()
		pc.pingsOut = 0
		pc.mu.Unlock()
		return nil
	case TypeGetAddr:
		return s.handleGetAddr(pc)
	case TypeAddr:
		return s.handleAddr(env)
	case TypeInv:
		return s.handleInv(pc, env)
	case TypeGetData:
		return s.handleGetData(pc, env)
	case TypeGetHeaders:
		return s.handleGetHeaders(pc, env)
	case TypeHeaders:
		return s.handleHeaders(pc, env)
	case TypeBlock:
		return s.handleBlock(pc, env)
	case TypeTx:
		return s.handleTx(pc, env)
	case TypeGetMempool:
		return s.handleGetMempool(pc)
	case TypeReject:
		return nil // informativo; sem ban score nesta versão
	case TypeVersion, TypeVerack:
		return fmt.Errorf("%w: %q após o handshake", ErrBadHandshake, env.Type)
	default:
		return nil // tipo desconhecido: ignora (compatibilidade futura)
	}
}

func (s *Server) handleGetAddr(pc *peerConn) error {
	s.mu.Lock()
	addrs := make([]string, 0, len(s.addrBook))
	for a := range s.addrBook {
		if a != pc.ver.ListenAddr {
			addrs = append(addrs, a)
		}
	}
	s.mu.Unlock()
	sort.Strings(addrs)
	if len(addrs) > MaxAddrPerMsg {
		addrs = addrs[:MaxAddrPerMsg]
	}
	return pc.send(TypeAddr, MsgAddr{Addrs: addrs})
}

func (s *Server) handleAddr(env Envelope) error {
	msg, err := decodePayload[MsgAddr](env)
	if err != nil {
		return err
	}
	s.mu.Lock()
	for _, a := range msg.Addrs {
		if len(s.addrBook) >= 256 {
			break
		}
		if a != "" && a != s.advertise {
			s.addrBook[a] = true
		}
	}
	s.mu.Unlock()
	s.persistAddrBook()
	return nil
}

func (s *Server) handleInv(pc *peerConn, env Envelope) error {
	msg, err := decodePayload[MsgInv](env)
	if err != nil {
		return err
	}
	var want []InvItem
	now := time.Now()
	for _, item := range msg.Items {
		id, err := hexToHash(item.ID)
		if err != nil {
			return err
		}
		switch item.Kind {
		case KindBlock:
			if _, err := s.chain.GetBlock(id); err == nil {
				continue
			}
		case KindTx:
			if s.mp.Has(id) {
				continue
			}
		default:
			continue
		}
		// dedup: dois peers anunciando o mesmo item → um único getdata
		s.mu.Lock()
		if t, ok := s.requested[id]; ok && now.Sub(t) < 30*time.Second {
			s.mu.Unlock()
			continue
		}
		s.requested[id] = now
		for rid, rt := range s.requested {
			if now.Sub(rt) > time.Minute {
				delete(s.requested, rid)
			}
		}
		s.mu.Unlock()
		want = append(want, item)
	}
	if len(want) == 0 {
		return nil
	}
	return pc.send(TypeGetData, MsgInv{Items: want})
}

func (s *Server) handleGetData(pc *peerConn, env Envelope) error {
	msg, err := decodePayload[MsgInv](env)
	if err != nil {
		return err
	}
	for _, item := range msg.Items {
		id, err := hexToHash(item.ID)
		if err != nil {
			return err
		}
		switch item.Kind {
		case KindBlock:
			blk, err := s.chain.GetBlock(id)
			if err != nil {
				_ = pc.send(TypeReject, MsgReject{Reason: "bloco desconhecido: " + item.ID})
				continue
			}
			if err := pc.send(TypeBlock, MsgBlock{Raw: blk.Bytes()}); err != nil {
				return err
			}
		case KindTx:
			tx, ok := s.mp.Get(id)
			if !ok {
				_ = pc.send(TypeReject, MsgReject{Reason: "tx desconhecida: " + item.ID})
				continue
			}
			if err := pc.send(TypeTx, MsgTx{Raw: tx.Bytes()}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Server) handleBlock(pc *peerConn, env Envelope) error {
	msg, err := decodePayload[MsgBlock](env)
	if err != nil {
		return err
	}
	blk, err := core.DecodeBlock(msg.Raw)
	if err != nil {
		return fmt.Errorf("p2p: bloco indecodificável do peer: %w", err)
	}
	id := blk.Header.ID()
	s.mu.Lock()
	delete(s.requested, id)
	s.mu.Unlock()

	switch err := s.chain.AcceptBlock(&blk); {
	case err == nil:
		s.mp.RemoveConfirmed(&blk)
		s.relayInv(pc, InvItem{Kind: KindBlock, ID: hashToHex(id)})
		if s.cfg.OnBlockAccepted != nil {
			s.cfg.OnBlockAccepted(&blk)
		}
		s.blockArrived(pc, id)
		return nil
	case errors.Is(err, chain.ErrOrphan):
		// pai desconhecido: pede a história que falta a quem mandou
		s.blockArrived(pc, id)
		s.startSync(pc)
		return nil
	default:
		_ = pc.send(TypeReject, MsgReject{Reason: err.Error()})
		return fmt.Errorf("p2p: bloco inválido do peer: %w", err)
	}
}

func (s *Server) handleTx(pc *peerConn, env Envelope) error {
	msg, err := decodePayload[MsgTx](env)
	if err != nil {
		return err
	}
	tx, err := core.DecodeTx(msg.Raw)
	if err != nil {
		return fmt.Errorf("p2p: tx indecodificável do peer: %w", err)
	}
	switch err := s.mp.Add(&tx); {
	case err == nil:
		s.relayInv(pc, InvItem{Kind: KindTx, ID: hashToHex(tx.TxID())})
	case errors.Is(err, mempool.ErrKnown):
		// corrida benigna de gossip: nada a fazer
	default:
		// inválida para o NOSSO estado (pode ser corrida honesta de
		// double-spend pós-bloco): reject informativo, sem derrubar o peer
		_ = pc.send(TypeReject, MsgReject{Reason: err.Error()})
	}
	return nil
}

// askMempool pede as pendentes do peer, uma única vez por conexão — no
// handshake (se já estamos em dia com ele) ou quando o IBD termina. É assim
// que uma tx pendente sobrevive enquanto QUALQUER node da rede estiver de pé.
func (s *Server) askMempool(pc *peerConn) {
	pc.mu.Lock()
	asked := pc.mempoolAsked
	pc.mempoolAsked = true
	pc.mu.Unlock()
	if !asked {
		_ = pc.send(TypeGetMempool, nil)
	}
}

// handleGetMempool anuncia nossas pendentes como inv de txs — o peer pede
// via getdata só as que não conhece, como no gossip normal.
func (s *Server) handleGetMempool(pc *peerConn) error {
	entries := s.mp.Entries() // maior fee rate primeiro
	if len(entries) > MaxMempoolInvPerMsg {
		entries = entries[:MaxMempoolInvPerMsg]
	}
	if len(entries) == 0 {
		return nil
	}
	items := make([]InvItem, len(entries))
	for i, e := range entries {
		items[i] = InvItem{Kind: KindTx, ID: hashToHex(e.TxID)}
	}
	return pc.send(TypeInv, MsgInv{Items: items})
}

// relayInv manda um inv a todos os peers, exceto a origem (nil = todos).
func (s *Server) relayInv(except *peerConn, item InvItem) {
	s.mu.Lock()
	peers := make([]*peerConn, 0, len(s.peers))
	for _, pc := range s.peers {
		if pc != except {
			peers = append(peers, pc)
		}
	}
	s.mu.Unlock()
	for _, pc := range peers {
		_ = pc.send(TypeInv, MsgInv{Items: []InvItem{item}})
	}
}

func (s *Server) persistAddrBook() {
	s.mu.Lock()
	addrs := make([]string, 0, len(s.addrBook))
	for a := range s.addrBook {
		addrs = append(addrs, a)
	}
	s.mu.Unlock()
	sort.Strings(addrs)
	_ = s.chain.SaveAddrBook(addrs)
}
