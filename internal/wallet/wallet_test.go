package wallet

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zhu/internal/core"
)

func newTestWallet(t *testing.T) (*Wallet, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wallet.json")
	w, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return w, path
}

func TestNewLoadRoundTripAndPerms(t *testing.T) {
	w, path := newTestWallet(t)

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("wallet.json com permissão %04o, esperava 0600", perm)
	}
	if !strings.HasPrefix(w.Address(), "P") {
		t.Fatalf("endereço %q deveria começar com P", w.Address())
	}
	// endereço round-tripa no decode do core
	pkh, err := core.DecodeAddress(w.Address())
	if err != nil || pkh != w.PubKeyHash() {
		t.Fatalf("DecodeAddress(%q) = %x, %v; esperava %x", w.Address(), pkh, err, w.PubKeyHash())
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Address() != w.Address() || loaded.PubKeyHash() != w.PubKeyHash() {
		t.Fatalf("Load devolveu outra identidade: %q vs %q", loaded.Address(), w.Address())
	}
}

func TestNewNeverOverwrites(t *testing.T) {
	_, path := newTestWallet(t)
	if _, err := New(path); !errors.Is(err, ErrWalletExists) {
		t.Fatalf("err = %v, esperava ErrWalletExists", err)
	}
}

func TestLoadRejectsBadPermsAndCorruption(t *testing.T) {
	_, path := newTestWallet(t)

	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); !errors.Is(err, ErrBadPerm) {
		t.Fatalf("err = %v, esperava ErrBadPerm", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}

	// endereço editado à mão não deriva mais da chave privada
	data, _ := os.ReadFile(path)
	tampered := strings.Replace(string(data), `"address": "P`, `"address": "Q`, 1)
	if err := os.WriteFile(path, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("err = %v, esperava ErrCorrupt", err)
	}
}

func TestBuildTxSelectsSignsAndMakesChange(t *testing.T) {
	w, _ := newTestWallet(t)
	utxos := []UTXO{
		{Prev: core.OutPoint{TxID: [32]byte{1}, Index: 0}, Value: 10_000_000},
		{Prev: core.OutPoint{TxID: [32]byte{2}, Index: 0}, Value: 50_000_000},
		{Prev: core.OutPoint{TxID: [32]byte{3}, Index: 1}, Value: 2_000_000},
	}
	toKey, _ := core.GenerateKey()
	to := core.HashPubKey(core.CompressPubKey(&toKey.PublicKey))

	const amount, feeRate = 30_000_000, 10
	tx, err := w.BuildTx(utxos, to, amount, feeRate)
	if err != nil {
		t.Fatalf("BuildTx: %v", err)
	}

	// largest-first: o UTXO de 50M sozinho cobre valor + taxa
	if len(tx.Ins) != 1 || tx.Ins[0].Prev.TxID != ([32]byte{2}) {
		t.Fatalf("seleção largest-first deveria usar só o UTXO de 50M: %+v", tx.Ins)
	}
	if tx.Outs[0].Value != amount || tx.Outs[0].PubKeyHash != to {
		t.Fatalf("output do destinatário errado: %+v", tx.Outs[0])
	}
	if len(tx.Outs) != 2 || tx.Outs[1].PubKeyHash != w.PubKeyHash() {
		t.Fatalf("faltou o troco de volta pra wallet: %+v", tx.Outs)
	}
	fee := feeRate * estimateSize(1, 2)
	if got, want := tx.Outs[1].Value, uint64(50_000_000)-amount-fee; got != want {
		t.Fatalf("troco = %d, esperava %d", got, want)
	}

	// a assinatura passa na verificação do core (mesma checagem da chain)
	sigHash, err := tx.SigHash(0, w.PubKeyHash())
	if err != nil {
		t.Fatal(err)
	}
	if !core.VerifySignature(tx.Ins[0].PubKey, sigHash, tx.Ins[0].Sig) {
		t.Fatal("assinatura do input não verifica")
	}
	if core.HashPubKey(tx.Ins[0].PubKey) != w.PubKeyHash() {
		t.Fatal("pubkey do input não corresponde ao dono dos UTXOs")
	}
}

func TestBuildTxMultipleInputs(t *testing.T) {
	w, _ := newTestWallet(t)
	utxos := []UTXO{
		{Prev: core.OutPoint{TxID: [32]byte{1}}, Value: 60_000},
		{Prev: core.OutPoint{TxID: [32]byte{2}}, Value: 50_000},
	}
	tx, err := w.BuildTx(utxos, [20]byte{9}, 100_000, 1)
	if err != nil {
		t.Fatalf("BuildTx: %v", err)
	}
	if len(tx.Ins) != 2 {
		t.Fatalf("deveria precisar dos dois UTXOs, usou %d", len(tx.Ins))
	}
	for i := range tx.Ins {
		sigHash, _ := tx.SigHash(i, w.PubKeyHash())
		if !core.VerifySignature(tx.Ins[i].PubKey, sigHash, tx.Ins[i].Sig) {
			t.Fatalf("assinatura do input %d não verifica", i)
		}
	}
}

func TestBuildTxErrors(t *testing.T) {
	w, _ := newTestWallet(t)
	utxos := []UTXO{{Prev: core.OutPoint{TxID: [32]byte{1}}, Value: 1_000}}

	if _, err := w.BuildTx(utxos, [20]byte{9}, 0, 1); !errors.Is(err, ErrZeroAmount) {
		t.Fatalf("amount 0: err = %v, esperava ErrZeroAmount", err)
	}
	if _, err := w.BuildTx(utxos, [20]byte{9}, 1_000_000, 1); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("sem fundos: err = %v, esperava ErrInsufficientFunds", err)
	}
	// cobre o valor mas não a taxa
	if _, err := w.BuildTx(utxos, [20]byte{9}, 1_000, 10); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("sem fundos pra taxa: err = %v, esperava ErrInsufficientFunds", err)
	}
}

func TestBuildTxDustChangeBecomesFee(t *testing.T) {
	w, _ := newTestWallet(t)
	const feeRate = 1
	fee := feeRate * estimateSize(1, 2)
	// sobra proposital menor que a poeira: 500 < DustLimit
	value := 100_000 + fee + 500
	utxos := []UTXO{{Prev: core.OutPoint{TxID: [32]byte{1}}, Value: value}}

	tx, err := w.BuildTx(utxos, [20]byte{9}, 100_000, feeRate)
	if err != nil {
		t.Fatalf("BuildTx: %v", err)
	}
	if len(tx.Outs) != 1 {
		t.Fatalf("troco-poeira não deveria virar output: %+v", tx.Outs)
	}
}
