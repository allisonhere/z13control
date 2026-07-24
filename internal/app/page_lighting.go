package app

// page_lighting.go — Aura RGB control, including daemon-managed custom
// palette cycles. Changes apply live to the selected zone.

import (
	"fmt"
	"strconv"

	"github.com/dahui/z13ctl/api"
	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const paletteCapability = "palette-cycle"

var defaultPalette = []string{"FF0066", "00E5FF", "7C4DFF"}

type lightingState struct {
	device     string // "", "keyboard", "lightbar"
	mode       string
	color      string
	palette    []string
	speed      string
	brightness int
	enabled    bool
}

func (a *App) buildLightingPage() gtk.Widgetter {
	page := vbox(16, "lighting-page")
	ls := &lightingState{
		mode: "static", color: "00E5FF", palette: append([]string(nil), defaultPalette...),
		speed: defaultLightingSpeed, brightness: 3, enabled: true,
	}
	var latest Snapshot
	var syncControls func(Snapshot)
	var rebuildStops func()
	var colorCard, paletteCard *gtk.Box

	apply := func() {
		if a.syncing || !ls.enabled {
			return
		}
		st := *ls
		a.do(func() error {
			if st.mode == "palette" {
				return a.backend.ApplyPalette(st.device, st.palette, st.speed, st.brightness)
			}
			return a.backend.ApplyLighting(st.device, st.color, st.color, st.mode, st.speed, st.brightness)
		})
	}

	// --- Zone selector ---
	zoneBody := hbox(8, "segmented")
	zones := []struct{ id, name string }{{"", "All Zones"}, {"keyboard", "Keyboard"}, {"lightbar", "Lightbar"}}
	zoneBtns := map[string]*gtk.Button{}
	for _, z := range zones {
		z := z
		b := gtk.NewButtonWithLabel(z.name)
		b.AddCSSClass("seg-btn")
		b.SetHExpand(true)
		if z.id == ls.device {
			b.AddCSSClass("selected")
		}
		b.ConnectClicked(func() {
			for id, bb := range zoneBtns {
				toggleClass(bb, "selected", id == z.id)
			}
			ls.device = z.id
			if syncControls != nil {
				wasSyncing := a.syncing
				a.syncing = true
				syncControls(latest)
				a.syncing = wasSyncing
			}
		})
		zoneBtns[z.id] = b
		zoneBody.Append(b)
	}
	page.Append(card("Zone", zoneBody))

	// --- Power ---
	powerBody, powerSwitch := pillToggle("Lighting enabled")
	powerSwitch.SetActive(true)
	powerSwitch.ConnectStateSet(func(on bool) bool {
		ls.enabled = on
		if a.syncing {
			return false
		}
		if on {
			apply()
		} else {
			dev := ls.device
			a.do(func() error { return a.backend.LightingOff(dev) })
		}
		return false
	})
	page.Append(card("", powerBody))

	// --- Effect selector ---
	modeBody := gtk.NewFlowBox()
	modeBody.AddCSSClass("mode-grid")
	modeBody.SetSelectionMode(gtk.SelectionNone)
	modeBody.SetColumnSpacing(8)
	modeBody.SetRowSpacing(8)
	modeBody.SetMaxChildrenPerLine(3)
	modeBody.SetHomogeneous(true)
	modeBtns := map[string]*gtk.Button{}
	for _, m := range lightingModes {
		m := m
		b := gtk.NewButtonWithLabel(m.Name)
		b.AddCSSClass("mode-btn")
		b.SetHExpand(true)
		if m.ID == ls.mode {
			b.AddCSSClass("selected")
		}
		b.ConnectClicked(func() {
			for id, bb := range modeBtns {
				toggleClass(bb, "selected", id == m.ID)
			}
			ls.mode = m.ID
			if colorCard != nil && paletteCard != nil {
				isPalette := ls.mode == "palette"
				colorCard.SetVisible(!isPalette)
				paletteCard.SetVisible(isPalette)
			}
			if rebuildStops != nil && ls.mode == "palette" {
				rebuildStops()
			}
			apply()
		})
		modeBtns[m.ID] = b
		modeBody.Append(b)
	}
	page.Append(card("Effect", modeBody))

	// --- Existing quick color swatches ---
	swatchGrid := gtk.NewFlowBox()
	swatchGrid.AddCSSClass("swatch-grid")
	swatchGrid.SetSelectionMode(gtk.SelectionNone)
	swatchGrid.SetColumnSpacing(10)
	swatchGrid.SetRowSpacing(10)
	swatchGrid.SetMaxChildrenPerLine(8)
	swatchGrid.SetHomogeneous(true)
	swatchBtns := map[string]*gtk.Button{}
	for i, c := range swatches {
		c := c
		b := gtk.NewButton()
		b.AddCSSClass("swatch")
		b.AddCSSClass("swatch-" + itoa(i))
		if c == ls.color {
			b.AddCSSClass("selected")
		}
		b.ConnectClicked(func() {
			for cc, bb := range swatchBtns {
				toggleClass(bb, "selected", cc == c)
			}
			ls.color = c
			apply()
		})
		swatchBtns[c] = b
		swatchGrid.Append(b)
	}
	colorCard = card("Color", swatchGrid)
	page.Append(colorCard)

	// --- Palette editor ---
	paletteBody := vbox(12, "palette-editor")
	preview := gtk.NewDrawingArea()
	preview.AddCSSClass("palette-preview")
	preview.SetContentHeight(42)
	preview.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		drawPalettePreview(cr, w, h, ls.palette)
	})
	paletteBody.Append(preview)

	stopList := vbox(8, "palette-stops")
	paletteBody.Append(stopList)

	addButton := gtk.NewButtonWithLabel("Add Color")
	addButton.AddCSSClass("primary")
	addButton.ConnectClicked(func() {
		if len(ls.palette) >= 8 {
			return
		}
		next := defaultPalette[len(ls.palette)%len(defaultPalette)]
		ls.palette = append(ls.palette, next)
		syncControls(latest)
		apply()
	})
	paletteBody.Append(addButton)

	speedBody := hbox(8, "segmented")
	speedBtns := map[string]*gtk.Button{}
	for _, speed := range []string{"slow", "normal", "fast"} {
		speed := speed
		b := gtk.NewButtonWithLabel(capitalize(speed))
		b.AddCSSClass("seg-btn")
		b.SetHExpand(true)
		b.ConnectClicked(func() {
			ls.speed = speed
			for id, bb := range speedBtns {
				toggleClass(bb, "selected", id == speed)
			}
			apply()
		})
		speedBtns[speed] = b
		speedBody.Append(b)
	}
	paletteBody.Append(caption("Cycle speed"))
	paletteBody.Append(speedBody)
	paletteCard = card("Palette", paletteBody)
	paletteCard.SetVisible(false)
	page.Append(paletteCard)

	rebuildStops = func() {
		for child := stopList.FirstChild(); child != nil; child = stopList.FirstChild() {
			stopList.Remove(child)
		}
		dialog := gtk.NewColorDialog()
		dialog.SetTitle("Choose palette color")
		dialog.SetWithAlpha(false)
		for i, color := range ls.palette {
			index := i
			row := hbox(8, "palette-stop")
			label := gtk.NewLabel("Color " + itoa(index+1))
			label.SetHAlign(gtk.AlignStart)
			label.SetHExpand(true)

			picker := gtk.NewColorDialogButton(dialog)
			rgba := hexRGBA(color)
			picker.SetRGBA(&rgba)
			picker.SetTooltipText("Choose color " + itoa(index+1))
			picker.NotifyProperty("rgba", func() {
				if a.syncing || index >= len(ls.palette) {
					return
				}
				ls.palette[index] = rgbaHex(picker.RGBA())
				preview.QueueDraw()
				apply()
			})

			left := gtk.NewButtonFromIconName("go-previous-symbolic")
			left.SetSensitive(index > 0)
			left.SetTooltipText("Move color left")
			left.ConnectClicked(func() {
				ls.palette[index-1], ls.palette[index] = ls.palette[index], ls.palette[index-1]
				rebuildStops()
				preview.QueueDraw()
				apply()
			})
			right := gtk.NewButtonFromIconName("go-next-symbolic")
			right.SetSensitive(index < len(ls.palette)-1)
			right.SetTooltipText("Move color right")
			right.ConnectClicked(func() {
				ls.palette[index], ls.palette[index+1] = ls.palette[index+1], ls.palette[index]
				rebuildStops()
				preview.QueueDraw()
				apply()
			})
			remove := gtk.NewButtonFromIconName("user-trash-symbolic")
			remove.SetSensitive(len(ls.palette) > 2)
			remove.SetTooltipText("Remove color")
			remove.ConnectClicked(func() {
				ls.palette = append(ls.palette[:index], ls.palette[index+1:]...)
				rebuildStops()
				preview.QueueDraw()
				addButton.SetSensitive(len(ls.palette) < 8)
				apply()
			})

			row.Append(label)
			row.Append(picker)
			row.Append(left)
			row.Append(right)
			row.Append(remove)
			stopList.Append(row)
		}
		addButton.SetSensitive(len(ls.palette) < 8)
		preview.QueueDraw()
	}
	rebuildStops()

	// --- Brightness ---
	brightRow, brightScale, brightVal := sliderRow("Brightness", 0, 3, 1)
	brightScale.SetValue(3)
	brightVal.SetLabel("3")
	brightScale.ConnectValueChanged(func() {
		ls.brightness = int(brightScale.Value())
		brightVal.SetLabel(itoa(ls.brightness))
		if a.syncing {
			return
		}
		dev := ls.device
		lvl := ls.brightness
		a.do(func() error { return a.backend.SetBrightness(dev, lvl) })
	})
	page.Append(card("Brightness", brightRow))

	syncControls = func(s Snapshot) {
		latest = s
		state := lightingForDevice(s, ls.device)
		ls.mode = orDefault(state.Mode, ls.mode)
		ls.color = orDefault(state.Color, ls.color)
		ls.speed = orDefault(state.Speed, defaultLightingSpeed)
		ls.brightness = state.Brightness
		ls.enabled = state.Enabled
		if palette := paletteForDevice(s, ls.device); len(palette) >= 2 {
			ls.palette = append([]string(nil), palette...)
		}

		supported := !s.Connected || hasCapability(s.Capabilities, paletteCapability)
		modeBtns["palette"].SetSensitive(supported)
		if supported {
			modeBtns["palette"].SetTooltipText("Cycle through a custom color palette")
		} else {
			modeBtns["palette"].SetTooltipText("Requires a z13ctl daemon with palette-cycle support")
			if ls.mode == "palette" {
				ls.mode = "static"
			}
		}

		powerSwitch.SetActive(ls.enabled)
		brightScale.SetValue(float64(ls.brightness))
		brightVal.SetLabel(itoa(ls.brightness))
		for id, b := range modeBtns {
			toggleClass(b, "selected", id == ls.mode)
		}
		for c, b := range swatchBtns {
			toggleClass(b, "selected", c == ls.color)
		}
		for speed, b := range speedBtns {
			toggleClass(b, "selected", speed == ls.speed)
		}
		isPalette := ls.mode == "palette"
		colorCard.SetVisible(!isPalette)
		paletteCard.SetVisible(isPalette)
		rebuildStops()
	}

	a.register(func(s Snapshot) {
		syncControls(s)
	})
	return page
}

func lightingForDevice(s Snapshot, device string) api.LightingState {
	if device != "" && s.Devices != nil {
		if state, ok := s.Devices[device]; ok {
			return state
		}
	}
	return s.Lighting
}

func paletteForDevice(s Snapshot, device string) []string {
	if device != "" && s.Devices != nil {
		if _, ok := s.Devices[device]; ok {
			return s.Palettes[device]
		}
	}
	return s.Palettes[""]
}

func hasCapability(capabilities []string, want string) bool {
	for _, capability := range capabilities {
		if capability == want {
			return true
		}
	}
	return false
}

func drawPalettePreview(cr *cairo.Context, width, height int, palette []string) {
	if len(palette) == 0 {
		return
	}
	pattern, err := cairo.NewPatternLinear(0, 0, float64(width), 0)
	if err != nil {
		return
	}
	for i, color := range palette {
		rgba := hexRGBA(color)
		offset := 0.0
		if len(palette) > 1 {
			offset = float64(i) / float64(len(palette)-1)
		}
		_ = pattern.AddColorStopRGB(offset, float64(rgba.Red()), float64(rgba.Green()), float64(rgba.Blue()))
	}
	cr.SetSource(pattern)
	cr.Rectangle(0, 0, float64(width), float64(height))
	cr.Fill()
}

func hexRGBA(color string) gdk.RGBA {
	if len(color) != 6 {
		color = "FFFFFF"
	}
	value, err := strconv.ParseUint(color, 16, 32)
	if err != nil {
		value = 0xFFFFFF
	}
	return gdk.NewRGBA(
		float32((value>>16)&0xff)/255,
		float32((value>>8)&0xff)/255,
		float32(value&0xff)/255,
		1,
	)
}

func rgbaHex(color *gdk.RGBA) string {
	clamp := func(value float32) int {
		if value < 0 {
			return 0
		}
		if value > 1 {
			return 255
		}
		return int(value*255 + 0.5)
	}
	return fmt.Sprintf("%02X%02X%02X", clamp(color.Red()), clamp(color.Green()), clamp(color.Blue()))
}

func capitalize(value string) string {
	if value == "" {
		return value
	}
	return string(value[0]-32) + value[1:]
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func toggleClass(w interface {
	AddCSSClass(string)
	RemoveCSSClass(string)
}, class string, on bool) {
	if on {
		w.AddCSSClass(class)
	} else {
		w.RemoveCSSClass(class)
	}
}
