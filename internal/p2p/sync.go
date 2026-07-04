package p2p

import (
	"fmt"

	"pandabk_coin/internal/core"
)

// syncWindow é quantos corpos de bloco pedimos por vez no IBD: janela
// pequena limita memória e deixa o Argon2 da validação ditar o ritmo.
const syncWindow = 16

// syncState acompanha o IBD com UM peer: os blocos anunciados pelos headers
// que ainda faltam baixar (pending, em ordem) e os pedidos em voo.
type syncState struct {
	pending  [][32]byte
	inFlight map[[32]byte]bool
}

// startSync dispara um getheaders com o locator atual — chamado após o
// handshake (se o peer declara mais trabalho) e sempre que a janela esvazia
// mas o peer continua na frente.
func (s *Server) startSync(pc *peerConn) {
	locator := s.chain.LocatorHashes()
	loc := make([]string, len(locator))
	for i, id := range locator {
		loc[i] = hashToHex(id)
	}
	_ = pc.send(TypeGetHeaders, MsgGetHeaders{Locator: loc})
}

// handleGetHeaders responde com até 2.000 headers da cadeia ativa a partir
// do ancestral comum mais alto do locator.
func (s *Server) handleGetHeaders(pc *peerConn, env Envelope) error {
	msg, err := decodePayload[MsgGetHeaders](env)
	if err != nil {
		return err
	}
	locator := make([][32]byte, 0, len(msg.Locator))
	for _, h := range msg.Locator {
		id, err := hexToHash(h)
		if err != nil {
			return err
		}
		locator = append(locator, id)
	}
	headers, err := s.chain.HeadersSince(locator, MaxHeadersPerMsg)
	if err != nil {
		return err
	}
	raw := make([][]byte, len(headers))
	for i := range headers {
		raw[i] = headers[i].Bytes()
	}
	return pc.send(TypeHeaders, MsgHeaders{Headers: raw})
}

// handleHeaders valida o encadeamento dos headers recebidos (PrevHash em
// sequência — a validação completa, incluindo o Argon2, acontece quando o
// bloco inteiro conecta) e enfileira os corpos que faltam.
func (s *Server) handleHeaders(pc *peerConn, env Envelope) error {
	msg, err := decodePayload[MsgHeaders](env)
	if err != nil {
		return err
	}
	if len(msg.Headers) == 0 {
		return nil // peer não tem nada além do que já temos
	}
	var prev core.Header
	var missing [][32]byte
	for i, raw := range msg.Headers {
		hdr, err := core.DecodeHeader(raw)
		if err != nil {
			return fmt.Errorf("p2p: header inválido do peer: %w", err)
		}
		if i > 0 && (hdr.PrevHash != prev.ID() || hdr.Height != prev.Height+1) {
			return fmt.Errorf("p2p: headers do peer não encadeiam")
		}
		prev = hdr
		id := hdr.ID()
		if _, err := s.chain.GetBlock(id); err == nil {
			continue // já temos o corpo
		}
		missing = append(missing, id)
	}

	pc.mu.Lock()
	if pc.sync.inFlight == nil {
		pc.sync.inFlight = make(map[[32]byte]bool)
	}
	pc.sync.pending = append(pc.sync.pending, missing...)
	pc.mu.Unlock()
	s.pumpSync(pc)
	return nil
}

// pumpSync mantém até syncWindow corpos em voo com o peer.
func (s *Server) pumpSync(pc *peerConn) {
	pc.mu.Lock()
	var batch []InvItem
	for len(pc.sync.inFlight) < syncWindow && len(pc.sync.pending) > 0 {
		id := pc.sync.pending[0]
		pc.sync.pending = pc.sync.pending[1:]
		if pc.sync.inFlight[id] {
			continue
		}
		pc.sync.inFlight[id] = true
		batch = append(batch, InvItem{Kind: KindBlock, ID: hashToHex(id)})
	}
	pc.mu.Unlock()
	if len(batch) > 0 {
		_ = pc.send(TypeGetData, MsgInv{Items: batch})
	}
}

// blockArrived tira o bloco da janela e decide o próximo passo do sync:
// mais corpos, mais headers (peer ainda na frente) ou nada (em dia; o
// steady-state via inv assume).
func (s *Server) blockArrived(pc *peerConn, id [32]byte) {
	pc.mu.Lock()
	delete(pc.sync.inFlight, id)
	drained := len(pc.sync.inFlight) == 0 && len(pc.sync.pending) == 0
	pc.mu.Unlock()
	if !drained {
		s.pumpSync(pc)
		return
	}
	// Janela vazia: se o peer declarou mais trabalho do que temos AGORA,
	// há mais história para buscar (ele tinha 5.000 blocos; cada headers
	// traz 2.000). Se não, o IBD com este peer acabou.
	_, _, ourWork := s.chain.Tip()
	if pc.work.Cmp(ourWork) > 0 {
		s.startSync(pc)
	}
}
