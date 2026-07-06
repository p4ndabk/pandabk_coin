package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"pandabk_coin/internal/core"
	"pandabk_coin/internal/node"
	"pandabk_coin/internal/rpcclient"
)

// card monta um cartão clean: número grande em cima, rótulo pequeno em
// cinza embaixo, sobre um retângulo de cantos arredondados. O rótulo também
// fica atualizável (cardSubs) — os cartões de retarget/halving usam.
func (u *ui) card(key, label string) fyne.CanvasObject {
	value := bigNumber("—")
	u.cardValues[key] = value
	sub := caption(strings.ToUpper(label), u.muted())
	u.cardSubs[key] = sub
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameButton))
	bg.CornerRadius = 14
	content := container.NewVBox(value, sub)
	return container.NewStack(bg, container.NewPadded(container.NewPadded(content)))
}

// ── Início ──────────────────────────────────────────────────────────────────

func (u *ui) statusTab() fyne.CanvasObject {
	mode := "conectado ao node em " + u.rpcAddr()
	if u.embedded != nil {
		mode = "node embutido em execução — datadir " + u.cfg.DataDir
	}
	dot := canvas.NewText("●", theme.Color(theme.ColorNamePrimary))
	dot.TextSize = 12
	modeLine := container.NewHBox(dot, caption(mode, u.muted()))

	grid := container.NewGridWithColumns(3,
		u.card("balance", "saldo (panda)"),
		u.card("height", "altura da chain"),
		u.card("difficulty", "dificuldade"),
		u.card("peers", "peers conectados"),
		u.card("hashrate", "hashes por segundo"),
		u.card("mempool", "transações na fila"),
	)
	// segunda fileira: o mempool.space de casa — ritmo, retarget e halving
	statsGrid := container.NewGridWithColumns(3,
		u.card("avgtime", "tempo médio por bloco"),
		u.card("retarget", "próximo retarget"),
		u.card("halving", "próximo halving"),
	)

	u.consensusLine = widget.NewLabel("")
	u.consensusLine.Wrapping = fyne.TextWrapWord

	u.blocksList = widget.NewLabel("…")
	u.blocksList.TextStyle = fyne.TextStyle{Monospace: true}
	u.mempoolList = widget.NewLabel("…")
	u.mempoolList.TextStyle = fyne.TextStyle{Monospace: true}
	u.mempoolList.Wrapping = fyne.TextWrapWord

	title := canvas.NewText("PANDA", theme.Color(theme.ColorNameForeground))
	title.TextSize = 30
	title.FontSource = fontMedium
	ver := caption("versão "+u.version, u.muted())

	return container.NewVScroll(container.NewPadded(container.NewVBox(
		container.NewHBox(title, container.NewCenter(ver)),
		modeLine,
		widget.NewSeparator(),
		grid,
		statsGrid,
		u.consensusLine,
		widget.NewSeparator(),
		caption("ÚLTIMOS BLOCOS", u.muted()),
		u.blocksList,
		widget.NewSeparator(),
		caption("NA FILA — ESPERANDO UM BLOCO", u.muted()),
		u.mempoolList,
	)))
}

func consensusText(info infoResp) string {
	return fmt.Sprintf("perfil %s · 1 bloco a cada %ds · recompensa %s PANDA · próximo halving no bloco %s",
		info.Profile, info.SpacingSecs, info.RewardPanda, formatUint(info.NextHalving))
}

// ── Carteira ────────────────────────────────────────────────────────────────

func (u *ui) walletTab() fyne.CanvasObject {
	u.walletTotal = bigNumber("—")
	u.walletSpend = caption("", u.muted())
	u.walletSpend.TextSize = 14
	u.walletUTXOs = widget.NewLabel("")

	u.walletAddr = widget.NewEntry()
	u.walletAddr.Disable() // só leitura, mas selecionável
	copyBtn := widget.NewButtonWithIcon("Copiar", theme.ContentCopyIcon(), func() {
		u.win.Clipboard().SetContent(u.walletAddr.Text)
	})

	addrLabel := caption("SEU ENDEREÇO — compartilhe para receber", u.muted())

	backup := widget.NewLabel("A chave desta wallet mora em " + u.cfg.WalletPath() +
		". Faça backup do arquivo: quem perde a chave perde os fundos, sem recuperação.")
	backup.Wrapping = fyne.TextWrapWord
	if u.external {
		backup.SetText("Este painel está conectado a um node externo — a wallet e o backup são gerenciados lá.")
	}

	bg := canvas.NewRectangle(theme.Color(theme.ColorNameButton))
	bg.CornerRadius = 14
	walletCard := container.NewStack(bg, container.NewPadded(container.NewPadded(container.NewVBox(
		u.walletTotal,
		u.walletSpend,
		u.walletUTXOs,
		widget.NewSeparator(),
		addrLabel,
		container.NewBorder(nil, nil, nil, copyBtn, u.walletAddr),
	))))

	u.walletActs = widget.NewLabel("…")
	u.walletActs.TextStyle = fyne.TextStyle{Monospace: true}
	u.walletActs.Wrapping = fyne.TextWrapWord

	// controles do extrato: filtro (tudo/transações/mineração) + paginação
	filterNames := map[string]string{"Tudo": "all", "Transações": "tx", "Mineração": "mined"}
	filter := widget.NewSelect([]string{"Tudo", "Transações", "Mineração"}, func(sel string) {
		u.setActsFilter(filterNames[sel])
	})
	filter.Selected = "Tudo" // estado inicial já é "all" — sem disparar callback

	u.actsPrev = widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() { u.turnActsPage(-1) })
	u.actsNext = widget.NewButtonWithIcon("", theme.NavigateNextIcon(), func() { u.turnActsPage(1) })
	u.actsPrev.Disable()
	u.actsNext.Disable()
	u.actsPageLbl = widget.NewLabel("página 1")
	pager := container.NewHBox(filter, layout.NewSpacer(), u.actsPrev, u.actsPageLbl, u.actsNext)

	return container.NewVScroll(container.NewPadded(container.NewVBox(
		walletCard,
		backup,
		widget.NewSeparator(),
		caption("ATIVIDADE — ENTRADAS E SAÍDAS CONFIRMADAS", u.muted()),
		pager,
		u.walletActs,
	)))
}

// formatActivity é o extrato bancário da wallet: sinal, valor, bloco e a
// contraparte de cada movimento confirmado (o que ainda espera minerador
// aparece em "NA FILA", na aba Início).
func formatActivity(entries []activityResp) string {
	if len(entries) == 0 {
		return "nada ainda — minere ou receba PANDA e o extrato aparece aqui"
	}
	var s strings.Builder
	for _, e := range entries {
		sign, who := "+", ""
		switch {
		case e.Coinbase:
			who = "recompensa de mineração ⛏️"
		case e.Direction == "in":
			who = "de " + shortAddr(e.Counterparty)
		default:
			sign = "−"
			who = "para " + shortAddr(e.Counterparty)
			if e.FeePanda != "" {
				who += "  (taxa " + e.FeePanda + ")"
			}
		}
		fmt.Fprintf(&s, "%s%-12s bloco %-6d %s  %s\n",
			sign, e.AmountPanda, e.Height, time.Unix(e.Time, 0).Format("02/01 15:04"), who)
	}
	return strings.TrimRight(s.String(), "\n")
}

func shortAddr(a string) string {
	if len(a) > 12 {
		return a[:12] + "…"
	}
	return a
}

// ── Enviar ──────────────────────────────────────────────────────────────────

func (u *ui) sendTab() fyne.CanvasObject {
	to := widget.NewEntry()
	to.SetPlaceHolder("endereço de destino (P...)")
	amount := widget.NewEntry()
	amount.SetPlaceHolder("valor em PANDA, ex.: 1.5")
	fee := widget.NewEntry()
	fee.SetPlaceHolder("taxa em subunidades/byte (vazio = padrão)")

	send := widget.NewButtonWithIcon("Enviar", theme.MailSendIcon(), func() {
		u.confirmAndSend(to, amount, fee)
	})
	send.Importance = widget.HighImportance

	hint := widget.NewLabel("A transação vai para a fila da rede e confirma quando um minerador a incluir num bloco — normalmente 1–2 blocos.")
	hint.Wrapping = fyne.TextWrapWord

	return container.NewPadded(container.NewVBox(
		caption("PARA", u.muted()), to,
		caption("VALOR", u.muted()), amount,
		caption("TAXA (OPCIONAL)", u.muted()), fee,
		container.NewBorder(nil, nil, nil, send),
		hint,
	))
}

func (u *ui) confirmAndSend(to, amount, fee *widget.Entry) {
	dest := strings.TrimSpace(to.Text)
	if _, err := core.DecodeAddress(dest); err != nil {
		dialog.ShowError(fmt.Errorf("endereço inválido: %v", err), u.win)
		return
	}
	value, err := node.ParseAmount(amount.Text)
	if err != nil || value == 0 {
		dialog.ShowError(fmt.Errorf("valor inválido: use algo como 1.5"), u.win)
		return
	}
	params := map[string]any{"to": dest, "amount": strings.TrimSpace(amount.Text)}
	if f := strings.TrimSpace(fee.Text); f != "" {
		rate, err := strconv.ParseUint(f, 10, 64)
		if err != nil || rate == 0 {
			dialog.ShowError(fmt.Errorf("taxa inválida: número inteiro > 0"), u.win)
			return
		}
		params["fee_rate"] = rate
	}

	summary := fmt.Sprintf("Enviar %s PANDA para\n%s ?", strings.TrimSpace(amount.Text), dest)
	dialog.NewConfirm("Confirmar envio", summary, func(ok bool) {
		if !ok {
			return
		}
		go func() {
			var res map[string]string
			err := rpcclient.Call(u.rpcAddr(), "sendtoaddress", params, &res)
			fyne.Do(func() {
				if err != nil {
					dialog.ShowError(err, u.win)
					return
				}
				to.SetText("")
				amount.SetText("")
				fee.SetText("")
				dialog.ShowInformation("Enviado 📤", "txid "+res["txid"]+"\n\nConfirma quando entrar num bloco.", u.win)
			})
		}()
	}, u.win).Show()
}

// formatRecentBlocks é a régua de blocos do mempool.space, em texto: mais
// novo primeiro, com o intervalo real entre blocos à vista.
func formatRecentBlocks(blocks []recentResp) string {
	if len(blocks) == 0 {
		return "ainda sem blocos"
	}
	var s strings.Builder
	for _, b := range blocks {
		gap := "     "
		if b.Elapsed > 0 {
			gap = fmt.Sprintf("+%s", (time.Duration(b.Elapsed) * time.Second).String())
		}
		miner := b.Miner
		if len(miner) > 12 {
			miner = miner[:12] + "…"
		}
		fmt.Fprintf(&s, "#%-6d %s  %d tx  %-7s %s\n",
			b.Height, time.Unix(b.Time, 0).Format("15:04:05"), b.Txs, gap, miner)
	}
	return strings.TrimRight(s.String(), "\n")
}

// formatMempoolList é a sala de espera: quem está aguardando um minerador.
func formatMempoolList(txs []mempoolResp) string {
	if len(txs) == 0 {
		return "nenhuma — tudo confirmado ✅"
	}
	var s strings.Builder
	for i, tx := range txs {
		if i == 8 {
			fmt.Fprintf(&s, "… e mais %d", len(txs)-8)
			break
		}
		fmt.Fprintf(&s, "%s…  %s PANDA  %d bytes  taxa %s (%.1f/byte)\n",
			tx.TxID[:16], tx.ValuePanda, tx.Size, tx.FeePanda, tx.FeeRate)
	}
	return strings.TrimRight(s.String(), "\n")
}

// ── Blocos (explorador) ─────────────────────────────────────────────────────

type blockResp struct {
	Height        uint64  `json:"height"`
	Hash          string  `json:"hash"`
	Time          int64   `json:"time"`
	Difficulty    float64 `json:"difficulty"`
	Size          int     `json:"size"`
	Confirmations uint64  `json:"confirmations"`
	Txs           []struct {
		TxID     string `json:"txid"`
		Coinbase bool   `json:"coinbase"`
		Ins      []struct {
			TxID  string `json:"txid"`
			Index uint32 `json:"index"`
		} `json:"ins"`
		Outs []struct {
			ValuePanda string `json:"value_panda"`
			Address    string `json:"address"`
		} `json:"outs"`
	} `json:"txs"`
}

func (u *ui) blocksTab() fyne.CanvasObject {
	query := widget.NewEntry()
	query.SetPlaceHolder("altura (ex.: 42) ou hash — vazio = a ponta")

	out := widget.NewLabel("Digite uma altura e veja o bloco por dentro: quem minerou, as transações, valores e destinos.")
	out.TextStyle = fyne.TextStyle{Monospace: true}
	out.Wrapping = fyne.TextWrapWord
	scroll := container.NewVScroll(out)

	show := func() {
		params := map[string]any{}
		if q := strings.TrimSpace(query.Text); q != "" {
			if h, err := strconv.ParseUint(q, 10, 64); err == nil {
				params["height"] = h
			} else {
				params["hash"] = q
			}
		}
		go func() {
			var b blockResp
			err := rpcclient.Call(u.rpcAddr(), "getblock", params, &b)
			fyne.Do(func() {
				if err != nil {
					out.SetText("⚠️ " + err.Error())
					return
				}
				out.SetText(formatBlock(b))
			})
		}()
	}
	query.OnSubmitted = func(string) { show() }
	view := widget.NewButtonWithIcon("Ver", theme.SearchIcon(), show)
	view.Importance = widget.HighImportance

	top := container.NewBorder(nil, nil, nil, view, query)
	return container.NewPadded(container.NewBorder(top, nil, nil, nil, scroll))
}

func formatBlock(b blockResp) string {
	var s strings.Builder
	fmt.Fprintf(&s, "bloco %d — %d confirmação(ões)\n", b.Height, b.Confirmations)
	fmt.Fprintf(&s, "hash  %s\n", b.Hash)
	fmt.Fprintf(&s, "%s · dificuldade %.2f · %d bytes\n",
		time.Unix(b.Time, 0).Format("02/01/2006 15:04:05"), b.Difficulty, b.Size)
	for i, tx := range b.Txs {
		s.WriteString("\n")
		if tx.Coinbase {
			fmt.Fprintf(&s, "tx %d · coinbase (recompensa do bloco) · %s…\n", i+1, tx.TxID[:16])
		} else {
			fmt.Fprintf(&s, "tx %d · %s…\n", i+1, tx.TxID[:16])
			for _, in := range tx.Ins {
				fmt.Fprintf(&s, "   gasta %s…:%d\n", in.TxID[:16], in.Index)
			}
		}
		for _, out := range tx.Outs {
			fmt.Fprintf(&s, "   → %s   %s PANDA\n", out.Address, out.ValuePanda)
		}
	}
	return s.String()
}

// ── Atividade ───────────────────────────────────────────────────────────────

func (u *ui) logsTab() fyne.CanvasObject {
	u.logView = widget.NewLabel("")
	u.logView.TextStyle = fyne.TextStyle{Monospace: true}
	u.logView.Wrapping = fyne.TextWrapWord
	if u.external {
		u.logView.SetText("Painel de node externo: os logs aparecem no terminal onde o node roda.")
	}
	u.logScroll = container.NewVScroll(u.logView)
	return container.NewPadded(u.logScroll)
}

// ── formatação ──────────────────────────────────────────────────────────────

func formatUint(v uint64) string {
	s := strconv.FormatUint(v, 10)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	lead := len(s) % 3
	if lead > 0 {
		b.WriteString(s[:lead])
		if len(s) > lead {
			b.WriteByte('.')
		}
	}
	for i := lead; i < len(s); i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < len(s) {
			b.WriteByte('.')
		}
	}
	return b.String()
}

func formatFloat(v float64) string {
	if v >= 100 {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return strconv.FormatFloat(v, 'f', 2, 64)
}
