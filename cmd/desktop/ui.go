package main

import (
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

	"pandabk_coin/internal/node"
	"pandabk_coin/internal/rpcclient"
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
	consensusLine *widget.Label
	walletAddr    *widget.Entry
	walletTotal   *canvas.Text
	walletSpend   *canvas.Text
	walletUTXOs   *widget.Label
	logView       *widget.Label
	logScroll     *container.Scroll
}

// infoResp espelha o getinfo do node (internal/node/rpc.go).
type infoResp struct {
	Profile     string  `json:"profile"`
	Height      uint64  `json:"height"`
	Difficulty  float64 `json:"difficulty"`
	SpacingSecs int64   `json:"target_spacing_seconds"`
	RewardPanda string  `json:"reward_panda"`
	NextHalving uint64  `json:"next_halving"`
	Peers       int     `json:"peers"`
	Mempool     int     `json:"mempool"`
	Mining      bool    `json:"mining"`
	HashRate    float64 `json:"hashrate"`
	Address     string  `json:"address"`
}

type balanceResp struct {
	Address        string `json:"address"`
	BalancePanda   string `json:"balance_panda"`
	SpendablePanda string `json:"spendable_panda"`
	UTXOs          int    `json:"utxos"`
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

	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Início", theme.HomeIcon(), u.statusTab()),
		container.NewTabItemWithIcon("Carteira", theme.AccountIcon(), u.walletTab()),
		container.NewTabItemWithIcon("Enviar", theme.MailSendIcon(), u.sendTab()),
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
			u.setCard("balance", bal.BalancePanda)
			u.walletTotal.Text = bal.BalancePanda + " PANDA"
			u.walletSpend.Text = bal.SpendablePanda + " PANDA gastáveis agora"
			u.walletTotal.Refresh()
			u.walletSpend.Refresh()
			u.walletUTXOs.SetText(formatUint(uint64(bal.UTXOs)) + " UTXOs — moedas de mineração maturam em 10 blocos")
		}
		if info.Address != "" && u.walletAddr.Text != info.Address {
			u.walletAddr.SetText(info.Address)
		}
	})
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
