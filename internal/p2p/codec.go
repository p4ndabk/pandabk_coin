// Package p2p é a rede da PANDA: TCP puro + frames JSON com prefixo de
// tamanho, gossip por inventário (inv/getdata) e sync inicial por headers.
// Não existe servidor central — a descentralização da rede é literalmente
// este pacote. Ver internal/p2p/SPEC.md.
package p2p

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// MaxFrame limita qualquer mensagem a 1 MiB — um peer malicioso não força
// alocação gigante: o tamanho declarado é checado ANTES de ler o corpo.
const MaxFrame = 1 << 20

var ErrFrameTooBig = errors.New("p2p: frame acima de 1 MiB")

// Envelope é o formato de toda mensagem no fio: {type, payload JSON}.
// Blocos/txs/headers viajam como bytes canônicos (base64 automático do
// encoding/json para []byte) — o JSON é só o envelope debugável; os bytes de
// consenso continuam os do encoding canônico do core.
type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// WriteMsg serializa e envia um frame: 4 bytes big-endian de tamanho + JSON.
func WriteMsg(w io.Writer, typ string, payload any) error {
	env := Envelope{Type: typ}
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		env.Payload = raw
	}
	body, err := json.Marshal(env)
	if err != nil {
		return err
	}
	if len(body) > MaxFrame {
		return ErrFrameTooBig
	}
	buf := make([]byte, 4+len(body))
	binary.BigEndian.PutUint32(buf, uint32(len(body)))
	copy(buf[4:], body)
	_, err = w.Write(buf)
	return err
}

// ReadMsg lê um frame completo. Tamanho acima de MaxFrame é erro imediato,
// sem alocar nem ler o corpo.
func ReadMsg(r io.Reader) (Envelope, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return Envelope{}, err
	}
	n := binary.BigEndian.Uint32(lenBuf[:])
	if n > MaxFrame {
		return Envelope{}, fmt.Errorf("%w (%d bytes declarados)", ErrFrameTooBig, n)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return Envelope{}, err
	}
	var env Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return Envelope{}, fmt.Errorf("p2p: frame não é JSON válido: %w", err)
	}
	return env, nil
}

// decodePayload tipa o payload de um envelope.
func decodePayload[T any](env Envelope) (T, error) {
	var v T
	if len(env.Payload) == 0 {
		return v, fmt.Errorf("p2p: mensagem %q sem payload", env.Type)
	}
	if err := json.Unmarshal(env.Payload, &v); err != nil {
		return v, fmt.Errorf("p2p: payload de %q inválido: %w", env.Type, err)
	}
	return v, nil
}
