package p2p

import (
	"errors"
	"fmt"
	"math/big"
	"net"
	"sync"
	"time"
)

var (
	ErrGenesisMismatch  = errors.New("p2p: gênesis diferente — outra rede")
	ErrProtocolMismatch = errors.New("p2p: versão de protocolo incompatível")
	ErrSelfConnection   = errors.New("p2p: conexão consigo mesmo (nonce igual)")
	ErrBadHandshake     = errors.New("p2p: handshake fora de ordem")
)

const handshakeTimeout = 10 * time.Second

// doHandshake troca version/verack sobre qualquer conexão (testável com
// net.Pipe): envia nossa version, valida a do peer (protocolo, gênesis,
// auto-conexão), confirma com verack e espera o verack dele. Nenhuma outra
// mensagem é processada antes disso. Os DOIS lados executam esta mesma
// sequência simultaneamente, então cada envio acontece em goroutine,
// concorrente com a leitura do outro lado — obrigatório sobre net.Pipe
// (síncrono, sem buffer) e inofensivo sobre TCP.
func doHandshake(conn net.Conn, local MsgVersion) (MsgVersion, error) {
	deadline := time.Now().Add(handshakeTimeout)
	_ = conn.SetDeadline(deadline)
	defer conn.SetDeadline(time.Time{})

	sendErr := make(chan error, 1)
	go func() { sendErr <- WriteMsg(conn, TypeVersion, local) }()
	env, err := ReadMsg(conn)
	if err != nil {
		return MsgVersion{}, err
	}
	if err := <-sendErr; err != nil {
		return MsgVersion{}, err
	}
	if env.Type == TypeReject {
		return MsgVersion{}, rejectError(env)
	}
	if env.Type != TypeVersion {
		return MsgVersion{}, fmt.Errorf("%w: esperava version, veio %q", ErrBadHandshake, env.Type)
	}
	remote, err := decodePayload[MsgVersion](env)
	if err != nil {
		return MsgVersion{}, err
	}
	if remote.Nonce == local.Nonce {
		return MsgVersion{}, ErrSelfConnection // drop silencioso, sem reject
	}
	if remote.Protocol != local.Protocol {
		go func() { _ = WriteMsg(conn, TypeReject, MsgReject{Reason: "protocolo incompatível"}) }()
		return MsgVersion{}, ErrProtocolMismatch
	}
	if remote.Genesis != local.Genesis {
		go func() { _ = WriteMsg(conn, TypeReject, MsgReject{Reason: "gênesis diferente — outra rede"}) }()
		return MsgVersion{}, ErrGenesisMismatch
	}

	go func() { sendErr <- WriteMsg(conn, TypeVerack, nil) }()
	env, err = ReadMsg(conn)
	if err != nil {
		return MsgVersion{}, err
	}
	if err := <-sendErr; err != nil {
		return MsgVersion{}, err
	}
	if env.Type == TypeReject {
		return MsgVersion{}, rejectError(env)
	}
	if env.Type != TypeVerack {
		return MsgVersion{}, fmt.Errorf("%w: esperava verack, veio %q", ErrBadHandshake, env.Type)
	}
	return remote, nil
}

func rejectError(env Envelope) error {
	if rej, err := decodePayload[MsgReject](env); err == nil {
		return fmt.Errorf("p2p: peer rejeitou: %s", rej.Reason)
	}
	return fmt.Errorf("p2p: peer rejeitou o handshake")
}

// peerConn é um vizinho conectado (handshake completo). Escritas são
// serializadas por wmu; o read loop é a única goroutine que lê.
type peerConn struct {
	conn     net.Conn
	addr     string // endereço remoto (o discado, se outbound)
	outbound bool
	ver      MsgVersion // version do peer no handshake
	work     *big.Int   // cum_work declarado no handshake

	wmu sync.Mutex

	mu       sync.Mutex
	pingsOut int
	sync     syncState
}

func (pc *peerConn) send(typ string, payload any) error {
	pc.wmu.Lock()
	defer pc.wmu.Unlock()
	_ = pc.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return WriteMsg(pc.conn, typ, payload)
}

func (pc *peerConn) close() { _ = pc.conn.Close() }

// peerWork interpreta o cum_work hex declarado (zero se malformado — peer
// que mente sobre trabalho só deixa de disparar sync a partir dele).
func peerWork(v MsgVersion) *big.Int {
	w, ok := new(big.Int).SetString(v.CumWork, 16)
	if !ok || w.Sign() < 0 {
		return new(big.Int)
	}
	return w
}
