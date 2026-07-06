package main

import (
	_ "embed"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Tema "Apple-clean": fundo quase-branco (ou quase-preto no dark), texto
// grafite, UM acento só (o azul do iOS), padding generoso e a fonte Inter
// (clone open-source da SF Pro; licença OFL em assets/). O objetivo é o app
// parecer uma carteira, não um painel de servidor.

//go:embed assets/InterDisplay-Regular.ttf
var interRegular []byte

//go:embed assets/InterDisplay-Medium.ttf
var interMedium []byte

//go:embed assets/InterDisplay-Light.ttf
var interLight []byte

var (
	fontRegular = fyne.NewStaticResource("InterDisplay-Regular.ttf", interRegular)
	fontMedium  = fyne.NewStaticResource("InterDisplay-Medium.ttf", interMedium)
	fontLight   = fyne.NewStaticResource("InterDisplay-Light.ttf", interLight)
)

type pandaTheme struct{}

var _ fyne.Theme = pandaTheme{}

func (pandaTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	dark := variant == theme.VariantDark
	switch name {
	case theme.ColorNameBackground:
		if dark {
			return color.NRGBA{R: 0x1c, G: 0x1c, B: 0x1e, A: 0xff} // systemBackground dark
		}
		return color.NRGBA{R: 0xfa, G: 0xfa, B: 0xfa, A: 0xff}
	case theme.ColorNameForeground:
		if dark {
			return color.NRGBA{R: 0xf5, G: 0xf5, B: 0xf7, A: 0xff}
		}
		return color.NRGBA{R: 0x1d, G: 0x1d, B: 0x1f, A: 0xff} // grafite Apple
	case theme.ColorNamePrimary, theme.ColorNameHyperlink, theme.ColorNameFocus:
		if dark {
			return color.NRGBA{R: 0x0a, G: 0x84, B: 0xff, A: 0xff} // azul iOS dark
		}
		return color.NRGBA{R: 0x00, G: 0x7a, B: 0xff, A: 0xff} // azul iOS
	case theme.ColorNameButton, theme.ColorNameInputBackground:
		if dark {
			return color.NRGBA{R: 0x2c, G: 0x2c, B: 0x2e, A: 0xff} // secondary dark
		}
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff} // cartões brancos
	case theme.ColorNameSeparator, theme.ColorNameInputBorder:
		if dark {
			return color.NRGBA{R: 0x3a, G: 0x3a, B: 0x3c, A: 0xff}
		}
		return color.NRGBA{R: 0xe5, G: 0xe5, B: 0xea, A: 0xff}
	case theme.ColorNamePlaceHolder, theme.ColorNameDisabled:
		if dark {
			return color.NRGBA{R: 0x8e, G: 0x8e, B: 0x93, A: 0xff} // systemGray
		}
		return color.NRGBA{R: 0x8e, G: 0x8e, B: 0x93, A: 0xff}
	case theme.ColorNameShadow:
		return color.NRGBA{A: 0x11} // sombra sutil, quase nada
	}
	return theme.DefaultTheme().Color(name, variant)
}

func (pandaTheme) Font(style fyne.TextStyle) fyne.Resource {
	if style.Monospace {
		return theme.DefaultTheme().Font(style) // logs ficam na mono padrão
	}
	if style.Bold {
		return fontMedium // "bold" da Apple é um Medium contido, não um Black
	}
	return fontRegular
}

func (pandaTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (pandaTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 8 // 2× o default: respiro é o que faz parecer Apple
	case theme.SizeNameInnerPadding:
		return 12
	case theme.SizeNameText:
		return 14
	case theme.SizeNameHeadingText:
		return 28
	case theme.SizeNameSubHeadingText:
		return 17
	case theme.SizeNameCaptionText:
		return 12
	case theme.SizeNameLineSpacing:
		return 6
	}
	return theme.DefaultTheme().Size(name)
}
