// powdemo é a bancada de mineração do M1/M1.5: proof of work real (Argon2id
// memory-hard) com retarget de dificuldade e recompensa simulada. Sem -db,
// cada execução minera sua própria "chain" imaginária (modo solo). Com -db,
// vários processos na mesma máquina competem pela MESMA chain usando um
// SQLite compartilhado como quadro-negro da rede — uma prévia didática do
// fork choice (M2) e do P2P (M4): quando dois acham o mesmo bloco, só um
// entra; o outro perdeu a corrida.
package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"pandabk_coin/internal/core"
	"pandabk_coin/internal/params"
	"pandabk_coin/internal/pow"
)

func runPowDemo(args []string) {
	fs := flag.NewFlagSet("powdemo", flag.ExitOnError)
	profileName := fs.String("profile", "devnet", "perfil de consenso: devnet (Argon2 64 MiB) ou test (1 MiB)")
	workers := fs.Int("workers", 1, "goroutines de mineração (1 core e ~64 MiB cada)")
	zeros := fs.Uint("zeros", 8, "dificuldade INICIAL: bits zero à esquerda (cada +1 dobra o trabalho médio)")
	blocks := fs.Int("blocks", 3, "quantos blocos VOCÊ minera antes de sair (0 = sem parar, até Ctrl+C)")
	spacing := fs.Duration("spacing", 60*time.Second, "tempo-alvo entre blocos que o retarget persegue")
	retargetN := fs.Uint64("retarget", 10, "ajustar a dificuldade a cada N blocos")
	progress := fs.Duration("progress", 5*time.Second, "intervalo do relatório de progresso da mineração")
	name := fs.String("name", "", "nome do minerador (aparece nos logs e no placar; default minerador-<pid>)")
	dbPath := fs.String("db", "", "arquivo SQLite local: liga o modo corrida entre mineradores")
	listen := fs.String("listen", "", "endereço host:porta (ex: :9551) para expor -db a mineradores de OUTRAS máquinas")
	peer := fs.String("peer", "", "endereço host:porta de um minerador -listen remoto: minera de forma independente e reconcilia com ele sempre que possível (sem depender dele estar de pé)")
	fs.Parse(args)

	p, err := resolveProfile(*profileName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if *workers < 1 {
		fmt.Fprintln(os.Stderr, "-workers precisa ser >= 1")
		os.Exit(2)
	}
	if *workers > runtime.NumCPU() {
		fmt.Fprintf(os.Stderr, "aviso: %d workers em uma máquina de %d cores\n", *workers, runtime.NumCPU())
	}
	if *zeros < 1 || *zeros > 64 {
		fmt.Fprintln(os.Stderr, "-zeros precisa estar entre 1 e 64")
		os.Exit(2)
	}
	if *retargetN < 1 {
		fmt.Fprintln(os.Stderr, "-retarget precisa ser >= 1")
		os.Exit(2)
	}

	if *dbPath == "" && *peer == "" {
		runPowDemoSolo(p, *workers, *zeros, *blocks, *spacing, *retargetN, *progress)
		return
	}
	if *peer != "" && *dbPath == "" {
		fmt.Fprintln(os.Stderr, "-peer precisa de -db: seu minerador guarda uma cópia local sincronizada,\n"+
			"em vez de depender 100% do peer estar de pé — e assim ele também vira um\n"+
			"-peer válido pra um terceiro node. Ex: -db bob.db -peer 192.168.1.10:9551")
		os.Exit(2)
	}
	if *listen != "" && *dbPath == "" {
		fmt.Fprintln(os.Stderr, "-listen precisa de -db: é o banco local que ele vai expor pra rede")
		os.Exit(2)
	}
	minerName := *name
	if minerName == "" {
		minerName = fmt.Sprintf("minerador-%d", os.Getpid())
	}
	runPowDemoShared(p, minerName, *dbPath, *listen, *peer, *workers, *zeros, *blocks, *spacing, *retargetN, *progress)
}

func resolveProfile(name string) (params.Params, error) {
	switch name {
	case "devnet":
		return params.DevNet(), nil
	case "test":
		return params.TestNet(), nil
	}
	return params.Params{}, fmt.Errorf("perfil desconhecido: %q (use devnet ou test)", name)
}

// initialBits converte "N bits zero à esquerda" na dificuldade compacta
// inicial da demo: target 2^(256-N), em média 2^N tentativas por bloco.
func initialBits(zeros uint) uint32 {
	return pow.TargetToCompact(new(big.Int).Lsh(big.NewInt(1), uint(256-zeros)))
}

// demoRetargetRules monta os parâmetros que o pow.NextBits da demo usa: a
// janela e o alvo vêm das flags/meta, e o limite fica folgado para a
// dificuldade também poder CAIR abaixo da inicial se a máquina for lenta.
func demoRetargetRules(p params.Params, retargetN uint64, spacing time.Duration) params.Params {
	rules := p
	rules.RetargetInterval = retargetN
	rules.TargetSpacing = spacing
	rules.PowLimitBits = 0x207fffff
	return rules
}

// ── modo solo (comportamento original do M1) ───────────────────────────────

func runPowDemoSolo(p params.Params, workers int, zeros uint, blocks int, spacing time.Duration, retargetN uint64, progress time.Duration) {
	rules := demoRetargetRules(p, retargetN, spacing)
	bits := initialBits(zeros)

	fmt.Printf(`⛏  PANDA powdemo — proof of work Argon2id (memory-hard)

   perfil            %s
   memória/hash      %d MiB   ← é isso que expulsa os ASICs
   workers           %d (≈ %d MiB de RAM minerando)
   dificuldade       %#08x inicial (~%s tentativas/bloco)
   retarget          a cada %d blocos, perseguindo %s por bloco
   recompensa        %s PANDA por bloco (simulada — a chain chega no M2)

`, p.Name, p.Argon2Mem/1024, workers, uint32(workers)*(p.Argon2Mem/1024),
		bits, humanCount(avgAttempts(bits)), retargetN, spacing,
		formatPanda(p.BlockSubsidy(1)))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	prev := [32]byte{} // "gênesis" da demo
	start := time.Now()
	epochStart := start
	var totalHashes, totalReward uint64
	var minedCount int

	for height := uint64(1); blocks == 0 || height <= uint64(blocks); height++ {
		fmt.Printf("⛏  [%s] procurando bloco %d | dificuldade %#08x (~%s tentativas em média)\n",
			time.Now().Format("15:04:05"), height, bits, humanCount(avgAttempts(bits)))

		header := core.Header{
			Version:   1,
			Height:    height,
			PrevHash:  prev,
			Timestamp: time.Now().Unix(),
			Bits:      bits,
		}
		// MerkleRoot aleatória simula as transações que um bloco real
		// carregaria (e evita repetir trabalho entre execuções).
		rand.Read(header.MerkleRoot[:])

		target := pow.CompactToTarget(bits)
		found, hashes, elapsed, ok := mine(ctx, header, target, p, workers, progress, "")
		totalHashes += hashes
		if !ok {
			fmt.Println("\ninterrompido.")
			break
		}
		minedCount++

		reward := p.BlockSubsidy(height)
		totalReward += reward
		id := found.ID()
		powHash := pow.PowHash(found.Bytes(), p)
		fmt.Printf("✅ [%s] bloco %d minerado!\n", time.Now().Format("15:04:05"), height)
		fmt.Printf("   ⏱  %s para minerar (alvo %s) | %d tentativas | dificuldade %#08x | nonce %d\n",
			fmtDur(elapsed), spacing, hashes, bits, found.Nonce)
		fmt.Printf("   recompensa  +%s PANDA  →  carteira simulada: %s PANDA\n", formatPanda(reward), formatPanda(totalReward))
		fmt.Printf("   hash PoW    %s\n", hex.EncodeToString(powHash[:]))
		fmt.Printf("   ID do bloco %s\n\n", hex.EncodeToString(id[:]))
		prev = id

		if height%retargetN == 0 {
			now := time.Now()
			epochTook := now.Sub(epochStart)
			expected := time.Duration(retargetN) * spacing
			newBits := pow.NextBits(epochStart.Unix(), now.Unix(), bits, rules)
			fmt.Printf("📈 [%s] RETARGET após %d blocos: época levou %s (esperado %s)\n",
				now.Format("15:04:05"), retargetN, epochTook.Round(time.Second), expected)
			fmt.Printf("   dificuldade %s: %#08x (~%s tent.) → %#08x (~%s tent.)\n\n",
				retargetDirection(bits, newBits), bits, humanCount(avgAttempts(bits)), newBits, humanCount(avgAttempts(newBits)))
			bits = newBits
			epochStart = now
		}
	}

	if minedCount > 0 {
		total := time.Since(start)
		fmt.Printf("── resumo ──────────────────────────────────\n")
		fmt.Printf("   %d blocos em %s (média %.1fs/bloco, alvo %s)\n",
			minedCount, total.Round(time.Second), total.Seconds()/float64(minedCount), spacing)
		fmt.Printf("   %d hashes ≈ %.1f H/s sustentado\n", totalHashes, float64(totalHashes)/total.Seconds())
		fmt.Printf("   carteira simulada: %s PANDA (recompensas ainda são de brincadeira —\n", formatPanda(totalReward))
		fmt.Printf("   viram saldo de verdade quando a chain existir, no M2/M3)\n")
		fmt.Printf("   dificuldade final %#08x (~%s tentativas/bloco)\n", bits, humanCount(avgAttempts(bits)))
	}
}

// ── modo corrida (SQLite local; -listen expõe pra rede, -peer reconcilia com
// outra máquina). NÃO existe autoridade única: cada minerador aceita seus
// próprios blocos localmente e sem esperar ninguém (igual ao M1.5 solo), e
// reconcile.go resolve divergências em segundo plano comparando trabalho
// acumulado — a rede não depende de nenhuma máquina específica continuar de
// pé (ver cmd/node/SINCRONIZACAO.md, Modo 2). ──────────────────────────────

func runPowDemoShared(p params.Params, name, dbPath, listenAddr, peerAddr string, workers int, zeros uint, blocksLimit int, spacing time.Duration, retargetN uint64, progress time.Duration) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	local, err := openDemoStore(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "abrindo %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer local.Close()
	location := dbPath

	// -peer nunca falha aqui: dialRaceStore não disca ainda, então este
	// minerador sobe normalmente mesmo que o outro Mac ainda esteja
	// desligado — reconcile()/peerLoop tentam a conexão de verdade abaixo.
	var peer *netStore
	if peerAddr != "" {
		peer = dialRaceStore(peerAddr)
		defer peer.Close()
		location = fmt.Sprintf("%s (reconciliando com peer %s)", dbPath, peerAddr)
	}

	if listenAddr != "" {
		addr, err := serveRace(ctx, listenAddr, local)
		if err != nil {
			fmt.Fprintf(os.Stderr, "expondo %s em %s: %v\n", dbPath, listenAddr, err)
			os.Exit(1)
		}
		fmt.Printf("📡 expondo %s pela rede em %s — outro Mac pode usar -peer <seu-ip>:%s\n\n",
			dbPath, addr, addr[strings.LastIndex(addr, ":")+1:])
	}

	// A config (perfil/spacing/retarget/zeros) é local, do mesmo jeito que
	// no modo de uma máquina só (M1.5) — sem depender do peer pra decidir
	// nada disso. Com -peer, é responsabilidade de quem roda os dois
	// mineradores passar as MESMAS flags nas duas máquinas (o equivalente
	// demo de "os dois concordam no genesis" antes de ligar a rede).
	wantMeta := demoMeta{profile: p.Name, spacing: spacing, retarget: retargetN, zeros: zeros}
	meta, created, err := local.initMeta(wantMeta)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lendo config local: %v\n", err)
		os.Exit(1)
	}
	if !created && meta != wantMeta {
		fmt.Printf("ℹ️  adotando as regras já gravadas em %s: perfil %s, alvo %s/bloco, retarget a cada %d, zeros iniciais %d\n\n",
			dbPath, meta.profile, meta.spacing, meta.retarget, meta.zeros)
	}
	if p, err = resolveProfile(meta.profile); err != nil {
		fmt.Fprintf(os.Stderr, "perfil gravado é inválido: %v\n", err)
		os.Exit(1)
	}
	rules := demoRetargetRules(p, meta.retarget, meta.spacing)

	// Reconcilia com o peer ANTES de minerar (faz o papel do antigo IBD — se
	// o banco local estiver vazio, reconcile() simplesmente adota a chain
	// inteira do peer) e continua reconciliando em segundo plano pra sempre.
	// Se o peer não responder agora, não é fatal: este node passa a minerar
	// de forma independente e o peerLoop resolve quando ele aparecer.
	if peer != nil {
		startedUp := true
		if err := reconcile(local, peer, name); err != nil {
			fmt.Printf("📴 [%s] %s: peer %s inalcançável no início — minerando de forma independente até ele aparecer\n\n",
				time.Now().Format("15:04:05"), name, peerAddr)
			startedUp = false
		}
		go peerLoop(ctx, local, peer, name, startedUp)
	}

	tip, err := local.tip()
	if err != nil {
		fmt.Fprintf(os.Stderr, "lendo tip: %v\n", err)
		os.Exit(1)
	}
	bits, err := bitsForHeight(local, rules, meta.zeros, tip.height+1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "derivando dificuldade: %v\n", err)
		os.Exit(1)
	}
	balance, myBlocks, err := local.minerBalance(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lendo carteira: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf(`⛏  PANDA powdemo — corrida de mineradores

   minerador         %s
   fonte             %s (altura atual: %d)
   perfil            %s | %d MiB por hash | %d worker(s)
   dificuldade       %#08x atual (~%s tentativas/bloco)
   retarget          a cada %d blocos, perseguindo %s por bloco
   carteira          %s PANDA (%d blocos seus já registrados)

`, name, location, tip.height, p.Name, p.Argon2Mem/1024, workers,
		bits, humanCount(avgAttempts(bits)), meta.retarget, meta.spacing,
		formatPanda(balance), myBlocks)

	sessionStart := time.Now()
	var sessionHashes uint64
	var minedCount int
	lastBits := uint32(0)

	for blocksLimit == 0 || minedCount < blocksLimit {
		if ctx.Err() != nil {
			break
		}
		tip, err = local.tip()
		if err != nil {
			fmt.Fprintf(os.Stderr, "lendo tip: %v\n", err)
			break
		}
		height := tip.height + 1
		bits, err = bitsForHeight(local, rules, meta.zeros, height)
		if err != nil {
			fmt.Fprintf(os.Stderr, "derivando dificuldade: %v\n", err)
			break
		}
		// A dificuldade é derivada do banco, então o retarget "acontece"
		// quando cruzamos uma fronteira de época — anuncie a mudança.
		if lastBits != 0 && bits != lastBits {
			fmt.Printf("📈 [%s] RETARGET no bloco %d: dificuldade %s: %#08x (~%s tent.) → %#08x (~%s tent.)\n\n",
				time.Now().Format("15:04:05"), height, retargetDirection(lastBits, bits),
				lastBits, humanCount(avgAttempts(lastBits)), bits, humanCount(avgAttempts(bits)))
		}
		lastBits = bits

		fmt.Printf("⛏  [%s] %s procurando bloco %d | dificuldade %#08x (~%s tentativas em média)\n",
			time.Now().Format("15:04:05"), name, height, bits, humanCount(avgAttempts(bits)))

		header := core.Header{
			Version:   1,
			Height:    height,
			PrevHash:  tip.id,
			Timestamp: time.Now().Unix(),
			Bits:      bits,
		}
		rand.Read(header.MerkleRoot[:])
		target := pow.CompactToTarget(bits)

		// Watcher: se outro minerador estender a chain enquanto trabalhamos,
		// cancela a busca atual — é o equivalente demo de "um bloco novo
		// chegou pela rede".
		mineCtx, cancelMine := context.WithCancel(ctx)
		watchDone := make(chan struct{})
		go func() {
			defer close(watchDone)
			t := time.NewTicker(time.Second)
			defer t.Stop()
			for {
				select {
				case <-mineCtx.Done():
					return
				case <-t.C:
					if nt, err := local.tip(); err == nil && nt.height >= height {
						cancelMine()
						return
					}
				}
			}
		}()
		found, hashes, elapsed, ok := mine(mineCtx, header, target, p, workers, progress, name+"  ")
		cancelMine()
		<-watchDone
		sessionHashes += hashes

		if !ok {
			if ctx.Err() != nil {
				fmt.Println("\ninterrompido.")
				break
			}
			nt, err := local.tip()
			if err != nil || nt.height < height {
				continue // cancelamento espúrio; tenta de novo
			}
			for h := height; h <= nt.height; h++ {
				row, err := local.blockAt(h)
				if err != nil {
					continue
				}
				fmt.Printf("📥 [%s] %s minerou o bloco %d (+%s PANDA para ele, ⏱ %s) — %s de trabalho descartado, recomeçando no %d\n\n",
					time.Now().Format("15:04:05"), row.miner, h, formatPanda(row.reward),
					fmtDur(time.Duration(row.durationMS)*time.Millisecond), fmtDur(elapsed), nt.height+1)
			}
			continue
		}

		// A base que usamos pra minerar (tip.id) pode ter mudado enquanto
		// procurávamos, de um jeito que o vigia acima NÃO pega: um reorg
		// (peerLoop/reconcile) que troca de chain e, raramente, deixa a
		// altura local MENOR do que quando começamos (peer com fork mais
		// curto porém mais pesado). O vigia só cancela quando a altura
		// AUMENTA; esta checagem cobre o resto — sem ela, inserir aqui
		// criaria um buraco na tabela (uma altura sem o bloco anterior).
		if curTip, err := local.tip(); err != nil || curTip.id != tip.id {
			fmt.Printf("♻️  [%s] a base do bloco %d mudou enquanto minerava (reorg) — descartando e recomeçando\n\n",
				time.Now().Format("15:04:05"), height)
			continue
		}

		reward := p.BlockSubsidy(height)
		id := found.ID()
		row := demoBlockRow{
			height: height, id: hex.EncodeToString(id[:]), prev: hex.EncodeToString(tip.id[:]),
			bits: bits, nonce: found.Nonce, miner: name, reward: reward,
			attempts: hashes, durationMS: elapsed.Milliseconds(), foundAt: time.Now().Unix(),
		}
		err = local.insertBlock(row)
		if errors.Is(err, errRaceLost) {
			who := "outro minerador"
			if winner, werr := local.blockAt(height); werr == nil {
				who = winner.miner
			}
			fmt.Printf("🐼 [%s] %s registrou o bloco %d primeiro — você perdeu a corrida (%s de trabalho descartado)\n\n",
				time.Now().Format("15:04:05"), who, height, fmtDur(elapsed))
			continue
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "gravando bloco: %v\n", err)
			continue
		}
		if peer != nil {
			// Melhor esforço: avisa o peer JÁ (sem esperar o próximo tick do
			// peerLoop). Se falhar — peer fora do ar, ou a chain dele já
			// divergiu — sem problema, o peerLoop reconcilia em ~1s de
			// qualquer jeito. O bloco já é seu, gravado localmente, com ou
			// sem o peer.
			_ = peer.insertBlock(row)
		}
		minedCount++
		myBlocks++
		balance += reward
		powHash := pow.PowHash(found.Bytes(), p)
		fmt.Printf("✅ [%s] bloco %d minerado por %s!\n", time.Now().Format("15:04:05"), height, name)
		fmt.Printf("   ⏱  %s para minerar (alvo %s) | %d tentativas | dificuldade %#08x | nonce %d\n",
			fmtDur(elapsed), meta.spacing, hashes, bits, found.Nonce)
		fmt.Printf("   recompensa  +%s PANDA  →  sua carteira: %s PANDA (%d blocos)\n",
			formatPanda(reward), formatPanda(balance), myBlocks)
		fmt.Printf("   hash PoW    %s\n", hex.EncodeToString(powHash[:]))
		fmt.Printf("   ID do bloco %s\n\n", hex.EncodeToString(id[:]))
	}

	total := time.Since(sessionStart)
	fmt.Printf("── resumo da sessão (%s) ───────────────────\n", name)
	fmt.Printf("   %d blocos seus nesta sessão em %s | %d hashes ≈ %.1f H/s\n",
		minedCount, total.Round(time.Second), sessionHashes, float64(sessionHashes)/total.Seconds())
	fmt.Printf("   carteira total: %s PANDA (%d blocos no banco)\n\n", formatPanda(balance), myBlocks)
	// Lê do banco LOCAL (sempre correto — nunca dependeu do peer pra
	// aceitar blocos), então o placar final não depende do peer estar de pé.
	if err := printRanking(local, rules, meta); err != nil {
		fmt.Fprintf(os.Stderr, "placar: %v\n", err)
	}
}

func retargetDirection(oldBits, newBits uint32) string {
	switch {
	case avgAttempts(newBits) > avgAttempts(oldBits):
		return "SUBIU (blocos saíram rápido demais)"
	case avgAttempts(newBits) < avgAttempts(oldBits):
		return "DESCEU (blocos saíram devagar demais)"
	}
	return "mantida"
}

// mine varre nonces em paralelo até um worker achar hash < target (ok=true)
// ou o contexto ser cancelado (ok=false — Ctrl+C ou outro minerador achou
// primeiro). Cada worker recebe uma faixa disjunta de nonces; a MerkleRoot
// aleatória já diferencia os blocos da demo entre execuções.
func mine(ctx context.Context, header core.Header, target *big.Int, p params.Params, workers int, progress time.Duration, who string) (core.Header, uint64, time.Duration, bool) {
	start := time.Now()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var counter atomic.Uint64
	foundCh := make(chan core.Header, workers)

	stride := ^uint64(0) / uint64(workers)
	for w := 0; w < workers; w++ {
		go func(startNonce uint64) {
			h := header
			buf := h.Bytes()
			for nonce := startNonce; ctx.Err() == nil; nonce++ {
				binary.BigEndian.PutUint64(buf[core.HeaderSize-8:], nonce)
				hash := pow.PowHash(buf, p)
				counter.Add(1)
				if new(big.Int).SetBytes(hash[:]).Cmp(target) <= 0 {
					h.Nonce = nonce
					select {
					case foundCh <- h:
					case <-ctx.Done():
					}
					return
				}
			}
		}(uint64(w) * stride)
	}

	ticker := time.NewTicker(progress)
	defer ticker.Stop()
	var lastCount uint64
	var lastTick = start
	for {
		select {
		case found := <-foundCh:
			return found, counter.Load(), time.Since(start), true
		case <-ticker.C:
			now := counter.Load()
			fmt.Printf("   ⛏  %s%5.1f H/s | %6d tentativas | %4.0fs procurando bloco %d...\n",
				who, float64(now-lastCount)/time.Since(lastTick).Seconds(),
				now, time.Since(start).Seconds(), header.Height)
			lastCount = now
			lastTick = time.Now()
		case <-ctx.Done():
			return header, counter.Load(), time.Since(start), false
		}
	}
}

// avgAttempts converte nBits no número médio de hashes por bloco
// (o "trabalho" que a dificuldade representa).
func avgAttempts(bits uint32) float64 {
	f, _ := new(big.Float).SetInt(pow.BlockWork(bits)).Float64()
	return f
}

func formatPanda(subunits uint64) string {
	whole := subunits / params.CoinUnit
	frac := subunits % params.CoinUnit
	if frac == 0 {
		return fmt.Sprintf("%d", whole)
	}
	return strings.TrimRight(fmt.Sprintf("%d.%08d", whole, frac), "0")
}

func humanCount(n float64) string {
	switch {
	case n >= 1e9:
		return fmt.Sprintf("%.1f bi", n/1e9)
	case n >= 1e6:
		return fmt.Sprintf("%.1f mi", n/1e6)
	case n >= 1e3:
		return fmt.Sprintf("%.1f mil", n/1e3)
	default:
		return fmt.Sprintf("%.0f", n)
	}
}

func fmtDur(d time.Duration) string {
	if d < 10*time.Second {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return d.Round(time.Second).String()
}
