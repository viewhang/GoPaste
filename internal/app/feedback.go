package app

import (
	"github.com/egoist/mygo/ui"

	"gopaste/internal/locales"
)

// hintDetails 显示可滚动和选择的完整操作反馈及文件夹授权入口
func (a *App) hintDetails(c *ui.Context, p palette) {
	w, h := c.Size()
	ui.DialogBase(c, &a.hintOpen, func(back, panel ui.Element) {
		back.Background(ui.RGBA(0, 0, 0, 0.12))
		panel.Width(min(480, w-24)).MaxHeight(h-16).Padding(16).Gap(12).Radius(12).
			Background(p.surface).Border(1, p.border).Shadow(0, 12, 34, 0, p.shadow)
		a.themeSurface(panel, p.surface)
		if a.settings.Theme == themeGlass {
			panel.BorderColor(ui.Transparent)
		}
		ui.Text(c, locales.OperationDetails()).FontSize(13).Bold().TextColor(p.text)
		ui.Scroll(c).Shrink(1).MaxHeight(h - 126).Children(func() {
			ui.Text(c, a.hintFlash).Selectable().FontSize(12).TextColor(p.text)
		})
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if a.themeButton(c, locales.Done()).Disabled(a.choosingPasteFolder).Clicked() {
				a.hintOpen = false
			}
			if a.fileAuthorization != nil && ui.PrimaryButton(c, locales.AuthorizeFolder()).Disabled(a.choosingPasteFolder).Clicked() {
				a.authorizePasteFolder()
			}
		})
	})
}
