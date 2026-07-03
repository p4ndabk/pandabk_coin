package core

import "testing"

func TestCoinbaseStructure(t *testing.T) {
	cb := NewCoinbase(42, 50*100_000_000, [20]byte{1}, []byte("w0"))
	if !cb.IsCoinbase() {
		t.Fatal("NewCoinbase deveria produzir uma coinbase")
	}
	regular := sampleTx()
	if regular.IsCoinbase() {
		t.Fatal("tx comum não pode ser coinbase")
	}

	// Alturas diferentes → txids diferentes (height embutida na PubKey).
	other := NewCoinbase(43, 50*100_000_000, [20]byte{1}, []byte("w0"))
	if cb.TxID() == other.TxID() {
		t.Fatal("coinbases de alturas diferentes precisam ter txids diferentes")
	}
}

func TestSigHashCommitsToEverything(t *testing.T) {
	tx := sampleTx()
	pkh := [20]byte{7}

	base, err := tx.SigHash(0, pkh)
	if err != nil {
		t.Fatal(err)
	}

	// Sighash é estável: não depende da Sig/PubKey preenchidas no input.
	signed := sampleTx()
	signed.Ins[0].Sig = []byte("assinatura qualquer")
	again, _ := signed.SigHash(0, pkh)
	if again != base {
		t.Fatal("sighash não pode depender do campo Sig")
	}

	// Mas muda se o output mudar (compromete com o destino)...
	tampered := sampleTx()
	tampered.Outs[0].Value++
	h, _ := tampered.SigHash(0, pkh)
	if h == base {
		t.Fatal("mudar um output tem que mudar o sighash")
	}

	// ...se o outpoint mudar...
	tampered = sampleTx()
	tampered.Ins[0].Prev.Index++
	h, _ = tampered.SigHash(0, pkh)
	if h == base {
		t.Fatal("mudar o outpoint tem que mudar o sighash")
	}

	// ...e se o PubKeyHash do output gasto for outro.
	h, _ = tx.SigHash(0, [20]byte{8})
	if h == base {
		t.Fatal("outro prevPKH tem que mudar o sighash")
	}

	if _, err := tx.SigHash(5, pkh); err == nil {
		t.Fatal("input fora da faixa deveria dar erro")
	}
}

func TestSignedInputVerifies(t *testing.T) {
	priv, _ := GenerateKey()
	pub := CompressPubKey(&priv.PublicKey)
	pkh := HashPubKey(pub)

	tx := sampleTx()
	sighash, err := tx.SigHash(0, pkh)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := SignHash(priv, sighash)
	if err != nil {
		t.Fatal(err)
	}
	tx.Ins[0].Sig = sig
	tx.Ins[0].PubKey = pub

	// O verificador refaz o sighash a partir da tx assinada e confere.
	recomputed, _ := tx.SigHash(0, pkh)
	if !VerifySignature(tx.Ins[0].PubKey, recomputed, tx.Ins[0].Sig) {
		t.Fatal("input assinado deveria verificar")
	}

	// Adulterar o valor do output depois de assinar quebra a assinatura.
	tx.Outs[0].Value++
	broken, _ := tx.SigHash(0, pkh)
	if VerifySignature(tx.Ins[0].PubKey, broken, tx.Ins[0].Sig) {
		t.Fatal("tx adulterada não pode verificar")
	}
}
