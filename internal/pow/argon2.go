// Package pow implementa o proof of work memory-hard da PANDA: o hash
// Argon2id sobre o header, a aritmética de dificuldade (nBits ↔ target ↔
// trabalho) e o ajuste periódico de dificuldade. Funções puras.
// Ver internal/pow/SPEC.md.
package pow

import (
	"golang.org/x/crypto/argon2"

	"pandabk_coin/internal/params"
)

// powSalt fixa o domínio do hash de PoW: muda o salt, muda a rede.
var powSalt = []byte("pandabk/pow/v1")

// PowHash é o hash caro de mineração: Argon2id sobre os 96 bytes do header,
// forçando o preenchimento de Argon2Mem KiB de RAM por tentativa. É separado
// do ID do bloco (SHA-256d, barato) — a chain indexa pelo ID e só paga o
// Argon2 na checagem de PoW.
func PowHash(header []byte, p params.Params) [32]byte {
	out := argon2.IDKey(header, powSalt, p.Argon2Time, p.Argon2Mem, p.Argon2Threads, 32)
	var h [32]byte
	copy(h[:], out)
	return h
}
