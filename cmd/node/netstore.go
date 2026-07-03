// netstore leva o modo corrida do powdemo (demostore.go) para a rede: um
// node com -db abre "-listen" e serve o banco local por TCP; outro node, em
// OUTRA máquina, se conecta com "-peer host:porta" e não precisa de nenhum
// arquivo local — todo tip/insertBlock/etc vira uma chamada de rede. Não é
// P2P de verdade (é cliente-servidor, um só lado guarda o banco), mas usa o
// mesmo formato de fio já decidido para o protocolo definitivo do M4:
// TCP puro, frame com prefixo de 4 bytes (tamanho, big-endian) + corpo JSON.
package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

const maxRaceFrame = 1 << 20 // 1 MiB — generoso pra um payload que é só JSON de blocos avulsos

func writeFrame(w io.Writer, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(body) > maxRaceFrame {
		return fmt.Errorf("mensagem de %d bytes excede o limite de %d", len(body), maxRaceFrame)
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(body)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}

func readFrame(r io.Reader, v any) error {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(lenBuf[:])
	if n > maxRaceFrame {
		return fmt.Errorf("mensagem de %d bytes excede o limite de %d", n, maxRaceFrame)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

// ── tipos de fio: versões exportadas dos tipos internos de demostore.go,
// só para o JSON conseguir serializar (os campos lá são minúsculos de
// propósito — são internos ao pacote, não uma API). ─────────────────────────

type wireMeta struct {
	Profile  string `json:"profile"`
	Spacing  string `json:"spacing"`
	Retarget uint64 `json:"retarget"`
	Zeros    uint   `json:"zeros"`
}

func toWireMeta(m demoMeta) wireMeta {
	return wireMeta{Profile: m.profile, Spacing: m.spacing.String(), Retarget: m.retarget, Zeros: m.zeros}
}

func (w wireMeta) toMeta() (demoMeta, error) {
	d, err := time.ParseDuration(w.Spacing)
	if err != nil {
		return demoMeta{}, fmt.Errorf("spacing recebido do peer é inválido: %w", err)
	}
	return demoMeta{profile: w.Profile, spacing: d, retarget: w.Retarget, zeros: w.Zeros}, nil
}

type wireTip struct {
	Height uint64 `json:"height"`
	ID     string `json:"id"`
}

func toWireTip(t demoTip) wireTip {
	return wireTip{Height: t.height, ID: hex.EncodeToString(t.id[:])}
}

func (w wireTip) toTip() (demoTip, error) {
	t := demoTip{height: w.Height}
	if w.Height == 0 {
		return t, nil
	}
	raw, err := hex.DecodeString(w.ID)
	if err != nil || len(raw) != 32 {
		return demoTip{}, fmt.Errorf("tip.id recebido do peer é inválido: %q", w.ID)
	}
	copy(t.id[:], raw)
	return t, nil
}

type wireBlock struct {
	Height     uint64 `json:"height"`
	ID         string `json:"id"`
	Prev       string `json:"prev"`
	Bits       uint32 `json:"bits"`
	Nonce      uint64 `json:"nonce"`
	Miner      string `json:"miner"`
	Reward     uint64 `json:"reward"`
	Attempts   uint64 `json:"attempts"`
	DurationMS int64  `json:"duration_ms"`
	FoundAt    int64  `json:"found_at"`
}

func toWireBlock(b demoBlockRow) wireBlock {
	return wireBlock{
		Height: b.height, ID: b.id, Prev: b.prev, Bits: b.bits, Nonce: b.nonce,
		Miner: b.miner, Reward: b.reward, Attempts: b.attempts,
		DurationMS: b.durationMS, FoundAt: b.foundAt,
	}
}

func (w wireBlock) toBlock() demoBlockRow {
	return demoBlockRow{
		height: w.Height, id: w.ID, prev: w.Prev, bits: w.Bits, nonce: w.Nonce,
		miner: w.Miner, reward: w.Reward, attempts: w.Attempts,
		durationMS: w.DurationMS, foundAt: w.FoundAt,
	}
}

type wireRank struct {
	Miner  string  `json:"miner"`
	Blocks uint64  `json:"blocks"`
	Reward uint64  `json:"reward"`
	AvgMS  float64 `json:"avg_ms"`
}

func toWireRank(r rankRow) wireRank {
	return wireRank{Miner: r.miner, Blocks: r.blocks, Reward: r.reward, AvgMS: r.avgMS}
}

func (w wireRank) toRank() rankRow {
	return rankRow{miner: w.Miner, blocks: w.Blocks, reward: w.Reward, avgMS: w.AvgMS}
}

// ── envelope da chamada: um Op por método do raceStore ──────────────────────

type rpcRequest struct {
	Op      string     `json:"op"`
	Meta    *wireMeta  `json:"meta,omitempty"`
	Height  uint64     `json:"height,omitempty"`
	Height2 uint64     `json:"height2,omitempty"`
	Name    string     `json:"name,omitempty"`
	Last    int        `json:"last,omitempty"`
	Block   *wireBlock `json:"block,omitempty"`
}

type rpcResponse struct {
	OK       bool        `json:"ok"`
	Error    string      `json:"error,omitempty"`
	RaceLost bool        `json:"race_lost,omitempty"`
	Meta     *wireMeta   `json:"meta,omitempty"`
	Created  bool        `json:"created,omitempty"`
	Tip      *wireTip    `json:"tip,omitempty"`
	Block    *wireBlock  `json:"block,omitempty"`
	Reward   uint64      `json:"reward,omitempty"`
	Blocks   uint64      `json:"blocks,omitempty"`
	Rows     []wireBlock `json:"rows,omitempty"`
	Ranks    []wireRank  `json:"ranks,omitempty"`
	First    int64       `json:"first,omitempty"`
	Last2    int64       `json:"last2,omitempty"`
}

func errResp(msg string) rpcResponse { return rpcResponse{OK: false, Error: msg} }

// ── servidor: expõe um *demoStore local para peers remotos ─────────────────

// serveRace escuta em addr e atende requisições de raceStore sobre store até
// o ctx ser cancelado. Cada conexão roda numa goroutine própria, com
// múltiplas requisições em sequência (conexão persistente — o watcher do
// peer faz um tip() por segundo nela).
func serveRace(ctx context.Context, addr string, store *demoStore) (string, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener fechado (ctx cancelado) ou erro fatal
			}
			go handleRaceConn(conn, store)
		}
	}()
	return ln.Addr().String(), nil
}

func handleRaceConn(conn net.Conn, store *demoStore) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	for {
		var req rpcRequest
		if err := readFrame(r, &req); err != nil {
			return // peer fechou a conexão ou mandou algo inválido
		}
		resp := handleRaceRequest(store, req)
		if err := writeFrame(conn, resp); err != nil {
			return
		}
	}
}

func handleRaceRequest(store *demoStore, req rpcRequest) rpcResponse {
	switch req.Op {
	case "init_meta":
		if req.Meta == nil {
			return errResp("requisição init_meta sem meta")
		}
		m, err := req.Meta.toMeta()
		if err != nil {
			return errResp(err.Error())
		}
		got, created, err := store.initMeta(m)
		if err != nil {
			return errResp(err.Error())
		}
		w := toWireMeta(got)
		return rpcResponse{OK: true, Meta: &w, Created: created}

	case "load_meta":
		m, err := store.loadMeta()
		if err != nil {
			return errResp(err.Error())
		}
		w := toWireMeta(m)
		return rpcResponse{OK: true, Meta: &w}

	case "tip":
		t, err := store.tip()
		if err != nil {
			return errResp(err.Error())
		}
		w := toWireTip(t)
		return rpcResponse{OK: true, Tip: &w}

	case "block_at":
		b, err := store.blockAt(req.Height)
		if err != nil {
			return errResp(err.Error())
		}
		w := toWireBlock(b)
		return rpcResponse{OK: true, Block: &w}

	case "insert_block":
		if req.Block == nil {
			return errResp("requisição insert_block sem block")
		}
		err := store.insertBlock(req.Block.toBlock())
		if errors.Is(err, errRaceLost) {
			return rpcResponse{OK: false, RaceLost: true, Error: err.Error()}
		}
		if err != nil {
			return errResp(err.Error())
		}
		return rpcResponse{OK: true}

	case "miner_balance":
		reward, blocks, err := store.minerBalance(req.Name)
		if err != nil {
			return errResp(err.Error())
		}
		return rpcResponse{OK: true, Reward: reward, Blocks: blocks}

	case "list_blocks":
		rows, err := store.listBlocks(req.Last)
		if err != nil {
			return errResp(err.Error())
		}
		wr := make([]wireBlock, len(rows))
		for i, b := range rows {
			wr[i] = toWireBlock(b)
		}
		return rpcResponse{OK: true, Rows: wr}

	case "ranking":
		ranks, err := store.ranking()
		if err != nil {
			return errResp(err.Error())
		}
		wr := make([]wireRank, len(ranks))
		for i, r := range ranks {
			wr[i] = toWireRank(r)
		}
		return rpcResponse{OK: true, Ranks: wr}

	case "epoch_window":
		first, last, err := store.epochWindow(req.Height, req.Height2)
		if err != nil {
			return errResp(err.Error())
		}
		return rpcResponse{OK: true, First: first, Last2: last}

	default:
		return errResp("operação desconhecida: " + req.Op)
	}
}

// ── cliente: implementa raceStore falando por TCP com um serveRace remoto ──

type netStore struct {
	mu   sync.Mutex
	conn net.Conn
	addr string
}

var _ raceStore = (*netStore)(nil)

// dialRaceStore devolve um cliente para addr SEM discar agora — a primeira
// chamada real (via call) faz a conexão sob demanda, e reconecta sozinha se
// cair. Isso é o que permite ligar um minerador com -peer ANTES do outro
// lado estar de pé: nada aqui falha por causa da rede até você realmente
// tentar usar o store (tip, insertBlock, ...).
func dialRaceStore(addr string) *netStore {
	return &netStore{addr: addr}
}

// call envia uma requisição e espera a resposta na mesma conexão. Se a
// conexão caiu (peer reiniciou, rede oscilou), tenta reconectar uma vez —
// suficiente pra demo sobreviver a um Ctrl+C do outro lado.
func (c *netStore) call(req rpcRequest) (rpcResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		conn, err := net.DialTimeout("tcp", c.addr, 5*time.Second)
		if err != nil {
			return rpcResponse{}, fmt.Errorf("reconectando a %s: %w", c.addr, err)
		}
		c.conn = conn
	}
	c.conn.SetDeadline(time.Now().Add(10 * time.Second))
	if err := writeFrame(c.conn, req); err != nil {
		c.conn.Close()
		c.conn = nil
		return rpcResponse{}, fmt.Errorf("enviando para %s: %w", c.addr, err)
	}
	var resp rpcResponse
	if err := readFrame(c.conn, &resp); err != nil {
		c.conn.Close()
		c.conn = nil
		return rpcResponse{}, fmt.Errorf("lendo resposta de %s: %w", c.addr, err)
	}
	return resp, nil
}

func (c *netStore) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *netStore) initMeta(m demoMeta) (demoMeta, bool, error) {
	wm := toWireMeta(m)
	resp, err := c.call(rpcRequest{Op: "init_meta", Meta: &wm})
	if err != nil {
		return demoMeta{}, false, err
	}
	if !resp.OK {
		return demoMeta{}, false, errors.New(resp.Error)
	}
	got, err := resp.Meta.toMeta()
	return got, resp.Created, err
}

func (c *netStore) loadMeta() (demoMeta, error) {
	resp, err := c.call(rpcRequest{Op: "load_meta"})
	if err != nil {
		return demoMeta{}, err
	}
	if !resp.OK {
		return demoMeta{}, errors.New(resp.Error)
	}
	return resp.Meta.toMeta()
}

func (c *netStore) tip() (demoTip, error) {
	resp, err := c.call(rpcRequest{Op: "tip"})
	if err != nil {
		return demoTip{}, err
	}
	if !resp.OK {
		return demoTip{}, errors.New(resp.Error)
	}
	return resp.Tip.toTip()
}

func (c *netStore) blockAt(height uint64) (demoBlockRow, error) {
	resp, err := c.call(rpcRequest{Op: "block_at", Height: height})
	if err != nil {
		return demoBlockRow{}, err
	}
	if !resp.OK {
		return demoBlockRow{}, errors.New(resp.Error)
	}
	return resp.Block.toBlock(), nil
}

func (c *netStore) insertBlock(b demoBlockRow) error {
	wb := toWireBlock(b)
	resp, err := c.call(rpcRequest{Op: "insert_block", Block: &wb})
	if err != nil {
		return err
	}
	if resp.RaceLost {
		return fmt.Errorf("%w (altura %d)", errRaceLost, b.height)
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	return nil
}

func (c *netStore) minerBalance(name string) (reward, blocks uint64, err error) {
	resp, err := c.call(rpcRequest{Op: "miner_balance", Name: name})
	if err != nil {
		return 0, 0, err
	}
	if !resp.OK {
		return 0, 0, errors.New(resp.Error)
	}
	return resp.Reward, resp.Blocks, nil
}

func (c *netStore) listBlocks(last int) ([]demoBlockRow, error) {
	resp, err := c.call(rpcRequest{Op: "list_blocks", Last: last})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, errors.New(resp.Error)
	}
	out := make([]demoBlockRow, len(resp.Rows))
	for i, w := range resp.Rows {
		out[i] = w.toBlock()
	}
	return out, nil
}

func (c *netStore) ranking() ([]rankRow, error) {
	resp, err := c.call(rpcRequest{Op: "ranking"})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, errors.New(resp.Error)
	}
	out := make([]rankRow, len(resp.Ranks))
	for i, w := range resp.Ranks {
		out[i] = w.toRank()
	}
	return out, nil
}

func (c *netStore) epochWindow(firstH, lastH uint64) (first, last int64, err error) {
	resp, err := c.call(rpcRequest{Op: "epoch_window", Height: firstH, Height2: lastH})
	if err != nil {
		return 0, 0, err
	}
	if !resp.OK {
		return 0, 0, errors.New(resp.Error)
	}
	return resp.First, resp.Last2, nil
}
