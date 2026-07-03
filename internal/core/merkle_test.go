package core

import "testing"

func TestMerkleRoot(t *testing.T) {
	a := sha256d([]byte("a"))
	b := sha256d([]byte("b"))
	c := sha256d([]byte("c"))

	// Vazio → hash zero (bloco sem txs é rejeitado na chain, não aqui).
	if MerkleRoot(nil) != ([32]byte{}) {
		t.Fatal("merkle de lista vazia deveria ser o hash zero")
	}

	// 1 tx → root == txid.
	if MerkleRoot([][32]byte{a}) != a {
		t.Fatal("merkle de 1 tx deveria ser o próprio txid")
	}

	// 2 txs → sha256d(a‖b), calculado manualmente.
	var pair [64]byte
	copy(pair[:32], a[:])
	copy(pair[32:], b[:])
	if MerkleRoot([][32]byte{a, b}) != sha256d(pair[:]) {
		t.Fatal("merkle de 2 txs divergiu do cálculo manual")
	}

	// 3 txs → nível ímpar duplica o último: root(a,b,c) == root(a,b,c,c).
	if MerkleRoot([][32]byte{a, b, c}) != MerkleRoot([][32]byte{a, b, c, c}) {
		t.Fatal("nível ímpar deveria duplicar o último elemento")
	}

	// Qualquer tx alterada muda a raiz.
	if MerkleRoot([][32]byte{a, b}) == MerkleRoot([][32]byte{a, c}) {
		t.Fatal("trocar uma tx deveria mudar a raiz")
	}

	// A entrada não pode ser mutada (nível ímpar dá append no slice).
	in := [][32]byte{a, b, c}
	MerkleRoot(in)
	if in[0] != a || in[1] != b || in[2] != c {
		t.Fatal("MerkleRoot mutou o slice de entrada")
	}
}
