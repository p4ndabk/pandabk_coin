package node

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"pandabk_coin/internal/chain"
	"pandabk_coin/internal/core"
	"pandabk_coin/internal/mempool"
	"pandabk_coin/internal/miner"
	"pandabk_coin/internal/p2p"
	"pandabk_coin/internal/params"
	"pandabk_coin/internal/pow"
	"pandabk_coin/internal/wallet"
)

var ErrRPCNotLoopback = errors.New("node: a RPC só aceita bind em loopback (127.0.0.1/::1) — é a interface de controle do dono, não uma API pública")

// Node amarra os subsistemas: chain (estado), mempool (fila), p2p (rede) e
// miner (ligado por padrão). Start sobe tudo; Stop desliga na ordem segura
// (p2p → miner → RPC → bbolt: nunca fechar o banco antes de quem escreve).
type Node struct {
	cfg    Config
	p      params.Params
	chain  *chain.Chain
	mp     *mempool.Mempool
	srv    *p2p.Server
	miner  *miner.Miner
	wallet *wallet.Wallet

	rpcLn  net.Listener
	rpcSrv *http.Server
	cancel context.CancelFunc
}

// New abre o datadir e monta os subsistemas (nada de rede ainda). Se vai
// minerar e o datadir não tem wallet, cria uma (0600) e loga o endereço —
// minerar exige um destino para a coinbase.
func New(cfg Config) (*Node, error) {
	p, err := cfg.Params()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, err
	}
	c, err := chain.Open(cfg.ChainPath(), p)
	if err != nil {
		return nil, fmt.Errorf("abrindo a chain em %s (outro node rodando no mesmo datadir?): %w", cfg.ChainPath(), err)
	}
	n := &Node{cfg: cfg, p: p, chain: c, mp: mempool.New(c, p)}

	if _, err := os.Stat(cfg.WalletPath()); err == nil {
		if n.wallet, err = wallet.Load(cfg.WalletPath()); err != nil {
			c.Close()
			return nil, err
		}
	} else if cfg.Mine {
		if n.wallet, err = wallet.New(cfg.WalletPath()); err != nil {
			c.Close()
			return nil, err
		}
		log.Printf("🔑 wallet nova criada em %s — endereço: %s (faça backup!)", cfg.WalletPath(), n.wallet.Address())
	}

	n.srv = p2p.NewServer(p2p.Config{
		Listen: cfg.Listen,
		Seeds:  cfg.Peers,
		OnBlockAccepted: func(b *core.Block) {
			n.logBlock("📥 bloco %d recebido da rede", b)
			if n.miner != nil {
				n.miner.TipChanged() // bloco da rede: template atual morreu
			}
		},
	}, c, n.mp, p)

	if cfg.Mine {
		if cfg.Miners < 1 {
			c.Close()
			return nil, fmt.Errorf("node: -miners precisa ser ≥ 1 (veio %d)", cfg.Miners)
		}
		n.miner = miner.New(c, n.mp, p, n.wallet.PubKeyHash(), cfg.Miners)
	}
	return n, nil
}

// Start sobe RPC, p2p e miner. A RPC recusa qualquer bind fora de loopback.
func (n *Node) Start() error {
	host, _, err := net.SplitHostPort(n.cfg.RPC)
	if err != nil {
		return fmt.Errorf("node: endereço de RPC inválido %q: %w", n.cfg.RPC, err)
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("%w (veio %q)", ErrRPCNotLoopback, n.cfg.RPC)
	}
	n.rpcLn, err = net.Listen("tcp", n.cfg.RPC)
	if err != nil {
		return fmt.Errorf("node: porta RPC %s em uso? %w", n.cfg.RPC, err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", n.handleRPC)
	n.rpcSrv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = n.rpcSrv.Serve(n.rpcLn) }()

	if err := n.srv.Start(); err != nil {
		_ = n.rpcSrv.Close()
		return fmt.Errorf("node: porta P2P %s em uso? %w", n.cfg.Listen, err)
	}

	var ctx context.Context
	ctx, n.cancel = context.WithCancel(context.Background())
	if n.miner != nil {
		n.miner.Start(ctx, func(b *core.Block) {
			n.logBlock("✅ bloco %d minerado", b)
			n.srv.BroadcastBlock(b.Header.ID())
		})
	}
	return nil
}

// logBlock loga um bloco aceito (nosso ou da rede) com a dificuldade, e
// anuncia os eventos de consenso que o bloco cruza: retarget (dificuldade
// reajustada) e halving (recompensa cortada pela metade).
func (n *Node) logBlock(verb string, b *core.Block) {
	id := b.Header.ID()
	diff := pow.Difficulty(b.Header.Bits, n.p)
	log.Printf(verb+" (%d txs, dificuldade %.2f) — %x", b.Header.Height, len(b.Txs), diff, id[:8])

	h := b.Header.Height
	if h > 0 && n.p.RetargetInterval > 0 && h%n.p.RetargetInterval == 0 {
		if prev, err := n.chain.GetBlock(b.Header.PrevHash); err == nil && prev.Header.Bits != b.Header.Bits {
			old := pow.Difficulty(prev.Header.Bits, n.p)
			log.Printf("🎯 retarget no bloco %d: dificuldade %.2f → %.2f (alvo: 1 bloco a cada %s)", h, old, diff, n.p.TargetSpacing)
		}
	}
	if h > 0 && n.p.HalvingInterval > 0 && h%n.p.HalvingInterval == 0 {
		log.Printf("✂️  halving no bloco %d: recompensa agora %s PANDA por bloco", h, FormatPanda(n.p.BlockSubsidy(h)))
	}
}

// ChainStatus resume o estado para o banner da CLI: altura e dificuldade da
// ponta, e a recompensa que o PRÓXIMO bloco pagará.
func (n *Node) ChainStatus() (height uint64, difficulty float64, nextSubsidy uint64) {
	tip, h, _ := n.chain.Tip()
	return h, pow.Difficulty(tip.Bits, n.p), n.p.BlockSubsidy(h + 1)
}

// Stop desliga na ordem segura e fecha o bbolt por último.
func (n *Node) Stop() error {
	_ = n.srv.Stop()
	if n.miner != nil {
		n.miner.Stop()
	}
	if n.cancel != nil {
		n.cancel()
	}
	if n.rpcSrv != nil {
		_ = n.rpcSrv.Close()
	}
	return n.chain.Close()
}

// RPCAddr é o endereço real da RPC (útil com porta :0 nos testes).
func (n *Node) RPCAddr() string {
	if n.rpcLn == nil {
		return n.cfg.RPC
	}
	return n.rpcLn.Addr().String()
}

// P2PAddr é o endereço real do listener p2p.
func (n *Node) P2PAddr() string { return n.srv.Addr() }
