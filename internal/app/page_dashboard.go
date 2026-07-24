package app

// page_dashboard.go — the landing page: live telemetry gauges, big profile
// selector cards, and an at-a-glance status strip.

import (
	"fmt"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func (a *App) buildDashboard() gtk.Widgetter {
	page := vbox(16, "dashboard")

	// --- Telemetry gauges ---
	a.tempGauge = newRingGauge("APU TEMP", "°C", 30, 100, [3]float64{1.0, 0.30, 0.36})
	a.fanGauge = newRingGauge("FAN SPEED", "", 0, 6500, [3]float64{0.0, 0.85, 1.0})

	gauges := hbox(16, "gauge-row")
	gauges.Append(card("", a.tempGauge.area))
	gauges.Append(card("", a.fanGauge.area))
	page.Append(gauges)

	// --- Profile cards ---
	page.Append(sectionHeading("PERFORMANCE PROFILE"))
	grid := gtk.NewFlowBox()
	grid.AddCSSClass("profile-grid")
	grid.SetSelectionMode(gtk.SelectionNone)
	grid.SetColumnSpacing(12)
	grid.SetRowSpacing(12)
	grid.SetMaxChildrenPerLine(4)
	grid.SetMinChildrenPerLine(2)
	grid.SetHomogeneous(true)

	profButtons := map[string]*gtk.Button{}
	for _, p := range profiles {
		p := p
		btn := gtk.NewButton()
		btn.AddCSSClass("profile-card")

		inner := vbox(6, "")
		icon := gtk.NewImageFromIconName(p.Icon)
		icon.AddCSSClass("profile-icon")
		icon.SetPixelSize(30)
		name := gtk.NewLabel(p.Name)
		name.AddCSSClass("profile-name")
		blurb := gtk.NewLabel(p.Blurb)
		blurb.AddCSSClass("profile-blurb")
		blurb.SetWrap(true)
		inner.Append(icon)
		inner.Append(name)
		inner.Append(blurb)
		btn.SetChild(inner)

		btn.ConnectClicked(func() {
			if a.syncing {
				return
			}
			if p.ID == "custom" {
				a.navigate("fans")
				return
			}
			a.do(func() error { return a.backend.SetProfile(p.ID) })
		})
		profButtons[p.ID] = btn
		grid.Append(btn)
	}
	page.Append(grid)

	// --- Status strip ---
	page.Append(sectionHeading("STATUS"))
	strip := hbox(12, "status-strip")
	profStat := newStatTile("Profile", "—")
	powStat := newStatTile("Sustained TDP", "—")
	battStat := newStatTile("Charge Limit", "—")
	odStat := newStatTile("Panel Overdrive", "—")
	strip.Append(profStat.box)
	strip.Append(powStat.box)
	strip.Append(battStat.box)
	strip.Append(odStat.box)
	page.Append(strip)

	// --- Sync ---
	a.register(func(s Snapshot) {
		for id, btn := range profButtons {
			if id == s.Profile {
				btn.AddCSSClass("selected")
			} else {
				btn.RemoveCSSClass("selected")
			}
		}
		profStat.set(titleCase(s.Profile))
		if s.TDP != nil {
			powStat.set(fmt.Sprintf("%d W", s.TDP.PL1SPL))
		}
		battStat.set(fmt.Sprintf("%d%%", s.Battery))
		odStat.set(onOff(s.PanelOverdrive))
	})

	return page
}

// statTile is a compact metric card in the status strip.
type statTile struct {
	box *gtk.Box
	val *gtk.Label
}

func newStatTile(label, value string) *statTile {
	box := vbox(2, "stat-tile")
	box.SetHExpand(true)
	v := gtk.NewLabel(value)
	v.AddCSSClass("stat-value")
	v.SetHAlign(gtk.AlignStart)
	l := gtk.NewLabel(label)
	l.AddCSSClass("stat-label")
	l.SetHAlign(gtk.AlignStart)
	box.Append(v)
	box.Append(l)
	return &statTile{box: box, val: v}
}

func (s *statTile) set(v string) { s.val.SetLabel(v) }

// sectionHeading is an uppercase section divider label.
func sectionHeading(text string) *gtk.Label {
	l := gtk.NewLabel(text)
	l.AddCSSClass("section-heading")
	l.SetHAlign(gtk.AlignStart)
	return l
}

func onOff(v int) string {
	if v != 0 {
		return "On"
	}
	return "Off"
}

func titleCase(s string) string {
	if s == "" {
		return "—"
	}
	return string(s[0]-32) + s[1:]
}
