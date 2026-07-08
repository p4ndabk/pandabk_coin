package wallet

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	_ "embed"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

// As 12 palavras são o padrão BIP39: 128 bits de aleatoriedade + 4 bits de
// checksum, codificados em 12 índices de 11 bits sobre uma lista fixa de
// 2048 palavras. Da frase deriva-se uma seed (PBKDF2-SHA512) e, dela, a
// chave P-256 pelo SLIP-0010 (chave-mestra da curva Nist256p1) — tudo
// padrão aberto: as mesmas palavras recuperam a mesma carteira em Go,
// Python ou qualquer implementação das specs.

//go:embed bip39_english.txt
var bip39Raw string

var (
	ErrBadMnemonic = errors.New("wallet: frase de 12 palavras inválida — confira a grafia (lista BIP39 em inglês) e a ordem")

	bip39Words []string
	bip39Index map[string]int
)

func init() {
	bip39Words = strings.Fields(bip39Raw)
	if len(bip39Words) != 2048 {
		panic(fmt.Sprintf("wallet: lista BIP39 embutida com %d palavras, esperava 2048", len(bip39Words)))
	}
	bip39Index = make(map[string]int, 2048)
	for i, w := range bip39Words {
		bip39Index[w] = i
	}
}

// NewMnemonic sorteia 128 bits (crypto/rand) e os codifica nas 12 palavras.
func NewMnemonic() (string, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", err
	}
	return encodeMnemonic(entropy), nil
}

func encodeMnemonic(entropy [16]byte) string {
	sum := sha256.Sum256(entropy[:])
	// 132 bits = entropia (128) ‖ 4 bits altos do SHA-256 (checksum)
	bits := new(big.Int).SetBytes(entropy[:])
	bits.Lsh(bits, 4)
	bits.Or(bits, big.NewInt(int64(sum[0]>>4)))

	words := make([]string, 12)
	mask := big.NewInt(2047)
	for i := 11; i >= 0; i-- {
		idx := new(big.Int).And(bits, mask).Int64()
		words[i] = bip39Words[idx]
		bits.Rsh(bits, 11)
	}
	return strings.Join(words, " ")
}

// normalizeMnemonic aceita espaços extras e maiúsculas; devolve a frase
// canônica (minúsculas, espaço único) validada: 12 palavras da lista e
// checksum correto.
func normalizeMnemonic(phrase string) (string, error) {
	words := strings.Fields(strings.ToLower(strings.TrimSpace(phrase)))
	if len(words) != 12 {
		return "", fmt.Errorf("%w (vieram %d palavras, precisa de 12)", ErrBadMnemonic, len(words))
	}
	bits := new(big.Int)
	for _, w := range words {
		idx, ok := bip39Index[w]
		if !ok {
			return "", fmt.Errorf("%w: %q não está na lista", ErrBadMnemonic, w)
		}
		bits.Lsh(bits, 11)
		bits.Or(bits, big.NewInt(int64(idx)))
	}
	check := byte(new(big.Int).And(bits, big.NewInt(15)).Int64())
	var entropy [16]byte
	new(big.Int).Rsh(bits, 4).FillBytes(entropy[:])
	sum := sha256.Sum256(entropy[:])
	if sum[0]>>4 != check {
		return "", fmt.Errorf("%w: checksum não confere (alguma palavra trocada?)", ErrBadMnemonic)
	}
	return strings.Join(words, " "), nil
}

// seedFromMnemonic é o BIP39 seed: PBKDF2-HMAC-SHA512, 2048 iterações,
// salt "mnemonic"+passphrase. A ZHU usa passphrase vazia.
func seedFromMnemonic(mnemonic, passphrase string) []byte {
	return pbkdf2.Key([]byte(mnemonic), []byte("mnemonic"+passphrase), 2048, 64, sha512.New)
}

// masterKeyP256 é a chave-mestra SLIP-0010 da curva Nist256p1:
// HMAC-SHA512("Nist256p1 seed", seed) → IL é a chave; se IL for inválida
// para a curva (≥ n ou zero), repete com o próprio resultado — exatamente
// como a spec manda, para qualquer implementação chegar na mesma chave.
func masterKeyP256(seed []byte) (*ecdsa.PrivateKey, error) {
	curve := elliptic.P256()
	n := curve.Params().N
	data := seed
	for range [16]struct{}{} { // na prática sai de primeira; limite anti-loop
		mac := hmac.New(sha512.New, []byte("Nist256p1 seed"))
		mac.Write(data)
		sum := mac.Sum(nil)
		k := new(big.Int).SetBytes(sum[:32])
		if k.Sign() != 0 && k.Cmp(n) < 0 {
			priv := &ecdsa.PrivateKey{D: k}
			priv.Curve = curve
			priv.X, priv.Y = curve.ScalarBaseMult(sum[:32])
			return priv, nil
		}
		data = sum
	}
	return nil, errors.New("wallet: seed inválida para a curva (probabilidade ~2^-32 por tentativa — seed suspeita)")
}

// keyFromMnemonic liga as pontas: frase validada → seed → chave P-256.
func keyFromMnemonic(phrase string) (*ecdsa.PrivateKey, string, error) {
	mnemonic, err := normalizeMnemonic(phrase)
	if err != nil {
		return nil, "", err
	}
	priv, err := masterKeyP256(seedFromMnemonic(mnemonic, ""))
	if err != nil {
		return nil, "", err
	}
	return priv, mnemonic, nil
}

// NewWithMnemonic cria a wallet a partir de 12 palavras NOVAS e devolve a
// frase — o chamador é obrigado a mostrá-la ao dono UMA vez ("anote num
// papel"). O wallet.json continua sendo escrito (0600, nunca sobrescreve):
// ele é o cache local; as palavras são o backup humano.
func NewWithMnemonic(path string) (*Wallet, string, error) {
	phrase, err := NewMnemonic()
	if err != nil {
		return nil, "", err
	}
	priv, _, err := keyFromMnemonic(phrase)
	if err != nil {
		return nil, "", err
	}
	w, err := saveKey(path, priv, phrase)
	if err != nil {
		return nil, "", err
	}
	return w, phrase, nil
}

// Restore reconstrói a wallet a partir das 12 palavras e grava o
// wallet.json em path (recusa arquivo existente, como New — proteger a
// chave atual vem antes de conveniência).
func Restore(path, phrase string) (*Wallet, error) {
	priv, mnemonic, err := keyFromMnemonic(phrase)
	if err != nil {
		return nil, err
	}
	return saveKey(path, priv, mnemonic)
}
