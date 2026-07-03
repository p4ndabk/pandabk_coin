package params

// GenesisSpec descreve o bloco 0 de um perfil. O gênesis é minerado uma única
// vez offline (comando dev `node genesis`, M2) e hardcoded aqui; a chain o
// valida por igualdade exata de hash, e o hash serve de ID da rede no
// handshake P2P.
type GenesisSpec struct {
	Timestamp int64
	Message   string
	Bits      uint32
	Nonce     uint64
	Hash      [32]byte
}

// Nonce e Hash zerados = gênesis ainda não minerado (acontece no M2).
var devnetGenesis = GenesisSpec{
	Timestamp: 1783036800, // 2026-07-03 00:00:00 UTC
	Message:   "PANDA devnet — um node em cada casa",
	Bits:      0x20010000,
}

var testGenesis = GenesisSpec{
	Timestamp: 1783036800,
	Message:   "PANDA test",
	Bits:      0x207fffff,
}
