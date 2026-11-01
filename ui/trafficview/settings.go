package trafficview

import (
	"fmt"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// settingsState is the interceptor's settings as read, and as typed.
type settingsState struct {
	loaded, loading, saving bool
	open                    bool
	st                      proto.TrafficSettings
	insecure, upstream      string
	maxEx, maxMB            string
	hosts, certs            string
	err                     string
}

func (v *View) loadSettings(o Options) {
	s := &v.settings
	if s.loaded || s.loading || o.Host == nil {
		return
	}
	s.loading = true
	h := o.Host
	go func() {
		st, err := h.TrafficSettings()
		o.Update(func() {
			s.loading, s.loaded = false, true
			if err != nil {
				s.err = client.Explain(err)
				return
			}
			s.set(st)
		})
	}()
}

func (s *settingsState) set(st proto.TrafficSettings) {
	s.st, s.err = st, ""
	s.insecure, s.upstream = strings.Join(st.Insecure, ", "), st.Upstream
	s.maxEx, s.maxMB = numText(st.MaxExchanges), numText(st.MaxMB)
	var hosts, certs []string
	for k, v := range st.Hosts {
		hosts = append(hosts, k+" = "+v)
	}
	sort.Strings(hosts)
	for _, c := range st.ClientCerts {
		certs = append(certs, c.Host+" "+c.Cert+" "+c.Key)
	}
	s.hosts, s.certs = strings.Join(hosts, "\n"), strings.Join(certs, "\n")
}

// parseHosts reads "name = address" lines.
func parseHosts(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}

// parseCerts reads "host cert.pem key.pem" lines.
func parseCerts(text string) ([]proto.TrafficClientCert, error) {
	var out []proto.TrafficClientCert
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if len(f) != 3 {
			return nil, fmt.Errorf("a client certificate is: host cert.pem key.pem (%q)", line)
		}
		out = append(out, proto.TrafficClientCert{Host: f[0], Cert: f[1], Key: f[2]})
	}
	return out, nil
}

// NotifiesErrors reports whether the user asked to be told of failed
// requests.
func (v *View) NotifiesErrors() bool { return v.settings.st.NotifyErrors }

// settingsSheet is the interceptor's settings, as a dialog over the tool.
func (v *View) settingsSheet(c *ui.Context, o Options) {
	s := &v.settings
	if !s.open {
		return
	}
	v.loadSettings(o)
	desc := "The interceptor on " + o.HostName + ", for every session there."
	sheet(c, &s.open, 560, "Traffic settings", desc, func() {
		ui.Column(c).FillWidth().Gap(12).Children(func() {
			widget.Field(c, "Don't check the certificates of", func() {
				ui.Column(c).FillWidth().Gap(4).Children(func() {
					widget.Input(c, &s.insecure, "*.test, staging.internal").FillWidth()
					ui.Text(c, "Development servers that sign their own. Loopback (localhost, 127.0.0.1) is never checked.").FontSize(theme.TextLabel).TextColor(theme.Muted)
				})
			})
			widget.Field(c, "Send through a proxy", func() {
				widget.Input(c, &s.upstream, "http://proxy.company:3128 (none by default)").FillWidth()
			})
			ui.Row(c).FillWidth().Gap(10).Children(func() {
				ui.Column(c).Grow(1).Children(func() {
					widget.Field(c, "Keep at most (exchanges)", func() { widget.Input(c, &s.maxEx, "2000").FillWidth() })
				})
				ui.Column(c).Grow(1).Children(func() {
					widget.Field(c, "Bodies at most (MB)", func() { widget.Input(c, &s.maxMB, "256").FillWidth() })
				})
			})
			area := func(label, hint string, value *string) {
				ui.Column(c).FillWidth().Gap(4).Children(func() {
					codeArea(c, label, value, 44)
					ui.Text(c, hint).FontSize(theme.TextLabel).TextColor(theme.Muted)
				})
			}
			area("Point names elsewhere", "name = address per line, as a hosts file: api.example.com = 127.0.0.1:8080", &s.hosts)
			area("Client certificates", "host cert.pem key.pem per line, files on "+o.HostName+", for servers that ask for one", &s.certs)
			check(c, &s.st.NotifyErrors, "Tell me when an app's request fails or is answered 5xx")
		})
		v.capturePrefsFields(c)
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			ui.Text(c, "Certificate authority").FontSize(theme.TextUI).FontWeight(600)
			if v.status.Fingerprint != "" {
				ui.Text(c, "SHA-256 "+v.status.Fingerprint).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.Muted).Selectable()
			}
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Making it anew means attaching again what trusted the old one.").FontSize(theme.TextLabel).TextColor(theme.Muted).Grow(1).Shrink(1)
				if widget.Button(c, widget.Secondary, nil, "Make the CA anew", "").Clicked() {
					renew := func() {
						h := o.Host
						go func() {
							ca, err := h.TrafficNewCA()
							o.Update(func() {
								if err != nil {
									s.err = client.Explain(err)
									return
								}
								v.status.Fingerprint = ca.Fingerprint
								v.inter.ca = nil
								v.notice = "A new CA: browsers, devices and containers attached before must be attached again to trust it."
							})
						}()
					}
					if o.Confirm != nil {
						o.Confirm("Make the CA anew?", "The old certificate authority goes: whatever trusts it, a system, a browser, a phone, stops trusting what enowX catches until it's attached again.", "Make it anew", renew)
					} else {
						renew()
					}
				}
			})
		})
		errorText(c, s.err)
	}, func() {
		label := "Save"
		if s.saving {
			label = "Saving…"
		}
		if widget.PrimaryButton(c, icon.Check, label, "").Disabled(s.saving).Clicked() {
			v.saveSettings(o)
		}
	})
}

func (v *View) saveSettings(o Options) {
	s := &v.settings
	st := s.st
	st.Insecure = splitList(s.insecure)
	st.Upstream = strings.TrimSpace(s.upstream)
	var err error
	if st.MaxExchanges, err = parseNum(s.maxEx, "Exchanges"); err != nil {
		s.err = err.Error()
		return
	}
	if st.MaxMB, err = parseNum(s.maxMB, "MB"); err != nil {
		s.err = err.Error()
		return
	}
	if err := v.saveCapturePrefs(o); err != nil {
		s.err = err.Error()
		return
	}
	st.Hosts = parseHosts(s.hosts)
	if st.ClientCerts, err = parseCerts(s.certs); err != nil {
		s.err = err.Error()
		return
	}
	s.saving, s.err = true, ""
	h := o.Host
	go func() {
		saved, err := h.SetTrafficSettings(st)
		o.Update(func() {
			s.saving = false
			if err != nil {
				s.err = client.Explain(err)
				return
			}
			s.set(saved)
			s.open = false
			v.notice = "Settings saved."
		})
	}()
}
