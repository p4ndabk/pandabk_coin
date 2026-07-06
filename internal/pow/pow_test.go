package pow

import (
	"math/big"
	"testing"
	"time"

	"pandabk_coin/internal/core"
	"pandabk_coin/internal/params"
)

func TestCompactRoundTrip(t *testing.T) {
	// Vetores normalizados conhecidos (inclui o nBits do gênesis do Bitcoin).
	for _, bits := range []uint32{0x1d00ffff, 0x20010000, 0x207fffff, 0x03123456} {
		target := CompactToTarget(bits)
		if target == nil {
			t.Fatalf("CompactToTarget(%#x) = nil", bits)
		}
		if got := TargetToCompact(target); got != bits {
			t.Errorf("round-trip de %#x virou %#x", bits, got)
		}
	}

	// 0x20010000 tem que ser exatamente 2^248 (o limite devnet documentado).
	want := new(big.Int).Lsh(big.NewInt(1), 248)
	if CompactToTarget(0x20010000).Cmp(want) != 0 {
		t.Fatal("0x20010000 deveria expandir para 2^248")
	}

	// Bit de sinal ligado = encoding inválido.
	if CompactToTarget(0x20800000) != nil {
		t.Fatal("nBits com bit de sinal deveria ser inválido")
	}
}

func TestDifficultyHumanScale(t *testing.T) {
	p := params.DevNet()
	// No limite da rede a dificuldade é exatamente 1.
	if got := Difficulty(p.PowLimitBits, p); got != 1.0 {
		t.Fatalf("Difficulty(PowLimit) = %v, esperava 1.0", got)
	}
	// Target 4× menor (2^246) = dificuldade 4×.
	quarter := TargetToCompact(new(big.Int).Lsh(big.NewInt(1), 246))
	if got := Difficulty(quarter, p); got != 4.0 {
		t.Fatalf("Difficulty(2^246) = %v, esperava 4.0", got)
	}
	// nBits inválido não pode virar pânico nem infinito.
	if got := Difficulty(0x20800000, p); got != 0 {
		t.Fatalf("Difficulty(inválido) = %v, esperava 0", got)
	}
}

func TestBlockWorkGrowsWithDifficulty(t *testing.T) {
	easy := BlockWork(0x207fffff) // target ~2^255
	hard := BlockWork(0x20010000) // target 2^248
	if hard.Cmp(easy) <= 0 {
		t.Fatal("target menor (mais difícil) tem que representar mais trabalho")
	}
	if BlockWork(0x20800000).Sign() != 0 {
		t.Fatal("nBits inválido deveria representar trabalho zero")
	}
}

func TestPowHashIsDeterministic(t *testing.T) {
	p := params.TestNet()
	h := core.Header{Version: 1, Bits: p.PowLimitBits, Nonce: 1}
	a := PowHash(h.Bytes(), p)
	b := PowHash(h.Bytes(), p)
	if a != b {
		t.Fatal("PowHash tem que ser determinístico")
	}
	h.Nonce = 2
	if PowHash(h.Bytes(), p) == a {
		t.Fatal("nonce diferente tem que mudar o hash")
	}
}

func TestCheckProofOfWork(t *testing.T) {
	p := params.TestNet()

	// Com o limite do perfil test (~2^255), metade dos hashes passa: minerar
	// leva pouquíssimas tentativas mesmo no CI.
	h := core.Header{Version: 1, Timestamp: 1783036800, Bits: p.PowLimitBits}
	mined := false
	for nonce := uint64(0); nonce < 64; nonce++ {
		h.Nonce = nonce
		if err := CheckProofOfWork(&h, p); err == nil {
			mined = true
			break
		}
	}
	if !mined {
		t.Fatal("64 tentativas com target 2^255 deveriam achar um bloco (p ≈ 1-2^-64)")
	}

	// Target absurdamente difícil (2^160): nenhum hash casual passa.
	h.Bits = 0x15010000
	h.Nonce = 0
	if err := CheckProofOfWork(&h, p); err != ErrHashAboveTarget {
		t.Fatalf("esperava ErrHashAboveTarget, veio %v", err)
	}

	// nBits mais fácil que o limite da rede é rejeitado antes do Argon2.
	easier := params.DevNet()
	easier.Argon2Mem = 1024 // não importa: falha antes do hash
	h.Bits = 0x207fffff     // acima do limite devnet 0x20010000
	if err := CheckProofOfWork(&h, easier); err != ErrTargetAboveLimit {
		t.Fatalf("esperava ErrTargetAboveLimit, veio %v", err)
	}

	h.Bits = 0x20800000
	if err := CheckProofOfWork(&h, p); err != ErrInvalidBits {
		t.Fatalf("esperava ErrInvalidBits, veio %v", err)
	}
}

func TestNextBitsRetarget(t *testing.T) {
	p := params.TestNet()
	p.PowLimitBits = 0x207fffff
	start := uint32(0x1d00ffff) // bem abaixo do limite: há espaço p/ subir e descer
	expected := int64(p.RetargetInterval) * int64(p.TargetSpacing/time.Second)

	// Época no tempo exato → dificuldade não muda.
	if got := NextBits(0, expected, start, p); got != start {
		t.Fatalf("época pontual não deveria mudar bits: %#x → %#x", start, got)
	}

	// Blocos 2× rápidos → target cai pela metade (2× mais difícil).
	half := NextBits(0, expected/2, start, p)
	wantHalf := new(big.Int).Rsh(CompactToTarget(start), 1)
	if CompactToTarget(half).Cmp(wantHalf) != 0 {
		t.Fatalf("época 2× rápida deveria dar target/2, veio %#x", half)
	}

	// Rápido além do clamp (10×) → trava em 4× mais difícil.
	clamped := NextBits(0, expected/10, start, p)
	wantQuarter := new(big.Int).Rsh(CompactToTarget(start), 2)
	if CompactToTarget(clamped).Cmp(wantQuarter) != 0 {
		t.Fatalf("clamp deveria travar em target/4, veio %#x", clamped)
	}

	// Lento além do clamp → trava em 4× mais fácil.
	slow := NextBits(0, expected*10, start, p)
	wantQuad := new(big.Int).Lsh(CompactToTarget(start), 2)
	if CompactToTarget(slow).Cmp(wantQuad) != 0 {
		t.Fatalf("clamp deveria travar em target×4, veio %#x", slow)
	}

	// Timestamps invertidos não explodem: caem no clamp de mais difícil.
	if got := NextBits(1000, 0, start, p); CompactToTarget(got).Cmp(wantQuarter) != 0 {
		t.Fatalf("timestamps invertidos deveriam clampar, veio %#x", got)
	}

	// O target nunca passa do limite da rede (dificuldade mínima).
	atLimit := NextBits(0, expected*10, p.PowLimitBits, p)
	if CompactToTarget(atLimit).Cmp(CompactToTarget(p.PowLimitBits)) > 0 {
		t.Fatal("retarget não pode ultrapassar PowLimitBits")
	}
}

func TestNextBitsTinyWindowDoesNotZeroTarget(t *testing.T) {
	// Regressão: janela curta (expected/MaxClamp = 0 na divisão inteira) com
	// época instantânea zerava o target → dificuldade impossível (~2^256).
	p := params.TestNet()
	p.RetargetInterval = 3
	p.TargetSpacing = time.Second // expected = 3s; 3/4 = 0

	got := NextBits(100, 100, 0x20100000, p) // época de 0s
	target := CompactToTarget(got)
	if target == nil || target.Sign() <= 0 {
		t.Fatalf("target zerou: bits %#x", got)
	}
	// Tem que endurecer (época rápida), mas dentro do clamp de 4×:
	// nunca menos de 1/4 do target original.
	quarter := new(big.Int).Rsh(CompactToTarget(0x20100000), 2)
	if target.Cmp(quarter) < 0 {
		t.Fatalf("endureceu além do clamp de 4×: bits %#x", got)
	}

	// TargetSpacing de 0 também não pode dividir por zero.
	p.TargetSpacing = 0
	if bad := NextBits(0, 10, 0x20100000, p); CompactToTarget(bad) == nil {
		t.Fatal("spacing zero não pode produzir bits inválidos")
	}
}
