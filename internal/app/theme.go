package app

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

const (
	// themeClassic 使用经典面板与卡片外观
	themeClassic = iota
	// themeGlass 使用 Liquid Glass 材质
	themeGlass
)

// glassHeaderHeight 设置玻璃主题整块顶栏的高度
const glassHeaderHeight = 102

// glassPanelRowHeight 设置玻璃主题记录行的高度
const glassPanelRowHeight = 64

// panelRowHeight 返回当前主题的记录行高度
func (a *App) panelRowHeight() float32 {
	if a.settings.Theme == themeGlass {
		return glassPanelRowHeight
	}
	return panelRowHeight
}

// themeBackground 为玻璃主题绘制中性底色并透出系统毛玻璃
func (a *App) themeBackground(c *ui.Context, e ui.Element, p palette) {
	if a.settings.Theme == themeGlass {
		background := p.bg
		if c.Vibrancy() {
			background = background.Alpha(0.72)
		}
		e.Background(background)
	}
}

// themeSurface 按主题绘制普通或带中性染色的玻璃容器
func (a *App) themeSurface(e ui.Element, background ui.Color) ui.Element {
	if a.settings.Theme == themeGlass {
		return e.Material(glass.Glass{Tint: background.Alpha(0.35)})
	}
	return e.Background(background)
}

// themeControl 统一透明主题表单控件的底色、描边与交互反馈
func (a *App) themeControl(c *ui.Context, e ui.Element) ui.Element {
	if a.settings.Theme != themeGlass {
		return e
	}
	t, p := c.Theme(), pal(c)
	e.Background(p.control).Border(1, p.hair)
	if e.Pressed() {
		e.Background(t.SurfacePressed)
	} else if e.Hovered() {
		e.Background(t.SurfaceHover).BorderColor(t.Border)
	}
	if e.Focused() {
		e.BorderColor(t.Focus)
	}
	return e
}

// themeSwitch 使用统一中性底色与启用色绘制轨道并保留中性玻璃滑块
func (a *App) themeSwitch(c *ui.Context, on *bool) ui.Element {
	if a.settings.Theme != themeGlass {
		return ui.Switch(c, on)
	}
	t, p := c.Theme(), pal(c)
	sw := ui.SwitchBase(c, on).Size(t.Space(9), t.Space(5)).Radius(t.Space(2.5))
	target := float32(0)
	if *on {
		target = 1
	}
	pos := sw.Animate("knob", target, 140*time.Millisecond)
	sw.Background(p.control.Mix(t.Accent, pos)).Border(1, p.hair.Mix(t.Accent, pos))
	if sw.Focused() {
		sw.BorderColor(t.Focus)
	}
	if c.Direction() == ui.RightToLeft {
		pos = 1 - pos
	}
	sw.Draw(func(p *ui.Painter, r ui.Rect) {
		width, height := r.H+6, r.H+2
		knob := ui.Rect{X: r.X - 1 + pos*(r.W-width+2), Y: r.Y - 1, W: width, H: height}
		p.Shadow(knob, height/2, 0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.25))
		p.Fill(knob, t.Surface, height/2)
		glass.Paint(p, knob, height/2, glass.Glass{Tint: t.Surface})
	})
	return sw
}

// themeButton 使用统一控件底色绘制普通按钮并保留点击和禁用行为
func (a *App) themeButton(c *ui.Context, label string) ui.Element {
	return a.themeControl(c, ui.Button(c, label))
}

// themePrimaryButton 按主题绘制带强调色的主要操作按钮
func (a *App) themePrimaryButton(c *ui.Context, label string) ui.Element {
	button := ui.PrimaryButton(c, label)
	if a.settings.Theme == themeGlass {
		button.Material(glass.Glass{Tint: c.Theme().Accent, Interactive: true})
	}
	return button
}

// themeTextInput 使用统一控件底色绘制输入框并保留编辑、提交与焦点行为
func (a *App) themeTextInput(c *ui.Context, value *string) ui.Element {
	if a.settings.Theme != themeGlass {
		return ui.TextInput(c, value)
	}
	t := c.Theme()
	return a.themeControl(c, ui.TextInputBase(c, value).Padding(t.Space(1.5), t.Space(2.5)).Radius(t.Radius))
}

// numberInputState 保存数字输入框的编辑文本与已提交数值
type numberInputState struct {
	text  string
	value float64
}

// themeNumberInput 将数字输入与上下步进按钮绘制在同一个边框内并保留范围校验
func (a *App) themeNumberInput(c *ui.Context, value *float64, lo, hi, step float64) ui.Element {
	t := c.Theme()
	background, border, hover := t.Surface, t.Border, t.Border.Mix(t.Text, 0.25)
	if a.settings.Theme == themeGlass {
		p := pal(c)
		background, border = p.control, p.hair
	}
	group := ui.Row(c).AlignItems(ui.Stretch).Width(t.Space(32)).Shrink(0).Role(ui.RoleGroup)
	decimals := 0
	if s := strconv.FormatFloat(step, 'f', -1, 64); strings.Contains(s, ".") {
		decimals = len(s) - strings.IndexByte(s, '.') - 1
	}
	format := func() string { return strconv.FormatFloat(*value, 'f', decimals, 64) }
	state := ui.Local(group, "number-input", func() numberInputState {
		return numberInputState{text: format(), value: *value}
	})
	if state.value != *value {
		state.text = format()
	}
	set := func(v float64) {
		v = math.Max(lo, math.Min(hi, v))
		p := math.Pow10(decimals)
		v = math.Round(v*p) / p
		*value = v
	}
	advance := func(delta float64) {
		set(*value + delta)
		state.text = format()
	}
	group.Children(func() {
		ui.Box(c).FillWidth().MinWidth(0).Children(func() {
			input := ui.TextInputBase(c, &state.text).FillWidth().MinWidth(0).
				Padding(t.Space(1.5), t.Space(6)+1, t.Space(1.5), t.Space(2.5)).Radius(t.Radius).
				Background(background).Border(1, border)
			if a.settings.Theme == themeGlass {
				a.themeControl(c, input)
			} else if input.Focused() {
				input.BorderColor(t.Focus)
			} else if input.Hovered() {
				input.BorderColor(hover)
			}
			if input.Changed() {
				if v, err := strconv.ParseFloat(strings.TrimSpace(state.text), 64); err == nil && v >= lo && v <= hi {
					set(v)
				}
			}
			if input.Shortcut(0, ui.KeyUp) {
				advance(step)
			}
			if input.Shortcut(0, ui.KeyDown) {
				advance(-step)
			}
			if !input.Focused() || input.Submitted() {
				state.text = format()
			}
			input.Range(lo, hi, *value).Step(step)
		})
		ui.Row(c).Absolute().Top(1).Bottom(1).Right(1).Width(t.Space(3.5) + 1).AlignItems(ui.Stretch).Children(func() {
			ui.Box(c).Width(1).Shrink(0).Background(border)
			ui.Column(c).FillWidth().Children(func() {
				button := func(up bool) ui.Element {
					b := ui.ButtonBase(c).FillWidth().Height(0).Grow(1).Padding(0)
					radius := max(float32(0), t.Radius-1)
					if up {
						b.Radius(0, radius, 0, 0).Label("Increase")
					} else {
						b.Radius(0, 0, radius, 0).Label("Decrease")
					}
					b.Disabled((up && *value >= hi) || (!up && *value <= lo))
					if !b.IsDisabled() {
						if b.Pressed() {
							b.Background(t.SurfacePressed)
						} else if b.Hovered() {
							b.Background(t.SurfaceHover)
						}
					}
					color := t.Text
					if b.IsDisabled() {
						color = t.TextMuted
					}
					b.DrawOver(func(p *ui.Painter, r ui.Rect) {
						icon := icoChevron
						if up {
							icon = icoChevronUp
						}
						size := t.Space(2.5)
						p.Icon(icon, ui.Rect{X: r.X + (r.W-size)/2, Y: r.Y + (r.H-size)/2, W: size, H: size}, color)
					})
					return b
				}
				increase := button(true)
				ui.Box(c).FillWidth().Height(1).Background(border)
				decrease := button(false)
				if increase.Clicked() {
					advance(step)
				}
				if decrease.Clicked() {
					advance(-step)
				}
			})
		})
	})
	state.value = *value
	return group
}

// themeSegmented 按主题绘制分段选择与连续移动的玻璃滑块并保留键盘和无障碍行为
func (a *App) themeSegmented(c *ui.Context, selected *int, labels ...string) ui.Element {
	if a.settings.Theme != themeGlass {
		return ui.Segmented(c, selected, labels...)
	}
	t := c.Theme()
	parts := ui.SegmentedBase(c, selected, len(labels))
	parts.Track.AlignItems(ui.Stretch).Padding(t.Space(0.5)).Gap(t.Space(0.5)).Radius(t.Radius).
		Background(pal(c).control).Border(1, pal(c).hair)
	offset, width := float32(0), float32(0)
	for i, label := range labels {
		w, _ := c.MeasureText(0, ui.Span{Text: label})
		w += t.Space(6)
		if i < *selected {
			offset += w + t.Space(0.5)
		} else if i == *selected {
			width = w
		}
	}
	offset = parts.Track.Animate("selection-offset", offset, 140*time.Millisecond)
	width = parts.Track.Animate("selection-width", width, 140*time.Millisecond)
	rtl := c.Direction() == ui.RightToLeft
	parts.Track.Draw(func(p *ui.Painter, r ui.Rect) {
		inset := t.Space(0.5) + 1
		x := r.X + inset + offset
		if rtl {
			x = r.X + r.W - inset - offset - width
		}
		glass.Paint(p, ui.Rect{X: x, Y: r.Y + inset, W: width, H: r.H - inset*2}, t.Radius-t.Space(0.5), glass.Glass{Tint: t.Surface})
	})
	parts.Track.Children(func() {
		for i, label := range labels {
			segment := parts.Segment(i).Padding(t.Space(1), t.Space(3)).Radius(t.Radius - t.Space(0.5))
			if i != *selected && segment.Hovered() {
				segment.Background(t.SurfaceHover)
			}
			segment.Children(func() { ui.Text(c, label).SingleLine() })
		}
	})
	return parts.Track
}

// themeSelect 使用统一控件底色绘制下拉选择并保留选项浮层与键盘行为
func (a *App) themeSelect(c *ui.Context, selected *string, options []string) ui.Element {
	if a.settings.Theme != themeGlass {
		return ui.Select(c, selected, options)
	}
	t := c.Theme()
	parts := ui.SelectBase(c, selected)
	trigger := parts.Trigger.Padding(t.Space(1.5), t.Space(3.5)).Gap(t.Space(1.5)).Radius(t.Radius).
		Justify(ui.SpaceBetween).MinWidth(t.Space(35))
	a.themeControl(c, trigger)
	trigger.Children(func() {
		ui.Text(c, *selected).SingleLine()
		ui.Icon(c, icoChevron).Size(t.Space(2.5), t.Space(2.5)).TextColor(t.TextMuted)
	})
	parts.Popup(func(panel ui.Element) {
		panel.Margin(t.Space(1), 0, 0).Padding(t.Space(1)).Radius(t.Radius+2).Border(1, ui.Transparent).
			Material(glass.Glass{}).Shadow(0, 6, 20, 0, ui.RGBA(0, 0, 0, 0.18))
		ui.Scroll(c).MaxHeight(240).Shrink(1).Children(func() {
			for _, option := range options {
				item := parts.Item(option).Padding(t.Space(1.5), t.Space(2.5)).Radius(t.Radius)
				if item.Highlighted() {
					item.Background(t.Selection)
				}
				item.Children(func() { ui.Text(c, option).SingleLine() })
			}
		})
	})
	return trigger
}
