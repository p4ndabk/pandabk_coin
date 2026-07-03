package core

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"errors"
)

// Chaves ECDSA P-256 da stdlib: pure Go, constante no tempo, zero dependência
// externa. A chave pública viaja comprimida (33 bytes) dentro do TxIn.

const PubKeySize = 33

var ErrInvalidPubKey = errors.New("core: chave pública inválida")

func GenerateKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

func CompressPubKey(pub *ecdsa.PublicKey) []byte {
	return elliptic.MarshalCompressed(elliptic.P256(), pub.X, pub.Y)
}

func ParsePubKey(b []byte) (*ecdsa.PublicKey, error) {
	if len(b) != PubKeySize {
		return nil, ErrInvalidPubKey
	}
	x, y := elliptic.UnmarshalCompressed(elliptic.P256(), b)
	if x == nil {
		return nil, ErrInvalidPubKey
	}
	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, nil
}

// SignHash assina um sighash com a chave privada (ASN.1 DER).
func SignHash(priv *ecdsa.PrivateKey, hash [32]byte) ([]byte, error) {
	return ecdsa.SignASN1(rand.Reader, priv, hash[:])
}

// VerifySignature confere uma assinatura DER contra a pubkey comprimida.
func VerifySignature(pubkey []byte, hash [32]byte, sig []byte) bool {
	pub, err := ParsePubKey(pubkey)
	if err != nil {
		return false
	}
	return ecdsa.VerifyASN1(pub, hash[:], sig)
}

// HashPubKey deriva o PubKeyHash de 20 bytes que tranca um TxOut:
// SHA-256 truncado (sem ripemd160, que é deprecado).
func HashPubKey(pub []byte) [20]byte {
	sum := sha256.Sum256(pub)
	var pkh [20]byte
	copy(pkh[:], sum[:20])
	return pkh
}
