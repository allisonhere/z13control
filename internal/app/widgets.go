package app

// widgets.go — small composable builders shared by the pages.

import "github.com/diamondburned/gotk4/pkg/gtk/v4"

// card wraps content in a titled surface panel.
func card(title string, body gtk.Widgetter) *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVertical, 10)
	box.AddCSSClass("card")
	if title != "" {
		t := gtk.NewLabel(title)
		t.AddCSSClass("card-title")
		t.SetHAlign(gtk.AlignStart)
		box.Append(t)
	}
	box.Append(body)
	return box
}

// vbox / hbox are terse constructors with a class applied.
func vbox(spacing int, class string) *gtk.Box {
	b := gtk.NewBox(gtk.OrientationVertical, spacing)
	if class != "" {
		b.AddCSSClass(class)
	}
	return b
}

func hbox(spacing int, class string) *gtk.Box {
	b := gtk.NewBox(gtk.OrientationHorizontal, spacing)
	if class != "" {
		b.AddCSSClass(class)
	}
	return b
}

// caption is a muted small label.
func caption(text string) *gtk.Label {
	l := gtk.NewLabel(text)
	l.AddCSSClass("caption")
	l.SetHAlign(gtk.AlignStart)
	l.SetWrap(true)
	return l
}

// sliderRow builds "Name … value" with a slider underneath. onChange fires only
// on genuine user interaction (the caller guards programmatic syncs via App.syncing).
func sliderRow(name string, min, max, step float64) (*gtk.Box, *gtk.Scale, *gtk.Label) {
	row := vbox(2, "slider-row")

	head := hbox(8, "")
	nameLbl := gtk.NewLabel(name)
	nameLbl.AddCSSClass("slider-name")
	nameLbl.SetHAlign(gtk.AlignStart)
	nameLbl.SetHExpand(true)
	valLbl := gtk.NewLabel("")
	valLbl.AddCSSClass("slider-value")
	valLbl.SetHAlign(gtk.AlignEnd)
	head.Append(nameLbl)
	head.Append(valLbl)

	scale := gtk.NewScaleWithRange(gtk.OrientationHorizontal, min, max, step)
	scale.SetDigits(0)
	scale.SetDrawValue(false)
	scale.SetHExpand(true)
	scale.SetFocusable(false)

	row.Append(head)
	row.Append(scale)
	return row, scale, valLbl
}

// pillToggle builds a labeled on/off switch row.
func pillToggle(name string) (*gtk.Box, *gtk.Switch) {
	row := hbox(8, "toggle-row")
	lbl := gtk.NewLabel(name)
	lbl.AddCSSClass("slider-name")
	lbl.SetHAlign(gtk.AlignStart)
	lbl.SetHExpand(true)
	sw := gtk.NewSwitch()
	sw.SetHAlign(gtk.AlignEnd)
	sw.SetVAlign(gtk.AlignCenter)
	row.Append(lbl)
	row.Append(sw)
	return row, sw
}
