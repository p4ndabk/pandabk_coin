package main

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"pandabk_coin/internal/core"
	"pandabk_coin/internal/node"
	"pandabk_coin/internal/rpcclient"
)

// card monta um cartão clean: número grande em cima, rótulo pequeno em
// cinza embaixo, sobre um retângulo de cantos arredondados.
func (u *ui) card(key, label string) fyne.CanvasObject {
	value := bigNumber("—")
	u.cardValues[key] = value
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameButton))
	bg.CornerRadius = 14
	content := container.NewVBox(value, caption(strings.ToUpper(label), u.muted()))
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

	u.consensusLine = widget.NewLabel("")
	u.consensusLine.Wrapping = fyne.TextWrapWord

	title := canvas.NewText("PANDA", theme.Color(theme.ColorNameForeground))
	title.TextSize = 30
	title.FontSource = fontMedium
	ver := caption("versão "+u.version, u.muted())

	return container.NewVScroll(container.NewPadded(container.NewVBox(
		container.NewHBox(title, container.NewCenter(ver)),
		modeLine,
		widget.NewSeparator(),
		grid,
		u.consensusLine,
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

	return container.NewVScroll(container.NewPadded(container.NewVBox(
		walletCard,
		backup,
	)))
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
