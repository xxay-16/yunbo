package nativeui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type compactProgress struct {
	widget.BaseWidget
	value float64
}

func newCompactProgress() *compactProgress        { p := &compactProgress{}; p.ExtendBaseWidget(p); return p }
func (p *compactProgress) SetValue(value float64) { p.value = max(0, min(1, value)); p.Refresh() }
func (p *compactProgress) CreateRenderer() fyne.WidgetRenderer {
	return &progressRenderer{owner: p, background: canvas.NewRectangle(theme.DisabledColor()), fill: canvas.NewRectangle(theme.PrimaryColor())}
}

type progressRenderer struct {
	owner            *compactProgress
	background, fill *canvas.Rectangle
}

func (r *progressRenderer) Layout(size fyne.Size) {
	r.background.Resize(size)
	r.fill.Resize(fyne.NewSize(size.Width*float32(r.owner.value), size.Height))
}
func (r *progressRenderer) MinSize() fyne.Size { return fyne.NewSize(60, 5) }
func (r *progressRenderer) Refresh() {
	r.background.FillColor = theme.DisabledColor()
	r.fill.FillColor = theme.PrimaryColor()
	r.Layout(r.owner.Size())
	r.background.Refresh()
	r.fill.Refresh()
}
func (r *progressRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.background, r.fill}
}
func (r *progressRenderer) Destroy() {}
