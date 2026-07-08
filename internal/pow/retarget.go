package pow

import (
	"math/big"
	"time"

	"zhu/internal/params"
)

// NextBits calcula a dificuldade da próxima época (retarget estilo Bitcoin):
// o target atual é escalado pela razão entre o tempo real que a época levou
// (last-first, timestamps do primeiro e do último bloco da janela) e o tempo
// esperado (RetargetInterval × TargetSpacing). O ajuste é limitado a
// [1/MaxClamp, MaxClamp]× por época e o target nunca sobe além de
// PowLimitBits (a dificuldade mínima da rede).
func NextBits(first, last int64, currentBits uint32, p params.Params) uint32 {
	expected := int64(p.RetargetInterval) * int64(p.TargetSpacing/time.Second)
	if expected < 1 {
		expected = 1
	}
	minActual := expected / p.MaxClamp
	if minActual < 1 {
		// Janelas curtas + timestamps em segundos podem dar 0 na divisão
		// inteira; um actual de 0 zeraria o target (dificuldade impossível).
		minActual = 1
	}
	actual := last - first
	if actual < minActual {
		actual = minActual
	}
	if actual > expected*p.MaxClamp {
		actual = expected * p.MaxClamp
	}

	target := CompactToTarget(currentBits)
	if target == nil || target.Sign() <= 0 {
		return p.PowLimitBits
	}
	target.Mul(target, big.NewInt(actual))
	target.Div(target, big.NewInt(expected))

	limit := CompactToTarget(p.PowLimitBits)
	if limit != nil && target.Cmp(limit) > 0 {
		target = limit
	}
	if target.Sign() <= 0 {
		target = big.NewInt(1)
	}
	return TargetToCompact(target)
}
