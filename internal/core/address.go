package core

import (
	"bytes"
	"errors"
	"math/big"
)

// Endereço = Base58Check(versão 0x37 ‖ PubKeyHash ‖ checksum). A versão 0x37
// faz todo endereço PANDA começar com a letra P; o checksum (4 bytes de
// SHA-256d) pega erro de digitação antes de qualquer moeda ser enviada.

const AddressVersion = 0x37

var (
	ErrInvalidAddress = errors.New("core: endereço inválido")
	ErrBadChecksum    = errors.New("core: checksum do endereço não confere")
)

const b58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

var b58Index = func() [256]int8 {
	var idx [256]int8
	for i := range idx {
		idx[i] = -1
	}
	for i, c := range b58Alphabet {
		idx[c] = int8(i)
	}
	return idx
}()

func base58Encode(input []byte) string {
	zeros := 0
	for zeros < len(input) && input[zeros] == 0 {
		zeros++
	}
	num := new(big.Int).SetBytes(input)
	radix := big.NewInt(58)
	mod := new(big.Int)
	var out []byte
	for num.Sign() > 0 {
		num.DivMod(num, radix, mod)
		out = append(out, b58Alphabet[mod.Int64()])
	}
	for i := 0; i < zeros; i++ {
		out = append(out, b58Alphabet[0])
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return string(out)
}

func base58Decode(s string) ([]byte, error) {
	zeros := 0
	for zeros < len(s) && s[zeros] == b58Alphabet[0] {
		zeros++
	}
	num := new(big.Int)
	radix := big.NewInt(58)
	for _, c := range []byte(s) {
		d := b58Index[c]
		if d < 0 {
			return nil, ErrInvalidAddress
		}
		num.Mul(num, radix)
		num.Add(num, big.NewInt(int64(d)))
	}
	return append(make([]byte, zeros), num.Bytes()...), nil
}

// AddressFromPKH monta o endereço de um PubKeyHash.
func AddressFromPKH(pkh [20]byte) string {
	payload := append([]byte{AddressVersion}, pkh[:]...)
	check := sha256d(payload)
	return base58Encode(append(payload, check[:4]...))
}

// PubKeyToAddress deriva o endereço da chave pública comprimida.
func PubKeyToAddress(pub []byte) string {
	return AddressFromPKH(HashPubKey(pub))
}

// DecodeAddress valida versão e checksum e devolve o PubKeyHash.
func DecodeAddress(s string) ([20]byte, error) {
	var pkh [20]byte
	raw, err := base58Decode(s)
	if err != nil {
		return pkh, err
	}
	if len(raw) != 1+20+4 {
		return pkh, ErrInvalidAddress
	}
	if raw[0] != AddressVersion {
		return pkh, ErrInvalidAddress
	}
	check := sha256d(raw[:21])
	if !bytes.Equal(check[:4], raw[21:]) {
		return pkh, ErrBadChecksum
	}
	copy(pkh[:], raw[1:21])
	return pkh, nil
}
