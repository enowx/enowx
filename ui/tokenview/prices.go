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
	ui.Row(c).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
			widget.CardNote(c, "US dollars a million tokens. Your prices go over enowX's; a model API config's own by its config.", 0)
		})
		if widget.PrimaryButton(c, icon.Plus, "Add price", "").Clicked() {
			v.price = priceForm{open: true, index: -1}
		}
	})
	v.cards(c, len(d.Prices), priceCardHeight, func() {
		own := 0
		for i, p := range d.Prices {
			index := -1
			if !p.Shipped {
				index = own
				own++
			}
			v.priceCard(c.Key(fmt.Sprintf("p%d", i)), p, index, d, o)
		}
	})
}

// priceCardHeight is the least a price's card takes: all it holds.
const priceCardHeight = 144

// priceCard is a model's price: enowX's, which can be set over, or one of the
// user's, at index of theirs, to change or remove.
func (v *View) priceCard(c *ui.Context, p proto.TokenPrice, index int, d Data, o Options) {
	widget.GridCard(c).Children(func() {
		widget.CardTitle(c, nil, p.Model, nil)
		widget.CardStats(c, []widget.Stat{
			{Value: money(p.Input), Label: "input"},
			{Value: money(p.CacheRead), Label: "cache read"},
			{Value: money(p.CacheWrite), Label: "cache write"},
			{Value: money(p.Output), Label: "output"},
		})
		ui.Spacer(c) // the foot stays at the card's bottom, however tall the row
		widget.CardFoot(c, func() {
			note := "enowX's"
			if !p.Shipped {
				note = "Yours"
			}
			if p.Config != "" {
				note += ", of " + p.Config
			} else if p.Provider != "" {
				note += ", " + p.Provider
			}
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, note, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
			if p.Shipped {
				if widget.GhostButton(c, "Set your own").Label("Set your own price for " + p.Model).Clicked() {
					v.price = formOfPrice(p, -1)
				}
				return
			}
			if widget.GhostButton(c, "Change").Clicked() {
				v.price = formOfPrice(p, index)
			}
			if widget.IconButton(c, icon.Trash, "Remove", false).Clicked() && o.Confirm != nil {
				o.Confirm("Remove your price for "+p.Model+"?", "enowX's own price, if it has one, counts again.", "Remove", func() {
					st := d.Settings
					st.Prices = slices.Delete(slices.Clone(st.Prices), index, index+1)
					o.Save(st)
				})
			}
		})
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
	widget.FormSheet(c, &f.open, 480, "A price of yours", "A model's ID, or a prefix ending in * (claude-sonnet-*). A config's price counts only for that config.", func() {
		widget.Field(c, "Model", func() { widget.Input(c.Key("model"), &f.model, "claude-sonnet-*").FillWidth().Font(theme.MonoFont) })
		fieldRow(c, "whose",
			func() {
				widget.Field(c, "Provider (optional)", func() { widget.Input(c.Key("provider"), &f.provider, "anthropic").FillWidth() })
			},
			func() {
				widget.Field(c, "Config (optional)", func() { widget.Input(c.Key("config"), &f.config, "work-openrouter").FillWidth() })
			},
		)
		var amounts []func()
		for _, fld := range []struct {
			label string
			v     *string
		}{{"Input", &f.input}, {"Cache read", &f.cacheRead}, {"Cache write", &f.cacheWrite}, {"Output", &f.output}} {
			amounts = append(amounts, func() {
				widget.Field(c, fld.label, func() { widget.Input(c, fld.v, "0.00").FillWidth() })
			})
		}
		fieldRow(c, "amounts", amounts...)
		if f.err != "" {
			widget.CardNote(c, f.err, 0).TextColor(theme.Error)
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
