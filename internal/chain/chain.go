package chain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"

	"pandabk_coin/internal/core"
	"pandabk_coin/internal/params"
	"pandabk_coin/internal/pow"
)

// Chain é a blockchain persistida de um node: valida e aceita blocos,
// mantém o UTXO set e decide a cadeia ativa por trabalho acumulado.
// Concreta e single-writer (mutex próprio + lock de arquivo do bbolt).
type Chain struct {
	db      *bolt.DB
	p       params.Params
	mu      sync.Mutex
	orphans orphanPool
	now     func() time.Time // injetável nos testes de timestamp
}

// UTXO é a visão externa de um output não gasto (para wallet/balance/mempool).
type UTXO struct {
	Prev     core.OutPoint
	Value    uint64
	PKH      [20]byte
	Height   uint64
	Coinbase bool
}

// Open cria/abre o banco em path e garante o gênesis do perfil: banco novo
// recebe o bloco 0; banco existente é conferido contra ele (abrir um banco
// de outra rede/perfil é erro, não corrupção silenciosa).
func Open(path string, p params.Params) (*Chain, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, err
	}
	c := &Chain{db: db, p: p, orphans: newOrphanPool(), now: time.Now}
	err = db.Update(func(btx *bolt.Tx) error {
		for _, name := range allBuckets {
			if _, err := btx.CreateBucketIfNotExists(name); err != nil {
				return err
			}
		}
		g := GenesisBlock(p)
		want := g.Header.ID()
		if got := btx.Bucket(bucketHeight).Get(heightKey(0)); got != nil {
			if !bytes.Equal(got, want[:]) {
				return fmt.Errorf("%w — este datadir pertence a OUTRA rede (perfil ou regras de build diferentes). Guarde/remova o chain.db antigo para começar nesta rede, ou use outro -datadir; a wallet.json continua válida", ErrBadGenesis)
			}
			return nil
		}
		return insertGenesis(btx, p)
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	return c, nil
}

func (c *Chain) Close() error {
	return c.db.Close()
}

// AcceptBlock valida b contra todas as regras de consenso e o incorpora:
// conecta na ponta, guarda como ramo lateral ou dispara reorg se o ramo
// dele acumular mais trabalho. Bloco duplicado é no-op sem erro; bloco sem
// pai vai pro pool de órfãos e retorna ErrOrphan (o p2p usa isso para pedir
// o pai). Aceitar um bloco pode destravar órfãos em cascata.
func (c *Chain) AcceptBlock(b *core.Block) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.accept(b); err != nil {
		return err
	}
	queue := [][32]byte{b.Header.ID()}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		for _, child := range c.orphans.takeChildren(parent) {
			if c.accept(child) == nil {
				queue = append(queue, child.Header.ID())
			}
		}
	}
	return nil
}

// accept processa um único bloco, na ordem barato→caro (ver SPEC):
// duplicado → sanidade → pai conhecido → contexto do header → txs/UTXO →
// PoW (Argon2id) por último, dentro do connect atômico.
func (c *Chain) accept(b *core.Block) error {
	id := b.Header.ID()

	var dup, dupInvalid bool
	if err := c.db.View(func(btx *bolt.Tx) error {
		e, ok, err := getIndexEntry(btx, id)
		if err != nil {
			return err
		}
		dup, dupInvalid = ok, ok && e.Status == statusInvalid
		return nil
	}); err != nil {
		return err
	}
	if dup {
		if dupInvalid {
			return ErrKnownInvalid
		}
		return nil
	}

	raw := b.Bytes()
	if err := checkSanity(b, raw, c.p); err != nil {
		return err
	}
	if b.Header.Height == 0 {
		// O único bloco 0 legítimo entra no Open; qualquer outro é fraude.
		return ErrBadGenesis
	}

	var prevE indexEntry
	var prevOK bool
	var tipID [32]byte
	var tipE indexEntry
	var ctxErr error
	if err := c.db.View(func(btx *bolt.Tx) error {
		var err error
		prevE, prevOK, err = getIndexEntry(btx, b.Header.PrevHash)
		if err != nil || !prevOK {
			return err
		}
		if prevE.Status == statusInvalid {
			ctxErr = ErrKnownInvalid
			return nil
		}
		if ctxErr = checkHeaderContext(btx, &b.Header, c.p, c.now().Unix()); ctxErr != nil {
			return nil
		}
		tipID, tipE, err = tipState(btx)
		return err
	}); err != nil {
		return err
	}
	if !prevOK {
		c.orphans.add(b)
		return ErrOrphan
	}
	if ctxErr != nil {
		return ctxErr
	}

	cum := new(big.Int).Add(prevE.CumWork, pow.BlockWork(b.Header.Bits))

	if b.Header.PrevHash == tipID {
		// Caminho comum: estende a ponta. Validação completa + connect numa
		// transação só — erro em qualquer regra desfaz tudo.
		return c.db.Update(func(btx *bolt.Tx) error {
			return connectBlock(btx, b, raw, id, cum, c.p, false)
		})
	}

	// Ramo lateral: PoW agora (lixo sem trabalho não ocupa disco); a
	// validação completa das txs acontece se/quando o ramo vencer (reorg).
	if err := pow.CheckProofOfWork(&b.Header, c.p); err != nil {
		return err
	}
	if err := c.db.Update(func(btx *bolt.Tx) error {
		if err := btx.Bucket(bucketBlocks).Put(id[:], raw); err != nil {
			return err
		}
		return putIndexEntry(btx, id, indexEntry{Height: b.Header.Height, Status: statusSide, CumWork: cum})
	}); err != nil {
		return err
	}
	if cum.Cmp(tipE.CumWork) > 0 {
		return c.reorgTo(id)
	}
	return nil
}

// Tip devolve o header, a altura e o trabalho acumulado da ponta ativa.
func (c *Chain) Tip() (core.Header, uint64, *big.Int) {
	var hdr core.Header
	var height uint64
	work := new(big.Int)
	_ = c.db.View(func(btx *bolt.Tx) error {
		id, e, err := tipState(btx)
		if err != nil {
			return err
		}
		if hdr, err = readHeader(btx, id); err != nil {
			return err
		}
		height, work = e.Height, e.CumWork
		return nil
	})
	return hdr, height, work
}

// GetBlock busca um bloco pelo ID (qualquer status).
func (c *Chain) GetBlock(id [32]byte) (*core.Block, error) {
	var blk *core.Block
	err := c.db.View(func(btx *bolt.Tx) error {
		b, _, err := getBlock(btx, id)
		blk = b
		return err
	})
	return blk, err
}

// LocatorHashes resume a cadeia ativa para o sync do p2p: os últimos 10
// blocos um a um, depois espaçamento dobrando, terminando no gênesis — um
// peer acha o último ancestral em comum em O(log n) mensagens.
func (c *Chain) LocatorHashes() [][32]byte {
	var loc [][32]byte
	_ = c.db.View(func(btx *bolt.Tx) error {
		_, tipE, err := tipState(btx)
		if err != nil {
			return err
		}
		hb := btx.Bucket(bucketHeight)
		step := uint64(1)
		h := tipE.Height
		for {
			raw := hb.Get(heightKey(h))
			if len(raw) == 32 {
				var id [32]byte
				copy(id[:], raw)
				loc = append(loc, id)
			}
			if h == 0 {
				break
			}
			if len(loc) >= 10 {
				step *= 2
			}
			if h < step {
				h = 0
			} else {
				h -= step
			}
		}
		return nil
	})
	return loc
}

// NextBits é o nBits que o PRÓXIMO bloco (filho da ponta atual) deve
// declarar — o miner usa para montar o template; a validação usa a mesma
// conta, então template e consenso nunca divergem.
func (c *Chain) NextBits() (uint32, error) {
	var bits uint32
	err := c.db.View(func(btx *bolt.Tx) error {
		id, _, err := tipState(btx)
		if err != nil {
			return err
		}
		tip, err := readHeader(btx, id)
		if err != nil {
			return err
		}
		bits, err = expectedBits(btx, tip, c.p)
		return err
	})
	return bits, err
}

// HeadersSince serve o getheaders do p2p: acha no locator o bloco mais alto
// que ainda está na cadeia ativa (o gênesis sempre está — mesma rede) e
// devolve os headers seguintes, em ordem, até max.
func (c *Chain) HeadersSince(locator [][32]byte, max int) ([]core.Header, error) {
	var out []core.Header
	err := c.db.View(func(btx *bolt.Tx) error {
		hb := btx.Bucket(bucketHeight)
		var start uint64 // fallback: do gênesis em diante
		for _, id := range locator {
			e, ok, err := getIndexEntry(btx, id)
			if err != nil {
				return err
			}
			if !ok || e.Status != statusActive {
				continue
			}
			if raw := hb.Get(heightKey(e.Height)); len(raw) == 32 && bytes.Equal(raw, id[:]) {
				start = e.Height
				break
			}
		}
		_, tipE, err := tipState(btx)
		if err != nil {
			return err
		}
		for h := start + 1; h <= tipE.Height && len(out) < max; h++ {
			raw := hb.Get(heightKey(h))
			if len(raw) != 32 {
				break
			}
			var id [32]byte
			copy(id[:], raw)
			hdr, err := readHeader(btx, id)
			if err != nil {
				return err
			}
			out = append(out, hdr)
		}
		return nil
	})
	return out, err
}

// SaveAddrBook / LoadAddrBook persistem o address book do p2p no bucket meta
// (JSON simples) — um node que reinicia rediscar a rede sem depender dos
// seeds de novo.
func (c *Chain) SaveAddrBook(addrs []string) error {
	data, err := json.Marshal(addrs)
	if err != nil {
		return err
	}
	return c.db.Update(func(btx *bolt.Tx) error {
		return btx.Bucket(bucketMeta).Put(keyAddrBook, data)
	})
}

func (c *Chain) LoadAddrBook() ([]string, error) {
	var addrs []string
	err := c.db.View(func(btx *bolt.Tx) error {
		raw := btx.Bucket(bucketMeta).Get(keyAddrBook)
		if raw == nil {
			return nil
		}
		return json.Unmarshal(raw, &addrs)
	})
	return addrs, err
}

// UTXOsByPKH varre o UTXO set filtrando por dono — a fonte do balance e da
// seleção de moedas da wallet (M3).
func (c *Chain) UTXOsByPKH(pkh [20]byte) ([]UTXO, error) {
	var out []UTXO
	err := c.db.View(func(btx *bolt.Tx) error {
		return btx.Bucket(bucketUTXO).ForEach(func(k, v []byte) error {
			e, err := decodeUTXOEntry(v)
			if err != nil {
				return err
			}
			if e.PKH != pkh {
				return nil
			}
			var op core.OutPoint
			copy(op.TxID[:], k[:32])
			op.Index = uint32(k[32])<<24 | uint32(k[33])<<16 | uint32(k[34])<<8 | uint32(k[35])
			out = append(out, UTXO{Prev: op, Value: e.Value, PKH: e.PKH, Height: e.Height, Coinbase: e.Coinbase})
			return nil
		})
	})
	return out, err
}

// LookupUTXO busca um único outpoint no UTXO set da cadeia ativa — a
// consulta que o mempool faz por input ao admitir uma transação.
func (c *Chain) LookupUTXO(op core.OutPoint) (UTXO, bool, error) {
	var u UTXO
	found := false
	err := c.db.View(func(btx *bolt.Tx) error {
		raw := btx.Bucket(bucketUTXO).Get(outPointKey(op))
		if raw == nil {
			return nil
		}
		e, err := decodeUTXOEntry(raw)
		if err != nil {
			return err
		}
		u = UTXO{Prev: op, Value: e.Value, PKH: e.PKH, Height: e.Height, Coinbase: e.Coinbase}
		found = true
		return nil
	})
	return u, found, err
}
