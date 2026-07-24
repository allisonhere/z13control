package app

// gauge.go — a reusable circular ring gauge rendered on a cairo DrawingArea.
// Used for the live APU-temperature and fan-RPM readouts on the dashboard.
// The displayed value eases toward its target each frame for a smooth sweep.

import (
	"math"

	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// ringGauge draws a value in [min,max] as a swept arc with a centered readout.
type ringGauge struct {
	area *gtk.DrawingArea

	label string // caption under the number, e.g. "APU TEMP"
	unit  string // suffix on the number, e.g. "°C"
	min   float64
	max   float64

	target  float64 // most recent real value
	shown   float64 // eased value currently drawn
	rgb     [3]float64
	ticking bool
}

// newRingGauge builds a gauge widget sized for the dashboard header.
func newRingGauge(label, unit string, min, max float64, rgb [3]float64) *ringGauge {
	g := &ringGauge{
		area:  gtk.NewDrawingArea(),
		label: label,
		unit:  unit,
		min:   min,
		max:   max,
		rgb:   rgb,
	}
	g.area.AddCSSClass("gauge")
	g.area.SetSizeRequest(124, 124)
	g.area.SetHExpand(true)
	g.area.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		g.draw(cr, w, h)
	})
	return g
}

// set updates the gauge target and starts the easing animation if idle.
func (g *ringGauge) set(v float64) {
	g.target = v
	if g.ticking {
		return
	}
	g.ticking = true
	glib.TimeoutAdd(16, func() bool {
		g.shown += (g.target - g.shown) * 0.18
		if math.Abs(g.target-g.shown) < 0.4 {
			g.shown = g.target
			g.ticking = false
			g.area.QueueDraw()
			return false
		}
		g.area.QueueDraw()
		return true
	})
}

// frac maps the eased value onto [0,1] within the gauge range.
func (g *ringGauge) frac() float64 {
	if g.max == g.min {
		return 0
	}
	f := (g.shown - g.min) / (g.max - g.min)
	return math.Max(0, math.Min(1, f))
}

func (g *ringGauge) draw(cr *cairo.Context, w, h int) {
	cx := float64(w) / 2
	cy := float64(h) / 2
	radius := math.Min(cx, cy) - 12
	const thick = 12.0

	// Arc spans 135° → 405° (a 270° open-bottom sweep).
	const start = 0.75 * math.Pi
	const sweep = 1.5 * math.Pi

	cr.SetLineCap(cairo.LineCapRound)

	// Track.
	cr.SetLineWidth(thick)
	cr.SetSourceRGBA(1, 1, 1, 0.08)
	cr.Arc(cx, cy, radius, start, start+sweep)
	cr.Stroke()

	// Value arc.
	end := start + sweep*g.frac()
	cr.SetLineWidth(thick)
	cr.SetSourceRGBA(g.rgb[0], g.rgb[1], g.rgb[2], 1)
	cr.Arc(cx, cy, radius, start, end)
	cr.Stroke()

	// Glow dot at the arc head.
	if g.frac() > 0.001 {
		hx := cx + radius*math.Cos(end)
		hy := cy + radius*math.Sin(end)
		cr.SetSourceRGBA(g.rgb[0], g.rgb[1], g.rgb[2], 0.35)
		cr.Arc(hx, hy, thick*0.9, 0, 2*math.Pi)
		cr.Fill()
		cr.SetSourceRGBA(1, 1, 1, 0.9)
		cr.Arc(hx, hy, thick*0.32, 0, 2*math.Pi)
		cr.Fill()
	}

	// Centered value.
	valText := formatGauge(g.shown)
	cr.SelectFontFace("Inter", cairo.FontSlantNormal, cairo.FontWeightBold)
	cr.SetFontSize(radius * 0.52)
	ext := cr.TextExtents(valText)
	cr.SetSourceRGBA(1, 1, 1, 0.96)
	cr.MoveTo(cx-ext.Width/2-ext.XBearing, cy+ext.Height/2)
	cr.ShowText(valText)

	// Unit, just right of the number's baseline.
	cr.SetFontSize(radius * 0.22)
	cr.SetSourceRGBA(g.rgb[0], g.rgb[1], g.rgb[2], 0.95)
	cr.MoveTo(cx+ext.Width/2+4, cy+ext.Height/2)
	cr.ShowText(g.unit)

	// Caption.
	cr.SelectFontFace("Inter", cairo.FontSlantNormal, cairo.FontWeightNormal)
	cr.SetFontSize(radius * 0.16)
	cext := cr.TextExtents(g.label)
	cr.SetSourceRGBA(1, 1, 1, 0.45)
	cr.MoveTo(cx-cext.Width/2-cext.XBearing, cy+radius*0.62)
	cr.ShowText(g.label)
}

// formatGauge renders the eased value without a decimal point.
func formatGauge(v float64) string {
	return itoa(int(math.Round(v)))
}
