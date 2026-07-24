package app

// page_battery.go — battery charge limit plus display/firmware toggles
// (panel overdrive, POST boot sound).

import (
	"fmt"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func (a *App) buildBatteryPage() gtk.Widgetter {
	page := vbox(16, "battery-page")

	// --- Charge limit ---
	battBody := vbox(8, "")
	battRow, battScale, battVal := sliderRow("Charge limit", 20, 100, 5)
	battScale.ConnectValueChanged(func() {
		v := int(battScale.Value())
		battVal.SetLabel(fmt.Sprintf("%d%%", v))
		if a.syncing {
			return
		}
		a.do(func() error { return a.backend.SetBatteryLimit(v) })
	})
	battBody.Append(battRow)
	battBody.Append(caption("Capping charge around 80% greatly extends battery lifespan " +
		"for a machine that mostly runs plugged in."))
	page.Append(card("Battery", battBody))

	// --- Display ---
	odRow, odSwitch := pillToggle("Panel refresh overdrive")
	odSwitch.ConnectStateSet(func(on bool) bool {
		if a.syncing {
			return false
		}
		v := boolToInt(on)
		a.do(func() error { return a.backend.SetPanelOverdrive(v) })
		return false
	})
	dispBody := vbox(6, "")
	dispBody.Append(odRow)
	dispBody.Append(caption("Reduces ghosting on fast motion; may introduce slight overshoot artifacts."))
	page.Append(card("Display", dispBody))

	// --- Firmware ---
	bsRow, bsSwitch := pillToggle("POST boot sound")
	bsSwitch.ConnectStateSet(func(on bool) bool {
		if a.syncing {
			return false
		}
		v := boolToInt(on)
		a.do(func() error { return a.backend.SetBootSound(v) })
		return false
	})
	page.Append(card("Firmware", bsRow))

	// --- Sync ---
	a.register(func(s Snapshot) {
		if s.Battery > 0 {
			battScale.SetValue(float64(s.Battery))
			battVal.SetLabel(fmt.Sprintf("%d%%", s.Battery))
		}
		odSwitch.SetActive(s.PanelOverdrive != 0)
		bsSwitch.SetActive(s.BootSound != 0)
	})

	return page
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
