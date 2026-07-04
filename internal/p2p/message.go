package p2p

import (
	"encoding/hex"
	"fmt"
)

// ProtocolVersion sobe quando o protocolo muda de forma incompatível.
const ProtocolVersion = 1

// Tipos de mensagem do protocolo (SPEC.md).
const (
	TypeVersion    = "version"
	TypeVerack     = "verack"
	TypePing       = "ping"
	TypePong       = "pong"
	TypeGetAddr    = "getaddr"
	TypeAddr       = "addr"
	TypeInv        = "inv"
	TypeGetData    = "getdata"
	TypeGetHeaders = "getheaders"
	TypeHeaders    = "headers"
	TypeBlock      = "block"
	TypeTx         = "tx"
	TypeReject     = "reject"
)

// MsgVersion abre o handshake: genesis é o ID da rede (mismatch = redes
// diferentes), height/cum_work dizem quem está atrasado, nonce detecta
// auto-conexão e listen_addr ("" = outbound-only, atrás de NAT) é o endereço
// anunciável no address book.
type MsgVersion struct {
	Protocol   uint32 `json:"protocol"`
	Genesis    string `json:"genesis"`
	Height     uint64 `json:"height"`
	CumWork    string `json:"cum_work"`
	ListenAddr string `json:"listen_addr,omitempty"`
	Nonce      uint64 `json:"nonce"`
}

type MsgAddr struct {
	Addrs []string `json:"addrs"`
}

// InvItem anuncia/pede um item por ID; Kind é "block" ou "tx".
type InvItem struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

const (
	KindBlock = "block"
	KindTx    = "tx"
)

type MsgInv struct {
	Items []InvItem `json:"items"`
}

type MsgGetHeaders struct {
	Locator []string `json:"locator"`
}

// MsgHeaders carrega headers crus de 96 bytes (base64 no JSON).
type MsgHeaders struct {
	Headers [][]byte `json:"headers"`
}

type MsgBlock struct {
	Raw []byte `json:"raw"`
}

type MsgTx struct {
	Raw []byte `json:"raw"`
}

type MsgReject struct {
	Reason string `json:"reason"`
}

// MaxHeadersPerMsg limita a resposta de getheaders (2.000 × 96 B ≈ 190 KiB
// antes do base64 — dentro do frame de 1 MiB).
const MaxHeadersPerMsg = 2000

// MaxAddrPerMsg limita o gossip de endereços.
const MaxAddrPerMsg = 32

func hashToHex(h [32]byte) string { return hex.EncodeToString(h[:]) }

func hexToHash(s string) ([32]byte, error) {
	var h [32]byte
	raw, err := hex.DecodeString(s)
	if err != nil || len(raw) != 32 {
		return h, fmt.Errorf("p2p: hash hex inválido %q", s)
	}
	copy(h[:], raw)
	return h, nil
}
