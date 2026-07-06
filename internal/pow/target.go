package pow

import (
	"errors"
	"math/big"

	"pandabk_coin/internal/core"
	"pandabk_coin/internal/params"
)

// Dificuldade em três formas: nBits (uint32 compacto, formato do Bitcoin,
// cabe no header), target (big.Int — o hash de PoW deve ficar abaixo dele) e
// trabalho (2^256/(target+1) — quanto menor o target, mais tentativas um
// bloco representa; a soma disso é o critério de fork choice).

var (
	ErrInvalidBits      = errors.New("pow: nBits inválido")
	ErrTargetAboveLimit = errors.New("pow: target acima do limite da rede (dificuldade baixa demais)")
	ErrHashAboveTarget  = errors.New("pow: hash Argon2id acima do target")
)

var maxHashSpace = new(big.Int).Lsh(big.NewInt(1), 256)

// CompactToTarget expande nBits (1 byte expoente + 3 bytes mantissa) para o
// target. Retorna nil para encodings negativos/malformados.
func CompactToTarget(bits uint32) *big.Int {
	if bits&0x00800000 != 0 {
		return nil
	}
	mantissa := int64(bits & 0x007fffff)
	exponent := uint(bits >> 24)
	t := big.NewInt(mantissa)
	if exponent <= 3 {
		return t.Rsh(t, 8*(3-exponent))
	}
	return t.Lsh(t, 8*(exponent-3))
}

// TargetToCompact normaliza um target de volta ao formato nBits.
func TargetToCompact(t *big.Int) uint32 {
	if t == nil || t.Sign() <= 0 {
		return 0
	}
	size := uint32((t.BitLen() + 7) / 8)
	var mantissa uint32
	if size <= 3 {
		mantissa = uint32(t.Uint64()) << (8 * (3 - size))
	} else {
		shifted := new(big.Int).Rsh(t, uint(8*(size-3)))
		mantissa = uint32(shifted.Uint64())
	}
	if mantissa&0x00800000 != 0 {
		mantissa >>= 8
		size++
	}
	return size<<24 | mantissa
}

// Difficulty expressa a dificuldade em escala humana: quantas vezes mais
// difícil que o mínimo da rede (PowLimitBits) esse nBits é. 1.0 = dificuldade
// mínima; 4.0 = precisa de 4× mais tentativas por bloco.
func Difficulty(bits uint32, p params.Params) float64 {
	target := CompactToTarget(bits)
	limit := CompactToTarget(p.PowLimitBits)
	if target == nil || limit == nil || target.Sign() <= 0 {
		return 0
	}
	f, _ := new(big.Rat).SetFrac(limit, target).Float64()
	return f
}

// BlockWork é quanto trabalho esperado um bloco com esses nBits representa.
func BlockWork(bits uint32) *big.Int {
	target := CompactToTarget(bits)
	if target == nil || target.Sign() <= 0 {
		return new(big.Int)
	}
	denom := new(big.Int).Add(target, big.NewInt(1))
	return new(big.Int).Div(maxHashSpace, denom)
}

// CheckProofOfWork valida os nBits do header contra o limite da rede e o
// hash Argon2id contra o target. É a checagem cara (~Argon2Mem KiB de RAM);
// na validação de um bloco ela deve vir por último, depois das checagens
// baratas.
func CheckProofOfWork(h *core.Header, p params.Params) error {
	target := CompactToTarget(h.Bits)
	if target == nil || target.Sign() <= 0 {
		return ErrInvalidBits
	}
	limit := CompactToTarget(p.PowLimitBits)
	if limit == nil || target.Cmp(limit) > 0 {
		return ErrTargetAboveLimit
	}
	hash := PowHash(h.Bytes(), p)
	if new(big.Int).SetBytes(hash[:]).Cmp(target) > 0 {
		return ErrHashAboveTarget
	}
	return nil
}
