package tokenview

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// priceForm is a price being added or changed: the user's own, over
// enowX's.
type priceForm struct {
	open                                      bool
	index                                     int // of the user's prices, -1 a new one
	provider, model, config                   string
	input, cacheRead, cacheWrite, output, err string
}

// prices lists the price table: the user's prices first, over enowX's.
func (v *View) prices(c *ui.Context, d Data, o Options) {
	ui.Row(c).FillWidth().Gap(10).Children(func() {
		ui.Text(c, "US dollars a million tokens. Your prices go over enowX's; a model API config's own (OpenRouter's, a local model's 0) by its config.").
			FontSize(12).TextColor(theme.Muted).Grow(1).Shrink(1)
		if widget.PrimaryButton(c, icon.Plus, "Add price", "").Clicked() {
			v.price = priceForm{open: true, index: -1}
		}
	})
	widget.Card(c).Children(func() {
		ui.Row(c).FillWidth().Padding(4, 10).Gap(10).Children(func() {
			ui.Text(c, "Model").FontSize(11).TextColor(theme.Muted).Grow(1)
			for _, h := range []string{"Input", "Cache read", "Cache write", "Output", ""} {
				ui.Text(c, h).FontSize(11).TextColor(theme.Muted).Width(74).TextAlign(ui.End)
			}
		})
		own := 0
		for i, p := range d.Prices {
			key := fmt.Sprintf("p%d", i)
			ui.Row(c.Key(key)).FillWidth().Padding(6, 10).Gap(10).Children(func() {
				ui.Row(c).Grow(1).MinWidth(0).Gap(8).Children(func() {
					ui.Text(c, p.Model).Font(theme.MonoFont).FontSize(11.5).SingleLine().Shrink(1)
					note := p.Provider
					if p.Config != "" {
						note = "config " + p.Config
					}
					if !p.Shipped {
						note = strings.TrimSpace("yours " + note)
					}
					ui.Text(c, note).FontSize(11).TextColor(theme.Muted).SingleLine().Shrink(1)
				})
				for _, x := range []float64{p.Input, p.CacheRead, p.CacheWrite, p.Output} {
					ui.Text(c, money(x)).FontSize(12).Width(74).TextAlign(ui.End).TextColor(theme.TextSoft).SingleLine()
				}
				ui.Row(c).Width(74).Justify(ui.End).Children(func() {
					if p.Shipped {
						if widget.IconButton(c, icon.Pencil, "Set your own price for "+p.Model, false).Clicked() {
							v.price = formOfPrice(p, -1)
						}
						return
					}
					idx := own
					if widget.IconButton(c, icon.Pencil, "Change", false).Clicked() {
						v.price = formOfPrice(p, idx)
					}
					if widget.IconButton(c, icon.Trash, "Remove", false).Clicked() && o.Confirm != nil {
						o.Confirm("Remove your price for "+p.Model+"?", "enowX's own price, if it has one, counts again.", "Remove", func() {
							st := d.Settings
							st.Prices = slices.Delete(slices.Clone(st.Prices), idx, idx+1)
							o.Save(st)
						})
					}
				})
			})
			if !p.Shipped {
				own++
			}
		}
	})
}

func money(x float64) string { return "$" + strconv.FormatFloat(x, 'f', -1, 64) }

func formOfPrice(p proto.TokenPrice, index int) priceForm {
	f := func(x float64) string { return strconv.FormatFloat(x, 'f', -1, 64) }
	return priceForm{open: true, index: index, provider: p.Provider, model: p.Model, config: p.Config,
		input: f(p.Input), cacheRead: f(p.CacheRead), cacheWrite: f(p.CacheWrite), output: f(p.Output)}
}

// priceSheet adds or changes one of the user's prices.
func (v *View) priceSheet(c *ui.Context, d Data, o Options) {
	f := &v.price
	if !f.open {
		return
	}
	widget.FormSheet(c, &f.open, 540, "A price of yours", "A model's ID, or a prefix ending in * (claude-sonnet-*). A config's price counts only for that config.", func() {
		widget.Field(c, "Model", func() { widget.Input(c.Key("model"), &f.model, "claude-sonnet-5-5").FillWidth() })
		ui.Row(c).FillWidth().Gap(10).Children(func() {
			ui.Column(c).Grow(1).Children(func() {
				widget.Field(c, "Provider (optional)", func() { widget.Input(c.Key("provider"), &f.provider, "anthropic").FillWidth() })
			})
			ui.Column(c).Grow(1).Children(func() {
				widget.Field(c, "Config (optional)", func() { widget.Input(c.Key("config"), &f.config, "work-openrouter").FillWidth() })
			})
		})
		ui.Row(c).FillWidth().Gap(10).Children(func() {
			for _, fld := range []struct {
				label string
				v     *string
			}{{"Input", &f.input}, {"Cache read", &f.cacheRead}, {"Cache write", &f.cacheWrite}, {"Output", &f.output}} {
				ui.Column(c.Key(fld.label)).Grow(1).Children(func() {
					widget.Field(c, fld.label, func() { widget.Input(c, fld.v, "0").FillWidth() })
				})
			}
		})
		if f.err != "" {
			ui.Text(c, f.err).FontSize(12).TextColor(theme.Error)
		}
	}, func() {
		if widget.PrimaryButton(c, nil, "Save", "").Clicked() {
			p, err := f.priceOf()
			if err != nil {
				f.err = err.Error()
				return
			}
			st := d.Settings
			list := slices.Clone(st.Prices)
			if f.index >= 0 && f.index < len(list) {
				list[f.index] = p
			} else {
				list = append(list, p)
			}
			st.Prices = list
			o.Save(st)
			f.open = false
		}
	})
}

func (f *priceForm) priceOf() (proto.TokenPrice, error) {
	p := proto.TokenPrice{Provider: strings.TrimSpace(f.provider), Model: strings.TrimSpace(f.model), Config: strings.TrimSpace(f.config)}
	if p.Model == "" {
		return p, fmt.Errorf("name the model, or a prefix ending in *")
	}
	for _, x := range []struct {
		s   string
		dst *float64
	}{{f.input, &p.Input}, {f.cacheRead, &p.CacheRead}, {f.cacheWrite, &p.CacheWrite}, {f.output, &p.Output}} {
		s := strings.TrimSpace(strings.TrimPrefix(x.s, "$"))
		if s == "" {
			continue
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil || v < 0 {
			return p, fmt.Errorf("prices are dollars a million tokens, 0 or more")
		}
		*x.dst = v
	}
	return p, nil
}
