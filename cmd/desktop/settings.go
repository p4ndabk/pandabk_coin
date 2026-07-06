package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"pandabk_coin/internal/node"
	"pandabk_coin/internal/wallet"
)

// confForm são os campos compartilhados entre a tela de primeira vez e a
// aba Ajustes — mesma edição, dois momentos.
type confForm struct {
	peers   *widget.Entry
	listen  *widget.Entry
	rpc     *widget.Entry
	datadir *widget.Entry
	mine    *widget.Check
	miners  *widget.Entry
}

func newConfForm(v confValues) *confForm {
	f := &confForm{
		peers:   widget.NewEntry(),
		listen:  widget.NewEntry(),
		rpc:     widget.NewEntry(),
		datadir: widget.NewEntry(),
		mine:    widget.NewCheck("Minerar (recomendado — 1 core e ~64 MiB)", nil),
		miners:  widget.NewEntry(),
	}
	f.peers.SetPlaceHolder("IP:porta de um node amigo — vazio = iniciar sua própria rede")
	f.peers.SetText(v.Peers)
	f.listen.SetText(v.Listen)
	f.rpc.SetText(v.RPC)
	f.datadir.SetText(v.DataDir)
	f.mine.SetChecked(v.Mine)
	f.miners.SetText(strconv.Itoa(v.Miners))
	return f
}

func (f *confForm) values() (confValues, error) {
	miners, err := strconv.Atoi(strings.TrimSpace(f.miners.Text))
	if err != nil {
		return confValues{}, fmt.Errorf("miners precisa ser um número (ex.: 1)")
	}
	v := confValues{
		Peers:   strings.TrimSpace(f.peers.Text),
		Listen:  strings.TrimSpace(f.listen.Text),
		RPC:     strings.TrimSpace(f.rpc.Text),
		DataDir: strings.TrimSpace(f.datadir.Text),
		Mine:    f.mine.Checked,
		Miners:  miners,
	}
	return v, v.validate()
}

// fields monta o formulário rotulado, na ordem do mais para o menos usado.
func (f *confForm) fields() fyne.CanvasObject {
	return container.NewVBox(
		labeled("CONECTAR A (PEERS)", f.peers),
		f.mine,
		labeled("WORKERS DE MINERAÇÃO", f.miners),
		widget.NewSeparator(),
		labeled("PORTA P2P (VAZIO = SÓ SAÍDA, FUNCIONA ATRÁS DE NAT)", f.listen),
		labeled("RPC LOCAL", f.rpc),
		labeled("PASTA DE DADOS (CHAIN E WALLET)", f.datadir),
	)
}

func labeled(label string, w fyne.CanvasObject) fyne.CanvasObject {
	c := caption(label, theme.Color(theme.ColorNamePlaceHolder))
	return container.NewVBox(c, w)
}

// ── Primeira vez ────────────────────────────────────────────────────────────

// setupScreen é a pré-configuração do primeiro uso: sem panda.conf salvo,
// o app pergunta o essencial ANTES de ligar o node.
func (u *ui) setupScreen() fyne.CanvasObject {
	title := canvas.NewText("Bem-vindo à PANDA", theme.Color(theme.ColorNameForeground))
	title.TextSize = 32
	title.FontSource = fontMedium
	sub := widget.NewLabel("Seu node valida a rede e minera para a sua própria wallet.\nConfigure o básico — dá para mudar depois na aba Ajustes.")
	sub.Wrapping = fyne.TextWrapWord

	form := newConfForm(confFromConfig(u.cfg))

	start := widget.NewButtonWithIcon("Começar a rodar meu node", theme.ConfirmIcon(), func() {
		v, err := form.values()
		if err != nil {
			dialog.ShowError(err, u.win)
			return
		}
		if err := saveConf(u.confPath, v); err != nil {
			dialog.ShowError(fmt.Errorf("salvando %s: %v", u.confPath, err), u.win)
			return
		}
		*u.cfg = v.apply(*u.cfg)
		u.createWalletThenStart()
	})
	start.Importance = widget.HighImportance

	restore := widget.NewButton("Já tenho 12 palavras (recuperar carteira)", func() {
		u.restoreWalletDialog(form)
	})

	note := caption("Wallet nova? O app cria e mostra as 12 palavras de backup antes de começar.", theme.Color(theme.ColorNamePlaceHolder))

	bg := canvas.NewRectangle(theme.Color(theme.ColorNameButton))
	bg.CornerRadius = 14
	card := container.NewStack(bg, container.NewPadded(container.NewPadded(container.NewVBox(
		form.fields(),
		start,
		restore,
		note,
	))))

	return container.NewVScroll(container.NewPadded(container.NewVBox(title, sub, card)))
}

// createWalletThenStart garante o ritual das 12 palavras: se vai minerar e
// ainda não existe wallet, cria AQUI e obriga o dono a ver as palavras
// antes de o node subir. Cancelou = apaga a wallet recém-criada (sem
// fundos ainda) e volta — nunca fica uma carteira sem backup anotado.
func (u *ui) createWalletThenStart() {
	path := u.cfg.WalletPath()
	if _, err := os.Stat(path); err == nil || !u.cfg.Mine {
		u.startAndBuild()
		return
	}
	w, phrase, err := wallet.NewWithMnemonic(path)
	if err != nil {
		dialog.ShowError(err, u.win)
		return
	}
	words := widget.NewLabel(numberedMnemonic(phrase))
	words.TextStyle = fyne.TextStyle{Monospace: true}
	warn := widget.NewLabel("Anote NUM PAPEL, nesta ordem. Elas aparecem UMA única vez e recuperam sua carteira em qualquer máquina. Quem lê as palavras leva os fundos.")
	warn.Wrapping = fyne.TextWrapWord
	addr := caption("endereço: "+w.Address(), theme.Color(theme.ColorNamePlaceHolder))
	content := container.NewVBox(words, addr, warn)

	dialog.NewCustomConfirm("Suas 12 palavras", "Anotei as palavras", "Cancelar", content, func(ok bool) {
		if !ok {
			_ = os.Remove(path) // recém-criada, sem fundos: melhor apagar que ficar sem backup
			return
		}
		u.startAndBuild()
	}, u.win).Show()
}

// restoreWalletDialog recupera a carteira a partir das 12 palavras — o
// caminho de quem trocou de máquina.
func (u *ui) restoreWalletDialog(form *confForm) {
	entry := widget.NewMultiLineEntry()
	entry.SetPlaceHolder("as 12 palavras, separadas por espaço")
	entry.Wrapping = fyne.TextWrapWord
	entry.SetMinRowsVisible(3)
	dialog.NewCustomConfirm("Recuperar carteira", "Recuperar", "Cancelar", entry, func(ok bool) {
		if !ok {
			return
		}
		v, err := form.values()
		if err != nil {
			dialog.ShowError(err, u.win)
			return
		}
		cfg2 := v.apply(*u.cfg)
		w, err := wallet.Restore(cfg2.WalletPath(), entry.Text)
		if err != nil {
			dialog.ShowError(err, u.win)
			return
		}
		dialog.ShowInformation("Carteira recuperada", "endereço: "+w.Address()+"\n\nAgora é só clicar em Começar.", u.win)
	}, u.win).Show()
}

// numberedMnemonic formata as palavras numeradas em 3 colunas.
func numberedMnemonic(phrase string) string {
	words := strings.Fields(phrase)
	var b strings.Builder
	for row := 0; row < 4; row++ {
		for col := 0; col < 3; col++ {
			i := row + col*4
			fmt.Fprintf(&b, "%2d. %-12s", i+1, words[i])
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// ── Aba Ajustes ─────────────────────────────────────────────────────────────

func (u *ui) settingsTab() fyne.CanvasObject {
	form := newConfForm(confFromConfig(u.cfg))

	where := caption("ARQUIVO: "+u.confPath, theme.Color(theme.ColorNamePlaceHolder))

	save := widget.NewButtonWithIcon("Salvar", theme.DocumentSaveIcon(), func() {
		v, err := form.values()
		if err != nil {
			dialog.ShowError(err, u.win)
			return
		}
		if err := saveConf(u.confPath, v); err != nil {
			dialog.ShowError(err, u.win)
			return
		}
		if u.embedded == nil {
			dialog.ShowInformation("Salvo", "Este app é um painel de um node externo — a configuração vale para o próximo node iniciado por aqui.", u.win)
			return
		}
		dialog.NewConfirm("Aplicar agora?", "Salvo. Reiniciar o node embutido com a nova configuração?\n(A chain fecha com segurança e reabre.)", func(ok bool) {
			if ok {
				u.restartEmbedded(v)
			}
		}, u.win).Show()
	})
	save.Importance = widget.HighImportance

	hint := widget.NewLabel("As chaves são as mesmas do panda.conf da linha de comando. O perfil e as regras da rede (tempo por bloco, recompensa) não moram aqui — elas são parte do binário, definidas no build.")
	hint.Wrapping = fyne.TextWrapWord

	return container.NewVScroll(container.NewPadded(container.NewVBox(
		form.fields(),
		container.NewBorder(nil, nil, nil, save),
		where,
		hint,
	)))
}

// restartEmbedded troca o node embutido pela nova configuração: para o
// atual (ordem segura) e sobe outro no mesmo processo.
func (u *ui) restartEmbedded(v confValues) {
	old := u.embedded
	u.embedded = nil // durante a troca, fechar a janela não tenta parar duas vezes
	cfg2 := v.apply(*u.cfg)
	go func() {
		if old != nil {
			_ = old.Stop()
		}
		n, err := node.New(cfg2)
		if err == nil {
			err = n.Start()
		}
		fyne.Do(func() {
			if err != nil {
				dialog.ShowError(fmt.Errorf("o node não voltou: %v\nAjuste a configuração e salve de novo.", err), u.win)
				return
			}
			u.embedded = n
			*u.cfg = cfg2
			u.setRPC(n.RPCAddr())
			dialog.ShowInformation("Pronto", "Node reiniciado com a nova configuração.", u.win)
		})
	}()
}
