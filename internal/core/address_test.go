package core

import (
	"strings"
	"testing"
)

func TestAddressRoundTrip(t *testing.T) {
	priv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	pub := CompressPubKey(&priv.PublicKey)
	if len(pub) != PubKeySize {
		t.Fatalf("pubkey comprimida tem %d bytes, want %d", len(pub), PubKeySize)
	}

	addr := PubKeyToAddress(pub)
	if !strings.HasPrefix(addr, "P") {
		t.Fatalf("endereço %q deveria começar com P", addr)
	}

	pkh, err := DecodeAddress(addr)
	if err != nil {
		t.Fatal(err)
	}
	if pkh != HashPubKey(pub) {
		t.Fatal("PubKeyHash decodificado não bate com o original")
	}
}

func TestDecodeAddressRejectsCorruption(t *testing.T) {
	addr := AddressFromPKH([20]byte{1, 2, 3})

	// Um caractere trocado → checksum falha.
	corrupted := []byte(addr)
	if corrupted[len(corrupted)-1] == 'x' {
		corrupted[len(corrupted)-1] = 'y'
	} else {
		corrupted[len(corrupted)-1] = 'x'
	}
	if _, err := DecodeAddress(string(corrupted)); err == nil {
		t.Fatal("endereço corrompido deveria falhar no checksum")
	}

	// Caractere fora do alfabeto base58 (0, O, I, l não existem).
	if _, err := DecodeAddress("P0OIl"); err == nil {
		t.Fatal("caractere inválido deveria falhar")
	}

	// Curto demais.
	if _, err := DecodeAddress("P"); err == nil {
		t.Fatal("endereço curto demais deveria falhar")
	}
}

func TestSignAndVerify(t *testing.T) {
	priv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	pub := CompressPubKey(&priv.PublicKey)
	hash := sha256d([]byte("mensagem"))

	sig, err := SignHash(priv, hash)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifySignature(pub, hash, sig) {
		t.Fatal("assinatura legítima deveria verificar")
	}

	other := sha256d([]byte("outra mensagem"))
	if VerifySignature(pub, other, sig) {
		t.Fatal("assinatura de outra mensagem não pode verificar")
	}

	otherKey, _ := GenerateKey()
	if VerifySignature(CompressPubKey(&otherKey.PublicKey), hash, sig) {
		t.Fatal("outra chave não pode verificar a assinatura")
	}
}
