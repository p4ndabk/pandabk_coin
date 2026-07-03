package params

import "testing"

func TestBlockSubsidyHalvingBoundaries(t *testing.T) {
	p := DevNet()
	cases := []struct {
		height uint64
		want   uint64
	}{
		{0, 50 * CoinUnit},
		{999, 50 * CoinUnit},
		{1000, 25 * CoinUnit},
		{1999, 25 * CoinUnit},
		{2000, 12_50000000 / 1}, // 12.5 PANDA
		{1000 * 64, 0},          // shift >= 64 não pode estourar
		{^uint64(0), 0},
	}
	for _, c := range cases {
		if got := p.BlockSubsidy(c.height); got != c.want {
			t.Errorf("BlockSubsidy(%d) = %d, want %d", c.height, got, c.want)
		}
	}
}

func TestMaxSupplyMatchesSchedule(t *testing.T) {
	p := DevNet()

	// Replay bloco a bloco das primeiras épocas + soma por época das demais
	// tem que bater com MaxSupply.
	var total uint64
	for epoch := uint64(0); ; epoch++ {
		s := p.BlockSubsidy(epoch * p.HalvingInterval)
		if s == 0 {
			break
		}
		total += s * p.HalvingInterval
	}
	if got := p.MaxSupply(); got != total {
		t.Fatalf("MaxSupply() = %d, want %d", got, total)
	}

	// Sanidade: teto na casa das ~100.000 PANDA (soma geométrica de 50×1000×2,
	// menos o arredondamento das divisões inteiras).
	if got := p.MaxSupply() / CoinUnit; got < 99_000 || got > 100_000 {
		t.Fatalf("MaxSupply() = %d PANDA, esperado ~100.000", got)
	}
}

func TestProfilesAreConsistent(t *testing.T) {
	for _, p := range []Params{DevNet(), TestNet(), MainNet()} {
		if p.HalvingInterval == 0 || p.RetargetInterval == 0 {
			t.Fatalf("perfil %s com intervalo zero", p.Name)
		}
		if p.Argon2Time == 0 || p.Argon2Mem == 0 || p.Argon2Threads == 0 {
			t.Fatalf("perfil %s com Argon2 inválido", p.Name)
		}
		if p.MaxBlockSize == 0 || p.MaxClamp < 2 {
			t.Fatalf("perfil %s com limites inválidos", p.Name)
		}
	}
}
