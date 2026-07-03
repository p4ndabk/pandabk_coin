// Package params centraliza os parâmetros de consenso e a política monetária
// da rede PANDA em perfis imutáveis. Todos os outros pacotes do node importam
// daqui; nenhuma regra numérica de consenso vive fora deste pacote.
// Ver internal/params/SPEC.md.
package params

import "time"

// CoinUnit é quantas subunidades formam 1 PANDA (como satoshis no Bitcoin).
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

	DefaultPort    string
	DefaultRPCPort string
	SeedPeers      []string
}

// DevNet é o perfil de desenvolvimento: ciclos curtos para ver halving e
// retarget acontecendo em horas, com o Argon2id em força real (64 MiB).
func DevNet() Params {
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
// mínima quase nula, para que `go test` rode em milissegundos.
func TestNet() Params {
	p := DevNet()
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
	p := DevNet()
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
