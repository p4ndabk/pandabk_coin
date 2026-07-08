package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"zhu/internal/params"
)

// newTestStore abre um banco de demo em arquivo temporário (WAL exige
// arquivo real; :memory: não é compartilhável entre conexões).
func newTestStore(t *testing.T) *demoStore {
	t.Helper()
	s, err := openDemoStore(filepath.Join(t.TempDir(), "demo.db"))
	if err != nil {
		t.Fatalf("openDemoStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func testRow(height uint64, miner string, foundAt int64) demoBlockRow {
	// prev encadeia com o id determinístico da altura anterior — insertBlock
	// exige que todo bloco estenda o tip local.
	prev := strings.Repeat("0", 64)
	if height > 1 {
		prev = strings.Repeat("a", 62) + itoa2(height-1)
	}
	return demoBlockRow{
		height: height,
		id:     strings.Repeat("a", 62) + itoa2(height),
		prev:   prev,
		bits:   0x20010000, nonce: height, miner: miner,
		reward: 50 * params.CoinUnit, attempts: 10, durationMS: 1500, foundAt: foundAt,
	}
}

// itoa2 gera 2 chars hex distintos por altura para o id ficar único.
func itoa2(h uint64) string {
	const hexdigits = "0123456789abcdef"
	return string([]byte{hexdigits[(h>>4)&0xf], hexdigits[h&0xf]})
}

func TestInsertBlockRace(t *testing.T) {
	s := newTestStore(t)

	if err := s.insertBlock(testRow(1, "alice", 100)); err != nil {
		t.Fatalf("primeiro insert: %v", err)
	}
	// Mesmo bloco de novo (id repetido) e bloco concorrente (id diferente):
	// os dois têm que perder a corrida, nunca sobrescrever o vencedor.
	err := s.insertBlock(testRow(1, "bob", 101))
	if !errors.Is(err, errRaceLost) {
		t.Fatalf("segundo insert na altura 1 deveria perder a corrida, veio %v", err)
	}
	row, err := s.blockAt(1)
	if err != nil || row.miner != "alice" {
		t.Fatalf("o vencedor tinha que continuar sendo alice: %+v, %v", row, err)
	}

	tip, err := s.tip()
	if err != nil || tip.height != 1 {
		t.Fatalf("tip deveria ser 1, veio %d (%v)", tip.height, err)
	}
}

func TestTipEmptyDB(t *testing.T) {
	s := newTestStore(t)
	tip, err := s.tip()
	if err != nil {
		t.Fatalf("tip em banco vazio: %v", err)
	}
	if tip.height != 0 || tip.id != [32]byte{} {
		t.Fatalf("banco vazio deveria dar altura 0 e prev zero, veio %+v", tip)
	}
}

func TestInitMetaFirstWriterWins(t *testing.T) {
	s := newTestStore(t)

	first := demoMeta{profile: "test", spacing: 2 * time.Second, retarget: 3, zeros: 8}
	got, created, err := s.initMeta(first)
	if err != nil || !created || got != first {
		t.Fatalf("primeiro initMeta deveria criar e devolver a própria config: %+v, created=%v, err=%v", got, created, err)
	}

	// Segundo minerador chega com flags diferentes: adota a config gravada.
	second := demoMeta{profile: "devnet", spacing: time.Minute, retarget: 10, zeros: 12}
	got, created, err = s.initMeta(second)
	if err != nil || created || got != first {
		t.Fatalf("segundo initMeta deveria adotar a config existente: %+v, created=%v, err=%v", got, created, err)
	}
}

func TestBitsForHeightDeterministic(t *testing.T) {
	s := newTestStore(t)
	rules := demoRetargetRules(params.TestNet(), 3, 10*time.Second)
	const zeros = 8

	// Antes de qualquer época completa, vale a dificuldade inicial.
	bits, err := bitsForHeight(s, rules, zeros, 1)
	if err != nil || bits != initialBits(zeros) {
		t.Fatalf("altura 1 deveria usar bits iniciais: %#x, %v", bits, err)
	}

	// Época 1 (blocos 1..3) minerada 2× mais devagar que o alvo
	// (janela de 60s para 30s esperados) → o retarget tem que FACILITAR.
	base := int64(1000)
	for h := uint64(1); h <= 3; h++ {
		if err := s.insertBlock(testRow(h, "alice", base+int64(h-1)*30)); err != nil {
			t.Fatalf("insert %d: %v", h, err)
		}
	}
	bits4, err := bitsForHeight(s, rules, zeros, 4)
	if err != nil {
		t.Fatalf("bitsForHeight(4): %v", err)
	}
	if bits4 == initialBits(zeros) {
		t.Fatal("depois de uma época lenta a dificuldade tinha que mudar")
	}
	if avgAttempts(bits4) >= avgAttempts(initialBits(zeros)) {
		t.Fatalf("época lenta deveria FACILITAR, veio %#x", bits4)
	}

	// Dentro da mesma época os bits não mudam; e um segundo handle do mesmo
	// arquivo (outro "minerador") deriva exatamente os mesmos bits.
	bits5, _ := bitsForHeight(s, rules, zeros, 5)
	if bits5 != bits4 {
		t.Fatalf("alturas 4 e 5 são da mesma época: %#x != %#x", bits4, bits5)
	}
	again, _ := bitsForHeight(s, rules, zeros, 4)
	if again != bits4 {
		t.Fatalf("bitsForHeight não é determinística: %#x != %#x", again, bits4)
	}
}

func TestMinerBalanceAndRanking(t *testing.T) {
	s := newTestStore(t)
	miners := []string{"alice", "bob", "alice"} // inserts em ordem de altura (insertBlock encadeia)
	for i, miner := range miners {
		h := uint64(i + 1)
		if err := s.insertBlock(testRow(h, miner, int64(1000+h))); err != nil {
			t.Fatalf("insert %d: %v", h, err)
		}
	}

	reward, blocks, err := s.minerBalance("alice")
	if err != nil || blocks != 2 || reward != 100*params.CoinUnit {
		t.Fatalf("alice deveria ter 2 blocos e 100 ZHU: %d blocos, %d, %v", blocks, reward, err)
	}
	if _, blocks, _ := s.minerBalance("ninguem"); blocks != 0 {
		t.Fatalf("minerador inexistente deveria ter 0 blocos, veio %d", blocks)
	}

	ranks, err := s.ranking()
	if err != nil || len(ranks) != 2 {
		t.Fatalf("ranking deveria ter 2 mineradores: %+v, %v", ranks, err)
	}
	if ranks[0].miner != "alice" || ranks[0].blocks != 2 {
		t.Fatalf("alice deveria liderar o placar: %+v", ranks[0])
	}
}
