// Package params centraliza os parâmetros de consenso e a política monetária
// da rede ZHU em perfis imutáveis. Todos os outros pacotes do node importam
// daqui; nenhuma regra numérica de consenso vive fora deste pacote.
// Ver internal/params/SPEC.md.
package params

import (
	"fmt"
	"strconv"
	"time"
)

// CoinUnit é quantas subunidades formam 1 ZHU (como satoshis no Bitcoin).
const CoinUnit uint64 = 100_000_000

type Params struct {
	Name             string
	TargetSpacing    time.Duration
	RetargetInterval uint64
	HalvingInterval  uint64
	InitialSubsidy   uint64

	// Parâmetros do Argon2id usados no proof of work (ver internal/pow).
	// Argon2Mem é em KiB. Threads fica em 1: o paralelismo é controlado
	// pelo número de workers do miner, não dentro de um hash.
	Argon2Mem     uint32
	Argon2Time    uint32
	Argon2Threads uint8

	// PowLimitBits é a dificuldade mínima da rede (o maior target aceito),
	// em formato compacto nBits.
	PowLimitBits uint32
	// MaxClamp limita o ajuste de dificuldade por retarget a [1/MaxClamp, MaxClamp]×.
	MaxClamp int64

	CoinbaseMaturity uint64
	// MaxBlockSize é regra de consenso: bloco serializado maior é inválido.
	// Mantém o crescimento do disco compatível com um node caseiro.
	MaxBlockSize uint32

	Genesis GenesisSpec

	// CustomBuild marca um binário compilado com regras próprias no
	// build.conf (spacing/halving/subsidy). O gênesis desse build é
	// derivado das regras em vez de conferido contra o hash hardcoded —
	// regras diferentes geram uma rede diferente por construção.
	CustomBuild bool

	DefaultPort    string
	DefaultRPCPort string
	SeedPeers      []string
}

// ── regras definidas no build ───────────────────────────────────────────────
//
// O desenvolvedor que compila pode fixar a economia da rede DAQUELE binário
// no build.conf (spacing/halving/subsidy); os scripts/build-*.sh injetam os
// valores aqui via -ldflags -X. Qualquer override muda a mensagem do bloco
// gênesis — logo muda o hash do gênesis, que é o ID da rede no handshake:
// binários com regras diferentes formam redes separadas por construção,
// nunca se misturam por acidente.
var (
	buildSpacing  string // ex.: "30s" (formato do time.ParseDuration)
	buildHalving  string // ex.: "1000" (blocos por época de halving)
	buildSubsidy  string // ex.: "50" (ZHU inteiros por bloco)
	buildRetarget string // ex.: "20" (reajusta a dificuldade a cada N blocos)
)

func applyBuildOverrides(p Params) Params {
	if buildSpacing == "" && buildHalving == "" && buildSubsidy == "" && buildRetarget == "" {
		return p
	}
	if buildSpacing != "" {
		d, err := time.ParseDuration(buildSpacing)
		if err != nil || d < time.Second {
			panic(fmt.Sprintf("build.conf: spacing inválido %q (use 30s, 1m, 10m...)", buildSpacing))
		}
		p.TargetSpacing = d
	}
	if buildHalving != "" {
		n, err := strconv.ParseUint(buildHalving, 10, 64)
		if err != nil || n == 0 || n > 10_000_000 {
			panic(fmt.Sprintf("build.conf: halving inválido %q (1 a 10.000.000 blocos)", buildHalving))
		}
		p.HalvingInterval = n
	}
	if buildSubsidy != "" {
		n, err := strconv.ParseUint(buildSubsidy, 10, 64)
		if err != nil || n == 0 || n > 1000 {
			panic(fmt.Sprintf("build.conf: subsidy inválido %q (1 a 1000 ZHU, inteiro)", buildSubsidy))
		}
		p.InitialSubsidy = n * CoinUnit
	}
	if buildRetarget != "" {
		n, err := strconv.ParseUint(buildRetarget, 10, 64)
		if err != nil || n < 2 || n > 100_000 {
			// n=1 é degenerado: a janela teria tamanho zero e a dificuldade
			// dispararia pelo clamp a cada bloco.
			panic(fmt.Sprintf("build.conf: retarget inválido %q (2 a 100000 blocos)", buildRetarget))
		}
		p.RetargetInterval = n
	}
	p.CustomBuild = true
	p.Name += "-custom"
	// As regras entram na mensagem do gênesis: o hash do bloco 0 passa a
	// identificar também a economia. Nonce/Hash hardcoded deixam de valer —
	// o gênesis deste build é derivado (ver chain.insertGenesis).
	p.Genesis.Message += fmt.Sprintf(" [spacing=%s halving=%d subsidy=%d retarget=%d]",
		p.TargetSpacing, p.HalvingInterval, p.InitialSubsidy/CoinUnit, p.RetargetInterval)
	p.Genesis.Nonce = 0
	p.Genesis.Hash = [32]byte{}
	return p
}

// DevNet é o perfil de desenvolvimento: ciclos curtos para ver halving e
// retarget acontecendo em horas, com o Argon2id em força real (64 MiB).
// Regras do build.conf (se houver) são aplicadas por cima.
func DevNet() Params {
	return applyBuildOverrides(devnetBase())
}

func devnetBase() Params {
	return Params{
		Name:             "devnet",
		TargetSpacing:    60 * time.Second,
		RetargetInterval: 100,
		HalvingInterval:  1000,
		InitialSubsidy:   50 * CoinUnit,
		Argon2Mem:        64 * 1024,
		Argon2Time:       1,
		Argon2Threads:    1,
		PowLimitBits:     0x20010000, // target 2^248 ≈ 256 hashes por bloco
		MaxClamp:         4,
		CoinbaseMaturity: 10,
		MaxBlockSize:     256 * 1024,
		Genesis:          devnetGenesis,
		DefaultPort:      ":9551",
		DefaultRPCPort:   "127.0.0.1:8555",
	}
}

// TestNet é o perfil para testes unitários: Argon2 com 1 MiB e dificuldade
// mínima quase nula, para que `go test` rode em milissegundos. Parte da base
// SEM os overrides de build — os testes precisam de regras estáveis.
func TestNet() Params {
	p := devnetBase()
	p.Name = "test"
	p.Argon2Mem = 1024
	p.PowLimitBits = 0x207fffff // target ≈ 2^255: quase todo hash passa
	p.Genesis = testGenesis
	return p
}

// MainNet é um placeholder: os valores definitivos (bloco de 10 min, halving
// de anos) serão calibrados quando a rede sair da fase de desenvolvimento.
// Não usar antes disso.
func MainNet() Params {
	p := devnetBase()
	p.Name = "mainnet"
	p.TargetSpacing = 10 * time.Minute
	p.RetargetInterval = 2016
	p.HalvingInterval = 210_000
	return p
}

// BlockSubsidy é a recompensa da coinbase na altura dada: o subsídio inicial
// cortado pela metade a cada HalvingInterval blocos, até zerar.
func (p Params) BlockSubsidy(height uint64) uint64 {
	halvings := height / p.HalvingInterval
	if halvings >= 64 {
		return 0
	}
	return p.InitialSubsidy >> halvings
}

// MaxSupply deriva o teto de emissão somando o cronograma de halving — nunca
// um número hardcoded, para não divergir de BlockSubsidy.
func (p Params) MaxSupply() uint64 {
	var total uint64
	for epoch := uint64(0); ; epoch++ {
		s := p.BlockSubsidy(epoch * p.HalvingInterval)
		if s == 0 {
			return total
		}
		total += s * p.HalvingInterval
	}
}
