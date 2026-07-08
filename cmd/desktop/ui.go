package main

import (
	"fmt"
	"image/color"
	"log"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"zhu/internal/node"
	"zhu/internal/rpcclient"
)

// ui é o estado da janela: de que node ela fala (externo via RPC ou
// embutido) e os textos que o loop de refresh atualiza.
type ui struct {
	app      fyne.App
	win      fyne.Window
	cfg      *node.Config
	confPath string
	logs     *logStore
	version  string
	external bool
	embedded *node.Node

	rpcMu sync.Mutex
	rpc   string // endereço RPC vivo (muda num restart) — use rpcAddr/setRPC

	// referências vivas das abas (atualizadas via fyne.Do no loop)
	cardValues    map[string]*canvas.Text
	cardSubs      map[string]*canvas.Text
	blocksList    *widget.Label
	mempoolList   *widget.Label
	consensusLine *widget.Label
	walletAddr    *widget.Entry
	walletTotal   *canvas.Text
	walletSpend   *canvas.Text
	walletUTXOs   *widget.Label
	walletActs    *widget.Label
	actsPageLbl   *widget.Label
	actsPrev      *widget.Button
	actsNext      *widget.Button
	logView       *widget.Label
	logScroll     *container.Scroll

	// extrato: filtro e página vêm de callbacks da UI, a recarga roda em
	// goroutine — tudo que os dois lados tocam fica atrás do actsMu
	actsMu     sync.Mutex
	actsFilter string // "all" | "tx" | "mined"
	actsPage   int

	// só a goroutine do refreshLoop toca nestes dois
	actsHeight  uint64
	actsFetched bool
}

const actsPageSize = 15

// infoResp espelha o getinfo do node (internal/node/rpc.go).
type infoResp struct {
	Profile     string  `json:"profile"`
	Height      uint64  `json:"height"`
	Difficulty  float64 `json:"difficulty"`
	SpacingSecs int64   `json:"target_spacing_seconds"`
	RewardZhu   string  `json:"reward_zhu"`
	NextHalving uint64  `json:"next_halving"`
	Peers       int     `json:"peers"`
	Mempool     int     `json:"mempool"`
	Mining      bool    `json:"mining"`
	HashRate    float64 `json:"hashrate"`
	Address     string  `json:"address"`
}

type balanceResp struct {
	Address      string `json:"address"`
	BalanceZhu   string `json:"balance_zhu"`
	SpendableZhu string `json:"spendable_zhu"`
	UTXOs        int    `json:"utxos"`
}

type statsResp struct {
	AvgBlockSecs   float64 `json:"avg_block_seconds"`
	AvgWindow      int     `json:"avg_window"`
	TargetSecs     int64   `json:"target_seconds"`
	BlocksToRetgt  uint64  `json:"blocks_to_retarget"`
	RetargetFactor float64 `json:"retarget_factor"`
	BlocksToHalve  uint64  `json:"blocks_to_halving"`
	RewardZhu      string  `json:"reward_zhu"`
	NextReward     string  `json:"next_reward_zhu"`
}

type recentResp struct {
	Height  uint64 `json:"height"`
	Time    int64  `json:"time"`
	Txs     int    `json:"txs"`
	Miner   string `json:"miner"`
	Elapsed int64  `json:"elapsed"`
}

type mempoolResp struct {
	TxID     string  `json:"txid"`
	Size     int     `json:"size"`
	ValueZhu string  `json:"value_zhu"`
	FeeZhu   string  `json:"fee_zhu"`
	FeeRate  float64 `json:"fee_rate"`
}

type activityResp struct {
	Height       uint64 `json:"height"`
	Time         int64  `json:"time"`
	Direction    string `json:"direction"`
	AmountZhu    string `json:"amount_zhu"`
	FeeZhu       string `json:"fee_zhu"`
	Coinbase     bool   `json:"coinbase"`
	Counterparty string `json:"counterparty"`
}

// rpcAddr/setRPC protegem o endereço RPC: o refreshLoop lê de uma goroutine
// enquanto um restart (aba Ajustes) pode trocá-lo.
func (u *ui) rpcAddr() string {
	u.rpcMu.Lock()
	defer u.rpcMu.Unlock()
	return u.rpc
}

func (u *ui) setRPC(addr string) {
	u.rpcMu.Lock()
	u.rpc = addr
	u.rpcMu.Unlock()
}

// startAndBuild decide o modo (painel de node externo ou node embutido),
// sobe o que for preciso e monta as abas. Erro de partida vira uma tela de
// erro com botão de fechar — nunca um crash mudo.
func (u *ui) startAndBuild() {
	if err := rpcclient.CallTimeout(u.cfg.RPC, "getinfo", nil, nil, 700*time.Millisecond); err == nil {
		u.setRPC(u.cfg.RPC)
		u.external = true
		log.Printf("🖥️  conectado ao node em execução em %s (modo painel)", u.cfg.RPC)
	} else {
		n, err := node.New(*u.cfg)
		if err == nil {
			err = n.Start()
		}
		if err != nil {
			u.showFatal(err)
			return
		}
		u.embedded = n
		u.setRPC(n.RPCAddr())
		log.Printf("🖥️  node embutido no ar (RPC %s)", u.rpcAddr())
	}
	u.build()
}

func (u *ui) build() {
	u.cardValues = map[string]*canvas.Text{}
	u.cardSubs = map[string]*canvas.Text{}

	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Início", theme.HomeIcon(), u.statusTab()),
		container.NewTabItemWithIcon("Carteira", theme.AccountIcon(), u.walletTab()),
		container.NewTabItemWithIcon("Enviar", theme.MailSendIcon(), u.sendTab()),
		container.NewTabItemWithIcon("Blocos", theme.SearchIcon(), u.blocksTab()),
		container.NewTabItemWithIcon("Atividade", theme.ListIcon(), u.logsTab()),
		container.NewTabItemWithIcon("Ajustes", theme.SettingsIcon(), u.settingsTab()),
	)
	tabs.SetTabLocation(container.TabLocationTop)
	u.win.SetContent(tabs)

	u.win.SetCloseIntercept(func() {
		if u.embedded == nil {
			u.app.Quit()
			return
		}
		dialog.NewConfirm("Encerrar", "Desligar o node e sair?\nA chain fecha com segurança e continua de onde parou.", func(ok bool) {
			if !ok {
				return
			}
			go func() {
				if err := u.embedded.Stop(); err != nil {
					log.Printf("encerrando: %v", err)
				}
				fyne.Do(u.app.Quit)
			}()
		}, u.win).Show()
	})

	go u.refreshLoop()
}

// refreshLoop mantém a janela viva: getinfo/getbalance a cada 2s e a aba
// Atividade a cada 1s (só quando o log mudou). Toda mutação de UI passa por
// fyne.Do — regra de threading do Fyne.
func (u *ui) refreshLoop() {
	u.refresh()
	info := time.NewTicker(2 * time.Second)
	logs := time.NewTicker(time.Second)
	defer info.Stop()
	defer logs.Stop()
	for {
		select {
		case <-info.C:
			u.refresh()
		case <-logs.C:
			if text, changed := u.logs.Snapshot(); changed && u.logView != nil {
				fyne.Do(func() {
					u.logView.SetText(text)
					u.logScroll.ScrollToBottom()
				})
			}
		}
	}
}

func (u *ui) refresh() {
	addr := u.rpcAddr()
	var info infoResp
	if err := rpcclient.CallTimeout(addr, "getinfo", nil, &info, 3*time.Second); err != nil {
		fyne.Do(func() { u.setCard("peers", "—"); u.consensusLine.SetText("sem resposta do node — " + err.Error()) })
		return
	}
	var bal balanceResp
	balErr := rpcclient.CallTimeout(addr, "getbalance", nil, &bal, 3*time.Second)
	var st statsResp
	stErr := rpcclient.CallTimeout(addr, "getstats", nil, &st, 3*time.Second)
	var recent []recentResp
	recErr := rpcclient.CallTimeout(addr, "getrecentblocks", map[string]int{"count": 10}, &recent, 3*time.Second)
	var pending []mempoolResp
	mpErr := rpcclient.CallTimeout(addr, "getmempool", nil, &pending, 3*time.Second)

	// extrato da wallet: varre a chain no node, então só quando há bloco novo
	if !u.actsFetched || info.Height != u.actsHeight {
		if u.reloadActs() == nil {
			u.actsFetched, u.actsHeight = true, info.Height
		}
	}

	fyne.Do(func() {
		u.setCard("height", formatUint(info.Height))
		u.setCard("difficulty", formatFloat(info.Difficulty))
		u.setCard("peers", formatUint(uint64(info.Peers)))
		u.setCard("mempool", formatUint(uint64(info.Mempool)))
		if info.Mining {
			u.setCard("hashrate", formatFloat(info.HashRate))
		} else {
			u.setCard("hashrate", "off")
		}
		u.consensusLine.SetText(consensusText(info))
		if balErr == nil {
			u.setCard("balance", bal.BalanceZhu)
			u.walletTotal.Text = bal.BalanceZhu + " ZHU"
			u.walletSpend.Text = bal.SpendableZhu + " ZHU gastáveis agora"
			u.walletTotal.Refresh()
			u.walletSpend.Refresh()
			u.walletUTXOs.SetText(formatUint(uint64(bal.UTXOs)) + " UTXOs — moedas de mineração maturam em 10 blocos")
		}
		if info.Address != "" && u.walletAddr.Text != info.Address {
			u.walletAddr.SetText(info.Address)
		}
		if stErr == nil {
			u.applyStats(st)
		}
		if recErr == nil {
			u.blocksList.SetText(formatRecentBlocks(recent))
		}
		if mpErr == nil {
			u.mempoolList.SetText(formatMempoolList(pending))
		}
	})
}

// applyStats preenche a fileira mempool.space-de-casa: ritmo real vs alvo,
// previsão do retarget e contagem regressiva do halving.
func (u *ui) applyStats(st statsResp) {
	if st.AvgWindow > 0 {
		u.setCard("avgtime", fmt.Sprintf("%.0fs", st.AvgBlockSecs))
		u.setCardSub("avgtime", fmt.Sprintf("ALVO %dS · ÚLTIMOS %d BLOCOS", st.TargetSecs, st.AvgWindow))
	} else {
		u.setCard("avgtime", "—")
		u.setCardSub("avgtime", fmt.Sprintf("ALVO %dS · AGUARDANDO BLOCOS", st.TargetSecs))
	}
	u.setCard("retarget", fmt.Sprintf("em %d", st.BlocksToRetgt))
	switch {
	case st.RetargetFactor > 1.02:
		u.setCardSub("retarget", fmt.Sprintf("DIFICULDADE DEVE SUBIR ↑ ×%.2f", st.RetargetFactor))
	case st.RetargetFactor > 0 && st.RetargetFactor < 0.98:
		u.setCardSub("retarget", fmt.Sprintf("DIFICULDADE DEVE CAIR ↓ ×%.2f", st.RetargetFactor))
	case st.RetargetFactor > 0:
		u.setCardSub("retarget", "RITMO NO ALVO — ESTÁVEL")
	default:
		u.setCardSub("retarget", "BLOCOS (SEM DADOS AINDA)")
	}
	u.setCard("halving", fmt.Sprintf("em %d", st.BlocksToHalve))
	u.setCardSub("halving", fmt.Sprintf("RECOMPENSA %s → %s ZHU", st.RewardZhu, st.NextReward))
}

// reloadActs busca a página atual do extrato (filtro + offset) e atualiza a
// aba Carteira. Pede um lançamento a mais que a página para saber se existe
// próxima. Uma resposta que chega atrasada, depois de o usuário já ter
// mudado filtro/página, é descartada.
func (u *ui) reloadActs() error {
	u.actsMu.Lock()
	filter, page := u.actsFilter, u.actsPage
	u.actsMu.Unlock()
	params := map[string]any{"count": actsPageSize + 1, "offset": page * actsPageSize}
	if filter != "" && filter != "all" {
		params["filter"] = filter
	}
	var acts []activityResp
	if err := rpcclient.CallTimeout(u.rpcAddr(), "getactivity", params, &acts, 5*time.Second); err != nil {
		return err
	}
	more := len(acts) > actsPageSize
	if more {
		acts = acts[:actsPageSize]
	}
	fyne.Do(func() {
		u.actsMu.Lock()
		stale := filter != u.actsFilter || page != u.actsPage
		u.actsMu.Unlock()
		if stale {
			return
		}
		text := formatActivity(acts)
		if len(acts) == 0 && page > 0 {
			text = "fim do extrato — volte uma página"
		}
		u.walletActs.SetText(text)
		u.actsPageLbl.SetText(fmt.Sprintf("página %d", page+1))
		if page > 0 {
			u.actsPrev.Enable()
		} else {
			u.actsPrev.Disable()
		}
		if more {
			u.actsNext.Enable()
		} else {
			u.actsNext.Disable()
		}
	})
	return nil
}

// setActsFilter/turnActsPage reagem aos controles do extrato: mudou de
// verdade → volta o estado e recarrega em goroutine (nunca RPC na thread da
// UI).
func (u *ui) setActsFilter(filter string) {
	u.actsMu.Lock()
	changed := filter != u.actsFilter
	u.actsFilter, u.actsPage = filter, 0
	u.actsMu.Unlock()
	if changed {
		go func() { _ = u.reloadActs() }()
	}
}

func (u *ui) turnActsPage(delta int) {
	u.actsMu.Lock()
	next := max(u.actsPage+delta, 0)
	changed := next != u.actsPage
	u.actsPage = next
	u.actsMu.Unlock()
	if changed {
		go func() { _ = u.reloadActs() }()
	}
}

func (u *ui) setCardSub(key, text string) {
	if t, ok := u.cardSubs[key]; ok && t.Text != text {
		t.Text = text
		t.Refresh()
	}
}

func (u *ui) setCard(key, value string) {
	if t, ok := u.cardValues[key]; ok && t.Text != value {
		t.Text = value
		t.Refresh()
	}
}

// showFatal troca o conteúdo da janela por uma tela de erro clara (não
// chama ShowAndRun — o main é dono do loop).
func (u *ui) showFatal(err error) {
	log.Printf("erro na inicialização: %v", err)
	msg := widget.NewLabel(err.Error())
	msg.Wrapping = fyne.TextWrapWord
	title := canvas.NewText("Não consegui iniciar o node", theme.Color(theme.ColorNameForeground))
	title.TextSize = 22
	title.FontSource = fontMedium
	u.win.SetContent(container.NewCenter(container.NewVBox(
		title, msg,
		widget.NewButton("Fechar", u.app.Quit),
	)))
}

// ── helpers visuais ─────────────────────────────────────────────────────────

func (u *ui) muted() color.Color {
	return theme.Color(theme.ColorNamePlaceHolder)
}

// bigNumber é o número fino e grande dos cartões (Inter Light, como os
// mostradores da Apple).
func bigNumber(text string) *canvas.Text {
	t := canvas.NewText(text, theme.Color(theme.ColorNameForeground))
	t.TextSize = 34
	t.FontSource = fontLight
	return t
}

func caption(text string, c color.Color) *canvas.Text {
	t := canvas.NewText(text, c)
	t.TextSize = 12
	return t
}
