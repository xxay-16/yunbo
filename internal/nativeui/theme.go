package nativeui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"image/color"
)

type CompactTheme struct{}

func (CompactTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNamePrimary {
		return color.NRGBA{R: 37, G: 99, B: 235, A: 255}
	}
	return theme.DefaultTheme().Color(name, variant)
}
func (CompactTheme) Font(style fyne.TextStyle) fyne.Resource { return theme.DefaultTheme().Font(style) }
func (CompactTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}
func (CompactTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameText:
		return 14
	case theme.SizeNamePadding:
		return 6
	case theme.SizeNameInnerPadding:
		return 4
	}
	return theme.DefaultTheme().Size(name)
}
