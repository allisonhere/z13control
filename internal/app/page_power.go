package app

// page_power.go — TDP (PPT power limits) and CPU undervolt. The basic slider
// covers the safe 5–70 W range; an Advanced toggle exposes independent PL1/PL2/
// PL3 sliders up to 93 W with a prominent warning. The undervolt card hides
// itself entirely when ryzen_smu is unavailable.

import (
	"fmt"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func (a *App) buildPowerPage() gtk.Widgetter {
	page := vbox(16, "power-page")

	// --- Basic TDP ---
	basicBody := vbox(10, "")
	basicRow, basicScale, basicVal := sliderRow("Sustained power", tdpMin, tdpBasic, 1)
	basicScale.ConnectValueChanged(func() {
		v := int(basicScale.Value())
		basicVal.SetLabel(fmt.Sprintf("%d W", v))
		if a.syncing {
			return
		}
		a.do(func() error { return a.backend.SetTDP(v, 0, 0, 0, false) })
	})
	basicBody.Append(basicRow)

	advToggleRow, advSwitch := pillToggle("Advanced (independent limits, up to 93 W)")
	basicBody.Append(advToggleRow)

	basicCard := card("Power Limit", basicBody)
	page.Append(basicCard)

	// --- Advanced TDP ---
	advBody := vbox(10, "")
	warn := gtk.NewLabel("Values above 75 W require force mode and may cause thermal " +
		"throttling, instability, or hardware damage. Use at your own risk.")
	warn.AddCSSClass("warning")
	warn.SetWrap(true)
	warn.SetHAlign(gtk.AlignStart)
	advBody.Append(warn)

	pl1Row, pl1Scale, pl1Val := sliderRow("PL1 · Sustained (SPL)", tdpMin, tdpMax, 1)
	pl2Row, pl2Scale, pl2Val := sliderRow("PL2 · Short boost (SPPT)", tdpMin, tdpMax, 1)
	pl3Row, pl3Scale, pl3Val := sliderRow("PL3 · Fast boost (FPPT)", tdpMin, tdpMax, 1)
	advBody.Append(pl1Row)
	advBody.Append(pl2Row)
	advBody.Append(pl3Row)

	applyAdv := func() {
		if a.syncing {
			return
		}
		p1, p2, p3 := int(pl1Scale.Value()), int(pl2Scale.Value()), int(pl3Scale.Value())
		a.do(func() error { return a.backend.SetTDP(0, p1, p2, p3, true) })
	}
	for _, sc := range []*gtk.Scale{pl1Scale, pl2Scale, pl3Scale} {
		sc := sc
		sc.ConnectValueChanged(func() {
			pl1Val.SetLabel(zonedWatts(int(pl1Scale.Value())))
			pl2Val.SetLabel(zonedWatts(int(pl2Scale.Value())))
			pl3Val.SetLabel(zonedWatts(int(pl3Scale.Value())))
			applyAdv()
		})
	}

	resetRow := hbox(10, "action-row")
	resetTdp := gtk.NewButtonWithLabel("Reset TDP to firmware default")
	resetTdp.SetHExpand(true)
	resetTdp.ConnectClicked(func() { a.do(a.backend.ResetTDP) })
	resetRow.Append(resetTdp)
	advBody.Append(resetRow)

	advCard := card("Advanced Power Limits", advBody)
	advCard.SetVisible(false)
	page.Append(advCard)

	advSwitch.ConnectStateSet(func(on bool) bool {
		advCard.SetVisible(on)
		basicRow.SetVisible(!on)
		return false
	})

	// --- Undervolt ---
	uvBody := vbox(10, "")
	uvWarn := gtk.NewLabel("Curve Optimizer offsets are volatile and only active under the " +
		"Custom profile. Unstable values can crash the system under load.")
	uvWarn.AddCSSClass("caption")
	uvWarn.SetWrap(true)
	uvWarn.SetHAlign(gtk.AlignStart)
	uvBody.Append(uvWarn)

	uvRow, uvScale, uvVal := sliderRow("CPU Curve Optimizer", uvMin, uvMax, 1)
	uvScale.ConnectValueChanged(func() {
		co := int(uvScale.Value())
		if co == 0 {
			uvVal.SetLabel("0 (stock)")
		} else {
			uvVal.SetLabel(itoa(co))
		}
		if a.syncing {
			return
		}
		a.do(func() error { return a.backend.SetUndervolt(co) })
	})
	uvBody.Append(uvRow)

	uvReset := gtk.NewButtonWithLabel("Reset to stock")
	uvReset.SetHExpand(true)
	uvReset.ConnectClicked(func() { a.do(a.backend.ResetUndervolt) })
	uvBody.Append(uvReset)

	uvCard := card("CPU Undervolt", uvBody)
	page.Append(uvCard)

	uvUnavailable := caption("Undervolt is unavailable: the ryzen_smu kernel module " +
		"(amkillam fork, required for Strix Halo) is not loaded.")
	page.Append(uvUnavailable)

	// --- Sync ---
	a.register(func(s Snapshot) {
		if s.TDP != nil {
			bv := clampInt(s.TDP.PL1SPL, tdpMin, tdpBasic)
			basicScale.SetValue(float64(bv))
			basicVal.SetLabel(fmt.Sprintf("%d W", s.TDP.PL1SPL))
			pl1Scale.SetValue(float64(s.TDP.PL1SPL))
			pl2Scale.SetValue(float64(s.TDP.PL2SPPT))
			pl3Scale.SetValue(float64(s.TDP.FPPT))
			pl1Val.SetLabel(zonedWatts(s.TDP.PL1SPL))
			pl2Val.SetLabel(zonedWatts(s.TDP.PL2SPPT))
			pl3Val.SetLabel(zonedWatts(s.TDP.FPPT))
		}
		uvCard.SetVisible(s.UndervoltAvailable)
		uvUnavailable.SetVisible(!s.UndervoltAvailable)
		if s.Undervolt != nil {
			uvScale.SetValue(float64(s.Undervolt.CPUCO))
		}
	})

	return page
}

// zonedWatts annotates values that cross the forced-power threshold.
func zonedWatts(w int) string {
	if w > tdpSafeMax {
		return fmt.Sprintf("%d W  ⚠", w)
	}
	return fmt.Sprintf("%d W", w)
}
