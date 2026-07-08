// demostore é a "rede" do modo corrida do powdemo: um SQLite compartilhado
// na mesma máquina onde cada minerador registra o bloco que achou. A altura
// é PRIMARY KEY, então quando dois acham o bloco N quase juntos só o primeiro
// INSERT vence — o outro recebe errRaceLost, exatamente o papel que o fork
// choice do M2 vai cumprir de verdade na chain real (que usa bbolt, não isto).
package main

import (
	"database/sql"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	_ "github.com/glebarez/go-sqlite"

	"zhu/internal/params"
	"zhu/internal/pow"
)

var errRaceLost = errors.New("outro minerador registrou um bloco nesta altura primeiro")

var errOutOfSequence = errors.New("bloco não encadeia com o tip local")

const demoSchema = `
CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS blocks (
  height      INTEGER PRIMARY KEY,
  id          TEXT NOT NULL UNIQUE,
  prev        TEXT NOT NULL,
  bits        INTEGER NOT NULL,
  nonce       INTEGER NOT NULL,
  miner       TEXT NOT NULL,
  reward      INTEGER NOT NULL,
  attempts    INTEGER NOT NULL,
  duration_ms INTEGER NOT NULL,
  found_at    INTEGER NOT NULL
);`

type demoStore struct{ db *sql.DB }

var _ raceStore = (*demoStore)(nil)

// demoMeta é a config da corrida, gravada pelo primeiro minerador e adotada
// por todos os outros — mineradores do mesmo banco precisam derivar a mesma
// dificuldade.
type demoMeta struct {
	profile  string
	spacing  time.Duration
	retarget uint64
	zeros    uint
}

type demoTip struct {
	height uint64
	id     [32]byte // zero = "gênesis" (banco vazio)
}

type demoBlockRow struct {
	height     uint64
	id, prev   string
	bits       uint32
	nonce      uint64
	miner      string
	reward     uint64
	attempts   uint64
	durationMS int64
	foundAt    int64
}

type rankRow struct {
	miner  string
	blocks uint64
	reward uint64
	avgMS  float64
}

// raceStore é o contrato que o powdemo (-db ou -peer) e os subcomandos de
// consulta usam para falar com a corrida: local (*demoStore, arquivo SQLite
// na mesma máquina) ou remoto (*netStore, TCP até um node -listen em outra
// máquina — ver netstore.go). As duas implementações têm exatamente as
// mesmas operações; quem chama nunca sabe se está falando com um arquivo ou
// com a rede.
type raceStore interface {
	initMeta(demoMeta) (demoMeta, bool, error)
	loadMeta() (demoMeta, error)
	tip() (demoTip, error)
	blockAt(height uint64) (demoBlockRow, error)
	insertBlock(demoBlockRow) error
	minerBalance(name string) (reward, blocks uint64, err error)
	listBlocks(last int) ([]demoBlockRow, error)
	ranking() ([]rankRow, error)
	epochWindow(firstH, lastH uint64) (first, last int64, err error)
	Close() error
}

func openDemoStore(path string) (*demoStore, error) {
	// busy_timeout + WAL: dois processos escrevendo no mesmo arquivo.
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(demoSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("criando schema: %w", err)
	}
	return &demoStore{db: db}, nil
}

func (s *demoStore) Close() error { return s.db.Close() }

// initMeta grava a config se o banco for novo e devolve a vigente. Uma única
// instrução INSERT (atômica no SQLite): dois mineradores subindo ao mesmo
// tempo não misturam configs — um grava as quatro chaves, o outro adota.
func (s *demoStore) initMeta(m demoMeta) (demoMeta, bool, error) {
	res, err := s.db.Exec(
		`INSERT INTO meta(key, value) VALUES
		   ('profile', ?), ('spacing', ?), ('retarget', ?), ('zeros', ?)
		 ON CONFLICT(key) DO NOTHING`,
		m.profile, m.spacing.String(),
		strconv.FormatUint(m.retarget, 10), strconv.FormatUint(uint64(m.zeros), 10))
	if err != nil {
		return demoMeta{}, false, err
	}
	n, _ := res.RowsAffected()
	got, err := s.loadMeta()
	return got, n == 4, err
}

func (s *demoStore) loadMeta() (demoMeta, error) {
	rows, err := s.db.Query(`SELECT key, value FROM meta`)
	if err != nil {
		return demoMeta{}, err
	}
	defer rows.Close()
	kv := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return demoMeta{}, err
		}
		kv[k] = v
	}
	if err := rows.Err(); err != nil {
		return demoMeta{}, err
	}
	if len(kv) < 4 {
		return demoMeta{}, errors.New("banco sem configuração (rode primeiro um minerador com -db)")
	}
	var m demoMeta
	m.profile = kv["profile"]
	if m.spacing, err = time.ParseDuration(kv["spacing"]); err != nil {
		return demoMeta{}, fmt.Errorf("spacing inválido no banco: %w", err)
	}
	if m.retarget, err = strconv.ParseUint(kv["retarget"], 10, 64); err != nil || m.retarget < 1 {
		return demoMeta{}, fmt.Errorf("retarget inválido no banco: %q", kv["retarget"])
	}
	z, err := strconv.ParseUint(kv["zeros"], 10, 8)
	if err != nil || z < 1 || z > 64 {
		return demoMeta{}, fmt.Errorf("zeros inválido no banco: %q", kv["zeros"])
	}
	m.zeros = uint(z)
	return m, nil
}

func (s *demoStore) tip() (demoTip, error) {
	var h uint64
	var idHex string
	err := s.db.QueryRow(`SELECT height, id FROM blocks ORDER BY height DESC LIMIT 1`).Scan(&h, &idHex)
	if errors.Is(err, sql.ErrNoRows) {
		return demoTip{}, nil // banco vazio: altura 0, prev = hash zero
	}
	if err != nil {
		return demoTip{}, err
	}
	raw, err := hex.DecodeString(idHex)
	if err != nil || len(raw) != 32 {
		return demoTip{}, fmt.Errorf("id inválido no banco na altura %d: %q", h, idHex)
	}
	t := demoTip{height: h}
	copy(t.id[:], raw)
	return t, nil
}

func (s *demoStore) blockAt(height uint64) (demoBlockRow, error) {
	var b demoBlockRow
	err := s.db.QueryRow(
		`SELECT height, id, prev, bits, nonce, miner, reward, attempts, duration_ms, found_at
		   FROM blocks WHERE height = ?`, height).
		Scan(&b.height, &b.id, &b.prev, &b.bits, &b.nonce, &b.miner, &b.reward, &b.attempts, &b.durationMS, &b.foundAt)
	return b, err
}

// insertBlock registra um bloco recém-minerado. O bloco tem que ENCADEAR com
// o tip local (altura tip+1 e prev = id do tip): um push de peer adiantado
// (ex: ele está na altura 235 e nós na 178) é recusado em vez de abrir um
// buraco de alturas no banco — quem traz os blocos que faltam, EM ORDEM, é o
// reconcile. A PRIMARY KEY em height resolve a corrida: se já existe bloco
// nessa altura, alguém venceu antes e o chamador recebe errRaceLost (o bloco
// dele vira "órfão" da demo).
func (s *demoStore) insertBlock(b demoBlockRow) error {
	t, err := s.tip()
	if err != nil {
		return err
	}
	switch {
	case b.height <= t.height:
		return fmt.Errorf("%w (altura %d)", errRaceLost, b.height)
	case b.height > t.height+1:
		return fmt.Errorf("%w: altura %d com tip local em %d — reconcile trará os que faltam", errOutOfSequence, b.height, t.height)
	case b.prev != hex.EncodeToString(t.id[:]):
		// altura certa mas outro pai: fork — a comparação de trabalho do
		// reconcile decide, não um insert avulso
		return fmt.Errorf("%w (altura %d de um fork)", errRaceLost, b.height)
	}
	_, err = s.db.Exec(
		`INSERT INTO blocks(height, id, prev, bits, nonce, miner, reward, attempts, duration_ms, found_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.height, b.id, b.prev, b.bits, b.nonce, b.miner, b.reward, b.attempts, b.durationMS, b.foundAt)
	if err != nil {
		if _, raced := s.blockAt(b.height); raced == nil {
			return fmt.Errorf("%w (altura %d)", errRaceLost, b.height)
		}
		return err
	}
	return nil
}

// pruneDangling remove blocos soltos acima do primeiro buraco de alturas —
// herança de bancos gravados antes do insertBlock exigir encadeamento (um
// push de peer adiantado abria buraco e a derivação de dificuldade travava
// no primeiro bloco ausente). Os blocos removidos voltam pelo reconcile, em
// ordem, se realmente pertencerem à chain vencedora.
func (s *demoStore) pruneDangling() (pruned int64, err error) {
	// Drena o SELECT por completo ANTES do DELETE: o pool tem uma única
	// conexão (SetMaxOpenConns(1)), então um Exec com o cursor ainda aberto
	// esperaria a conexão para sempre.
	rows, err := s.db.Query(`SELECT height FROM blocks ORDER BY height`)
	if err != nil {
		return 0, err
	}
	var heights []uint64
	for rows.Next() {
		var h uint64
		if err := rows.Scan(&h); err != nil {
			rows.Close()
			return 0, err
		}
		heights = append(heights, h)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	var contig uint64
	gap := false
	for _, h := range heights {
		if h != contig+1 {
			gap = true
			break
		}
		contig = h
	}
	if !gap {
		return 0, nil
	}
	res, err := s.db.Exec(`DELETE FROM blocks WHERE height > ?`, contig)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// truncateAbove descarta todo bloco com altura > height — usado só pelo
// reconcile (reconcile.go) quando este banco perde a comparação de trabalho
// acumulado pra outra chain: os blocos descartados aqui são o "reorg" da
// demo, o mesmo papel que undo sets cumprem na chain de verdade (M2).
func (s *demoStore) truncateAbove(height uint64) error {
	_, err := s.db.Exec(`DELETE FROM blocks WHERE height > ?`, height)
	return err
}

func (s *demoStore) minerBalance(name string) (reward, blocks uint64, err error) {
	err = s.db.QueryRow(
		`SELECT COALESCE(SUM(reward), 0), COUNT(*) FROM blocks WHERE miner = ?`, name).
		Scan(&reward, &blocks)
	return reward, blocks, err
}

func (s *demoStore) listBlocks(last int) ([]demoBlockRow, error) {
	rows, err := s.db.Query(
		`SELECT height, id, prev, bits, nonce, miner, reward, attempts, duration_ms, found_at
		   FROM blocks ORDER BY height DESC LIMIT ?`, last)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []demoBlockRow
	for rows.Next() {
		var b demoBlockRow
		if err := rows.Scan(&b.height, &b.id, &b.prev, &b.bits, &b.nonce, &b.miner, &b.reward, &b.attempts, &b.durationMS, &b.foundAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	// A query pega os N mais recentes; exibimos em ordem crescente.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}

func (s *demoStore) ranking() ([]rankRow, error) {
	rows, err := s.db.Query(
		`SELECT miner, COUNT(*), SUM(reward), AVG(duration_ms)
		   FROM blocks GROUP BY miner ORDER BY COUNT(*) DESC, SUM(reward) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []rankRow
	for rows.Next() {
		var r rankRow
		if err := rows.Scan(&r.miner, &r.blocks, &r.reward, &r.avgMS); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// epochWindow devolve os found_at do primeiro e do último bloco da janela —
// os dois timestamps que o pow.NextBits compara com o tempo esperado.
func (s *demoStore) epochWindow(firstH, lastH uint64) (first, last int64, err error) {
	fb, err := s.blockAt(firstH)
	if err != nil {
		return 0, 0, fmt.Errorf("bloco %d da janela de retarget: %w", firstH, err)
	}
	lb, err := s.blockAt(lastH)
	if err != nil {
		return 0, 0, fmt.Errorf("bloco %d da janela de retarget: %w", lastH, err)
	}
	return fb.foundAt, lb.foundAt, nil
}

// bitsForHeight deriva a dificuldade de uma altura SÓ a partir do banco:
// começa nos zeros iniciais da meta e reaplica pow.NextBits época por época.
// Determinística — qualquer minerador que leia o mesmo banco chega nos
// mesmos bits, sem trocar nenhuma mensagem (é o consenso da demo).
func bitsForHeight(s raceStore, rules params.Params, zeros uint, height uint64) (uint32, error) {
	bits := initialBits(zeros)
	n := rules.RetargetInterval
	epochs := (height - 1) / n // épocas completas antes desta altura
	for e := uint64(1); e <= epochs; e++ {
		first, last, err := s.epochWindow((e-1)*n+1, e*n)
		if err != nil {
			return 0, err
		}
		bits = pow.NextBits(first, last, bits, rules)
	}
	return bits, nil
}

// ── subcomandos de consulta (zhu blocks / zhu ranking) ───────────────────

// openStoreForQuery abre o banco local (-db) ou conecta num node remoto
// (-peer) — os dois viram o mesmo raceStore para o resto do comando.
func openStoreForQuery(fs *flag.FlagSet, args []string, dbPath, peer *string) raceStore {
	configPath := fs.String("config", "", "arquivo de configuração chave=valor (default: zhu.conf, se existir)")
	fs.Parse(args)
	fromCLI := applyConfig(fs, *configPath)
	if *dbPath == "" && *peer == "" {
		fmt.Fprintln(os.Stderr, "informe -db mineracao.db (mesma máquina) ou -peer host:porta (rede)")
		os.Exit(2)
	}
	if *dbPath != "" && *peer != "" {
		// Um zhu.conf de minerador traz db E peer juntos (é válido lá);
		// para consulta, a cópia local é a fonte natural — só é erro se o
		// próprio usuário pediu os dois na linha de comando.
		if fromCLI["db"] && fromCLI["peer"] {
			fmt.Fprintln(os.Stderr, "use -db OU -peer, não os dois")
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "ℹ️  config traz db e peer: consultando o banco local %s\n", *dbPath)
		*peer = ""
	}
	if *peer != "" {
		store := dialRaceStore(*peer)
		if _, err := store.tip(); err != nil { // comando avulso: falha rápido se não alcançar
			fmt.Fprintf(os.Stderr, "conectando a %s: %v\n", *peer, err)
			os.Exit(1)
		}
		return store
	}
	if _, err := os.Stat(*dbPath); err != nil {
		fmt.Fprintf(os.Stderr, "banco %s não encontrado — rode um minerador com -db primeiro\n", *dbPath)
		os.Exit(1)
	}
	store, err := openDemoStore(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "abrindo %s: %v\n", *dbPath, err)
		os.Exit(1)
	}
	return store
}

// queryLabel devolve o texto pra exibir no cabeçalho: o caminho do arquivo
// local ou o endereço do peer remoto, o que estiver preenchido.
func queryLabel(dbPath, peer string) string {
	if peer != "" {
		return "peer " + peer
	}
	return dbPath
}

func runBlocks(args []string) {
	fs := flag.NewFlagSet("blocks", flag.ExitOnError)
	dbPath := fs.String("db", "", "arquivo SQLite da demo (mesma máquina)")
	peer := fs.String("peer", "", "endereço host:porta de um node -listen (outra máquina)")
	last := fs.Int("last", 20, "quantos blocos recentes mostrar")
	store := openStoreForQuery(fs, args, dbPath, peer)
	defer store.Close()

	blocks, err := store.listBlocks(*last)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listando blocos: %v\n", err)
		os.Exit(1)
	}
	if len(blocks) == 0 {
		fmt.Println("nenhum bloco no banco ainda.")
		return
	}
	fmt.Printf("── últimos %d blocos (%s) ─────────────────────────────────────\n", len(blocks), queryLabel(*dbPath, *peer))
	fmt.Printf(" %6s  %-14s  %-14s  %-11s  %10s  %8s  %10s\n",
		"altura", "quando", "minerador", "dificuldade", "tentativas", "⏱ tempo", "recompensa")
	for _, b := range blocks {
		fmt.Printf(" %6d  %-14s  %-14s  %#08x   %10s  %8s  %7s 🐼\n",
			b.height, time.Unix(b.foundAt, 0).Format("02/01 15:04:05"), b.miner,
			b.bits, humanCount(float64(b.attempts)),
			fmtDur(time.Duration(b.durationMS)*time.Millisecond), formatZhu(b.reward))
	}
}

func runRanking(args []string) {
	fs := flag.NewFlagSet("ranking", flag.ExitOnError)
	dbPath := fs.String("db", "", "arquivo SQLite da demo (mesma máquina)")
	peer := fs.String("peer", "", "endereço host:porta de um node -listen (outra máquina)")
	store := openStoreForQuery(fs, args, dbPath, peer)
	defer store.Close()

	meta, err := store.loadMeta()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	p, err := resolveProfile(meta.profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "perfil gravado no banco é inválido: %v\n", err)
		os.Exit(1)
	}
	if err := printRanking(store, demoRetargetRules(p, meta.retarget, meta.spacing), meta); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

// printRanking imprime o placar por minerador + o estado atual da rede demo.
// Usado pelo subcomando ranking e pelo resumo final do powdemo -db/-peer.
func printRanking(store raceStore, rules params.Params, meta demoMeta) error {
	ranks, err := store.ranking()
	if err != nil {
		return err
	}
	if len(ranks) == 0 {
		fmt.Println("nenhum bloco no banco ainda.")
		return nil
	}
	var totalBlocks, totalReward uint64
	for _, r := range ranks {
		totalBlocks += r.blocks
		totalReward += r.reward
	}
	fmt.Printf("── placar ──────────────────────────────────\n")
	for i, r := range ranks {
		fmt.Printf("   %dº %-14s %5d blocos (%4.1f%%)  %8s ZHU  ⏱ média %s/bloco\n",
			i+1, r.miner, r.blocks, 100*float64(r.blocks)/float64(totalBlocks),
			formatZhu(r.reward), fmtDur(time.Duration(r.avgMS)*time.Millisecond))
	}

	tip, err := store.tip()
	if err != nil {
		return err
	}
	bits, err := bitsForHeight(store, rules, meta.zeros, tip.height+1)
	if err != nil {
		return err
	}
	fmt.Printf("\n   altura atual %d | emissão %s ZHU | dificuldade do próximo bloco %#08x (~%s tentativas)\n",
		tip.height, formatZhu(totalReward), bits, humanCount(avgAttempts(bits)))
	if totalBlocks >= 2 {
		if first, err := store.blockAt(1); err == nil {
			if last, err := store.blockAt(tip.height); err == nil && last.foundAt > first.foundAt {
				pace := time.Duration(last.foundAt-first.foundAt) * time.Second / time.Duration(totalBlocks-1)
				fmt.Printf("   ritmo da rede: %s/bloco (alvo %s)\n", fmtDur(pace), meta.spacing)
			}
		}
	}
	return nil
}
