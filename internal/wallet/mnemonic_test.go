package wallet

import (
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// Vetor oficial BIP39 (repo Trezor): entropia zero → frase conhecida, e a
// seed com passphrase "TREZOR" tem que bater byte a byte — prova de que
// qualquer implementação da spec (Python incluso) chega no mesmo lugar.
func TestBIP39OfficialVector(t *testing.T) {
	var entropy [16]byte // 0x000...0
	phrase := encodeMnemonic(entropy)
	want := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	if phrase != want {
		t.Fatalf("frase = %q, esperava o vetor oficial", phrase)
	}
	seed := seedFromMnemonic(phrase, "TREZOR")
	wantSeed := "c55257c360c07c72029aebc1b53c05ed0362ada38ead3e3e9efa3708e53495531f09a6987599d18264c1e1c92f2cf141630c7a3c4ab7c81b2f001698e7463b04"
	if hex.EncodeToString(seed) != wantSeed {
		t.Fatalf("seed = %x", seed)
	}
}

// Vetor oficial SLIP-0010 (curva Nist256p1, test vector 1): da seed
// 000102...0f a chave-mestra tem que ser exatamente esta.
func TestSLIP10MasterKeyVector(t *testing.T) {
	seed, _ := hex.DecodeString("000102030405060708090a0b0c0d0e0f")
	priv, err := masterKeyP256(seed)
	if err != nil {
		t.Fatalf("masterKeyP256: %v", err)
	}
	want := "612091aaa12e22dd2abef664f8a01a82cae99ad7441b7ef8110424915c268bc2"
	got := hex.EncodeToString(priv.D.FillBytes(make([]byte, 32)))
	if got != want {
		t.Fatalf("chave-mestra = %s, esperava o vetor SLIP-0010", got)
	}
}

func TestMnemonicRoundTripRestore(t *testing.T) {
	dir := t.TempDir()
	w1, phrase, err := NewWithMnemonic(filepath.Join(dir, "a.json"))
	if err != nil {
		t.Fatalf("NewWithMnemonic: %v", err)
	}
	if len(strings.Fields(phrase)) != 12 {
		t.Fatalf("frase com %d palavras", len(strings.Fields(phrase)))
	}

	// As MESMAS palavras (até com caixa e espaços bagunçados) recuperam o
	// MESMO endereço em outro arquivo/máquina.
	messy := "  " + strings.ToUpper(phrase) + "  "
	w2, err := Restore(filepath.Join(dir, "b.json"), messy)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if w2.Address() != w1.Address() {
		t.Fatalf("endereços divergem: %s != %s", w2.Address(), w1.Address())
	}

	// O wallet.json restaurado passa no Load normal — e devolve as MESMAS
	// palavras (a frase fica gravada no arquivo e é validada contra a chave).
	loaded, err := Load(filepath.Join(dir, "b.json"))
	if err != nil {
		t.Fatalf("Load do restaurado: %v", err)
	}
	if loaded.Mnemonic() != phrase {
		t.Fatalf("Mnemonic() = %q, esperava a frase original", loaded.Mnemonic())
	}

	// Restore nunca sobrescreve uma wallet existente.
	if _, err := Restore(filepath.Join(dir, "a.json"), phrase); !errors.Is(err, ErrWalletExists) {
		t.Fatalf("sobrescrever deveria dar ErrWalletExists, veio %v", err)
	}
}

func TestMnemonicRejectsBadPhrases(t *testing.T) {
	abandon11 := strings.Repeat("abandon ", 11)
	cases := map[string]string{
		"11 palavras":       strings.TrimSpace(abandon11),
		"palavra inventada": abandon11 + "pandinha",
		// O vetor oficial termina em "about"; 12× "abandon" tem checksum errado.
		"checksum quebrado": abandon11 + "abandon",
	}
	for name, phrase := range cases {
		if _, err := normalizeMnemonic(phrase); !errors.Is(err, ErrBadMnemonic) {
			t.Errorf("%s: esperava ErrBadMnemonic, veio %v", name, err)
		}
	}
}
