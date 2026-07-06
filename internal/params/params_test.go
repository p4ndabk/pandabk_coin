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

func TestBuildOverridesCustomizeDevnet(t *testing.T) {
	defer func() { buildSpacing, buildHalving, buildSubsidy, buildRetarget = "", "", "", "" }()
	buildSpacing, buildHalving, buildSubsidy, buildRetarget = "30s", "500", "10", "20"

	p := DevNet()
	if p.TargetSpacing != 30e9 || p.HalvingInterval != 500 || p.InitialSubsidy != 10*CoinUnit || p.RetargetInterval != 20 {
		t.Fatalf("overrides não aplicados: %v %d %d %d", p.TargetSpacing, p.HalvingInterval, p.InitialSubsidy, p.RetargetInterval)
	}
	if !p.CustomBuild || p.Name != "devnet-custom" {
		t.Fatalf("build custom não marcado: CustomBuild=%v Name=%q", p.CustomBuild, p.Name)
	}
	// As regras têm que estar na mensagem do gênesis (mudam o hash = ID da rede)
	// e o hash/nonce hardcoded têm que ter sido descartados.
	if p.Genesis.Hash != ([32]byte{}) || p.Genesis.Nonce != 0 {
		t.Fatal("gênesis hardcoded deveria ter sido descartado no build custom")
	}
	want := " [spacing=30s halving=500 subsidy=10 retarget=20]"
	if got := p.Genesis.Message; len(got) < len(want) || got[len(got)-len(want):] != want {
		t.Fatalf("mensagem do gênesis sem as regras: %q", got)
	}

	// TestNet fica imune: os testes unitários precisam de regras estáveis.
	tp := TestNet()
	if tp.CustomBuild || tp.HalvingInterval != 1000 {
		t.Fatalf("TestNet não pode herdar overrides de build: %+v", tp)
	}
}

func TestBuildOverridesRejectGarbage(t *testing.T) {
	defer func() { buildSpacing, buildHalving, buildSubsidy, buildRetarget = "", "", "", "" }()
	for _, bad := range []func(){
		func() { buildSpacing = "rapidinho" },
		func() { buildSpacing = "10ms" }, // < 1s
		func() { buildHalving = "0" },
		func() { buildSubsidy = "5000" },  // > 1000 PANDA
		func() { buildRetarget = "1" },    // janela degenerada
		func() { buildRetarget = "1e12" }, // não-numérico/absurdo
	} {
		buildSpacing, buildHalving, buildSubsidy, buildRetarget = "", "", "", ""
		bad()
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("valor inválido (%q/%q/%q/%q) deveria dar panic com mensagem clara", buildSpacing, buildHalving, buildSubsidy, buildRetarget)
				}
			}()
			DevNet()
		}()
	}
}
