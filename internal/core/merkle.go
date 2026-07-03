package core

// MerkleRoot resume os txids do bloco num único hash de 32 bytes: pares de
// hashes são combinados (SHA-256d) nível a nível até sobrar um; nível ímpar
// duplica o último elemento, como no Bitcoin. Lista vazia retorna o hash
// zero — um bloco sem coinbase é inválido e é rejeitado na validação da
// chain, não aqui.
func MerkleRoot(txids [][32]byte) [32]byte {
	if len(txids) == 0 {
		return [32]byte{}
	}
	level := make([][32]byte, len(txids))
	copy(level, txids)
	for len(level) > 1 {
		if len(level)%2 == 1 {
			level = append(level, level[len(level)-1])
		}
		next := make([][32]byte, 0, len(level)/2)
		for i := 0; i < len(level); i += 2 {
			var pair [64]byte
			copy(pair[:32], level[i][:])
			copy(pair[32:], level[i+1][:])
			next = append(next, sha256d(pair[:]))
		}
		level = next
	}
	return level[0]
}
