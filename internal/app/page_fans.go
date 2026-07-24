package app

// page_fans.go — the visual 8-point fan-curve editor. Points are dragged on a
// cairo DrawingArea; temps stay strictly increasing and PWM non-decreasing. A
// dashed marker tracks the live APU temperature so you tune against real load.

import (
	"math"

	"github.com/dahui/z13ctl/api"
	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type fanEditor struct {
	area     *gtk.DrawingArea
	points   [8]api.FanCurvePoint
	dragging int
	hovered  int
	app      *App

	chartX, chartY, chartW, chartH float64
}

func (a *App) buildFanPage() gtk.Widgetter {
	page := vbox(16, "fan-page")

	ed := &fanEditor{dragging: -1, hovered: -1, app: a, points: defaultFanCurve()}
	ed.area = gtk.NewDrawingArea()
	ed.area.AddCSSClass("fan-canvas")
	ed.area.SetSizeRequest(-1, 240)
	ed.area.SetHExpand(true)
	ed.area.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		ed.draw(cr, w, h)
	})

	drag := gtk.NewGestureDrag()
	drag.SetPropagationPhase(gtk.PhaseCapture)
	var sx, sy float64
	drag.ConnectDragBegin(func(x, y float64) {
		idx := ed.hitTest(x, y)
		if idx < 0 {
			drag.SetState(gtk.EventSequenceDenied)
			return
		}
		ed.dragging = idx
		sx, sy = x, y
		ed.area.QueueDraw()
	})
	drag.ConnectDragUpdate(func(ox, oy float64) {
		if ed.dragging < 0 {
			return
		}
		ed.points[ed.dragging].Temp = ed.xToTemp(sx + ox)
		ed.points[ed.dragging].PWM = ed.yToPWM(sy + oy)
		ed.enforce(ed.dragging)
		ed.area.QueueDraw()
	})
	drag.ConnectDragEnd(func(_, _ float64) {
		ed.dragging = -1
		ed.area.QueueDraw()
	})
	ed.area.AddController(drag)

	motion := gtk.NewEventControllerMotion()
	motion.ConnectMotion(func(x, y float64) {
		if ed.dragging >= 0 {
			return
		}
		prev := ed.hovered
		ed.hovered = ed.hitTest(x, y)
		if ed.hovered != prev {
			ed.area.QueueDraw()
		}
	})
	motion.ConnectLeave(func() {
		if ed.hovered >= 0 {
			ed.hovered = -1
			ed.area.QueueDraw()
		}
	})
	ed.area.AddController(motion)

	page.Append(card("Fan Curve — drag any point", ed.area))

	// Actions.
	actions := hbox(10, "action-row")
	applyBtn := gtk.NewButtonWithLabel("Apply Curve")
	applyBtn.AddCSSClass("primary")
	applyBtn.SetHExpand(true)
	applyBtn.ConnectClicked(func() {
		pts := ed.points
		a.doWithFeedback("✓ Fan curve applied", func() error {
			return a.backend.SetFanCurve(pts)
		})
	})
	resetBtn := gtk.NewButtonWithLabel("Reset to Auto")
	resetBtn.SetHExpand(true)
	resetBtn.ConnectClicked(func() {
		a.doWithFeedback("✓ Fans reset to firmware auto mode", func() error {
			return a.backend.ResetFanCurve()
		})
	})
	actions.Append(applyBtn)
	actions.Append(resetBtn)
	page.Append(card("", actions))

	page.Append(caption("Applying a custom curve switches the active profile to “Custom”. " +
		"Both fans share one curve — they cool the same APU. The dashed line marks the live APU temperature."))

	a.register(func(s Snapshot) {
		if s.FanCurve != nil && len(s.FanCurve.Points) == 8 {
			copy(ed.points[:], s.FanCurve.Points)
		}
		ed.area.QueueDraw()
	})

	// Redraw once a second so the live-temperature marker tracks telemetry.
	a.fanRedraw = ed.area
	return page
}

// --- coordinate mapping (35–105°C on X, 0–255 PWM on Y) ---

func (ed *fanEditor) tempToX(t int) float64 { return ed.chartX + float64(t-35)/70.0*ed.chartW }
func (ed *fanEditor) pwmToY(p int) float64  { return ed.chartY + ed.chartH - float64(p)/255.0*ed.chartH }

func (ed *fanEditor) xToTemp(x float64) int {
	return clampInt(35+int(math.Round((x-ed.chartX)/ed.chartW*70.0)), 35, 105)
}
func (ed *fanEditor) yToPWM(y float64) int {
	return clampInt(int(math.Round((ed.chartY+ed.chartH-y)/ed.chartH*255.0)), 0, 255)
}

func (ed *fanEditor) hitTest(x, y float64) int {
	const tol = 22.0
	best, bestD := -1, tol*tol
	for i, p := range ed.points {
		dx := x - ed.tempToX(p.Temp)
		dy := y - ed.pwmToY(p.PWM)
		if d := dx*dx + dy*dy; d < bestD {
			bestD, best = d, i
		}
	}
	return best
}

// enforce keeps temps strictly increasing and PWM non-decreasing around idx.
func (ed *fanEditor) enforce(idx int) {
	ed.points[idx].Temp = clampInt(ed.points[idx].Temp, 35, 105)
	ed.points[idx].PWM = clampInt(ed.points[idx].PWM, 0, 255)
	for i := idx + 1; i < 8; i++ {
		if ed.points[i].Temp <= ed.points[i-1].Temp {
			ed.points[i].Temp = ed.points[i-1].Temp + 1
		}
		if ed.points[i].PWM < ed.points[i-1].PWM {
			ed.points[i].PWM = ed.points[i-1].PWM
		}
	}
	for i := idx - 1; i >= 0; i-- {
		if ed.points[i].Temp >= ed.points[i+1].Temp {
			ed.points[i].Temp = ed.points[i+1].Temp - 1
		}
		if ed.points[i].PWM > ed.points[i+1].PWM {
			ed.points[i].PWM = ed.points[i+1].PWM
		}
	}
	for i := range ed.points {
		ed.points[i].Temp = clampInt(ed.points[i].Temp, 35, 105)
		ed.points[i].PWM = clampInt(ed.points[i].PWM, 0, 255)
	}
}

func (ed *fanEditor) draw(cr *cairo.Context, width, height int) {
	w, h := float64(width), float64(height)
	const lm, bm, tm, rm = 44.0, 24.0, 12.0, 12.0
	ed.chartX, ed.chartY = lm, tm
	ed.chartW, ed.chartH = w-lm-rm, h-tm-bm

	// Grid.
	cr.SetLineWidth(1)
	cr.SetSourceRGBA(1, 1, 1, 0.06)
	for _, pct := range []int{0, 25, 50, 75, 100} {
		y := ed.pwmToY(pct * 255 / 100)
		cr.MoveTo(ed.chartX, y)
		cr.LineTo(ed.chartX+ed.chartW, y)
	}
	for t := 35; t <= 105; t += 10 {
		x := ed.tempToX(t)
		cr.MoveTo(x, ed.chartY)
		cr.LineTo(x, ed.chartY+ed.chartH)
	}
	cr.Stroke()

	// Axis labels.
	cr.SelectFontFace("Inter", cairo.FontSlantNormal, cairo.FontWeightNormal)
	cr.SetFontSize(10)
	cr.SetSourceRGBA(1, 1, 1, 0.4)
	for _, pct := range []int{0, 25, 50, 75, 100} {
		y := ed.pwmToY(pct * 255 / 100)
		cr.MoveTo(6, y+3)
		cr.ShowText(itoa(pct) + "%")
	}
	for t := 40; t <= 100; t += 20 {
		x := ed.tempToX(t)
		cr.MoveTo(x-8, ed.chartY+ed.chartH+16)
		cr.ShowText(itoa(t) + "°")
	}

	// Live APU-temperature marker.
	if ed.app != nil && ed.app.snap.Temperature >= 35 && ed.app.snap.Temperature <= 105 {
		tx := ed.tempToX(ed.app.snap.Temperature)
		cr.SetSourceRGBA(1, 1, 1, 0.35)
		cr.SetLineWidth(1.5)
		cr.SetDash([]float64{5, 4}, 0)
		cr.MoveTo(tx, ed.chartY)
		cr.LineTo(tx, ed.chartY+ed.chartH)
		cr.Stroke()
		cr.SetDash(nil, 0)
	}

	// Filled area.
	cr.SetSourceRGBA(0.0, 0.85, 1.0, 0.12)
	cr.MoveTo(ed.tempToX(ed.points[0].Temp), ed.pwmToY(0))
	for _, p := range ed.points {
		cr.LineTo(ed.tempToX(p.Temp), ed.pwmToY(p.PWM))
	}
	cr.LineTo(ed.tempToX(ed.points[7].Temp), ed.pwmToY(0))
	cr.ClosePath()
	cr.Fill()

	// Curve line.
	cr.SetLineCap(cairo.LineCapRound)
	cr.SetLineJoin(cairo.LineJoinRound)
	cr.SetLineWidth(2.5)
	cr.SetSourceRGBA(0.0, 0.85, 1.0, 1)
	for i, p := range ed.points {
		x, y := ed.tempToX(p.Temp), ed.pwmToY(p.PWM)
		if i == 0 {
			cr.MoveTo(x, y)
		} else {
			cr.LineTo(x, y)
		}
	}
	cr.Stroke()

	// Points.
	for i, p := range ed.points {
		x, y := ed.tempToX(p.Temp), ed.pwmToY(p.PWM)
		r := 6.0
		if i == ed.dragging || i == ed.hovered {
			r = 8.5
			cr.SetSourceRGBA(1, 1, 1, 0.5)
			cr.Arc(x, y, r+3, 0, 2*math.Pi)
			cr.Stroke()
		}
		cr.SetSourceRGBA(0.04, 0.05, 0.09, 1)
		cr.Arc(x, y, r+1.5, 0, 2*math.Pi)
		cr.Fill()
		cr.SetSourceRGBA(0.0, 0.85, 1.0, 1)
		cr.Arc(x, y, r, 0, 2*math.Pi)
		cr.Fill()
	}
}
