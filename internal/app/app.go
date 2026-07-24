package app

// app.go — application shell: window, sidebar navigation, page stack, the live
// telemetry loop and the daemon-connection banner. Individual control surfaces
// live in page_*.go and register a sync callback so a single refresh() fans the
// latest daemon snapshot out to every page.

import (
	_ "embed"
	"fmt"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

//go:embed style.css
var styleCSS string

// App is the running z13center application.
type App struct {
	backend *Backend
	gtkApp  *gtk.Application
	win     *gtk.ApplicationWindow

	stack      *gtk.Stack
	navButtons map[string]*gtk.Button
	navActive  string

	// header + connection.
	headerTitle *gtk.Label
	headerRead  *gtk.Label
	connBanner  *gtk.Revealer
	connLabel   *gtk.Label
	toastUntil  time.Time

	// dashboard gauges (also driven by the telemetry loop).
	tempGauge *ringGauge
	fanGauge  *ringGauge

	// fan-page canvas, redrawn each telemetry tick for the live-temp marker.
	fanRedraw *gtk.DrawingArea

	syncFns []func(Snapshot)
	snap    Snapshot
	syncing bool // suppresses widget-change handlers during programmatic sync
}

// New constructs the application.
func New() *App {
	return &App{
		backend:    NewBackend(),
		navButtons: map[string]*gtk.Button{},
	}
}

// Run creates the GTK application and blocks until the window closes.
func (a *App) Run(args []string) int {
	a.gtkApp = gtk.NewApplication("io.github.z13center", 0)
	a.gtkApp.ConnectActivate(a.activate)
	return a.gtkApp.Run(args)
}

func (a *App) activate() {
	a.loadCSS()

	a.win = gtk.NewApplicationWindow(a.gtkApp)
	a.win.SetTitle("z13 Control Center")
	a.win.SetDefaultSize(760, 560)
	a.win.SetDecorated(false) // no server title bar — our header is the drag handle
	a.win.AddCSSClass("z13-root")

	root := gtk.NewBox(gtk.OrientationHorizontal, 0)
	root.Append(a.buildSidebar())
	root.Append(a.buildContent())
	a.win.SetChild(root)

	a.refresh()
	a.startTelemetry()
	a.startDaemonEvents()
	a.win.Present()
}

func (a *App) loadCSS() {
	provider := gtk.NewCSSProvider()
	provider.LoadFromString(styleCSS)
	gtk.StyleContextAddProviderForDisplay(
		gdk.DisplayGetDefault(), provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
}

// buildSidebar builds the left navigation rail.
func (a *App) buildSidebar() gtk.Widgetter {
	side := gtk.NewBox(gtk.OrientationVertical, 0)
	side.AddCSSClass("sidebar")
	side.SetSizeRequest(210, -1)

	brand := gtk.NewBox(gtk.OrientationVertical, 0)
	brand.AddCSSClass("brand")
	logo := gtk.NewLabel("Z13")
	logo.AddCSSClass("brand-logo")
	logo.SetHAlign(gtk.AlignStart)
	sub := gtk.NewLabel("CONTROL CENTER")
	sub.AddCSSClass("brand-sub")
	sub.SetHAlign(gtk.AlignStart)
	brand.Append(logo)
	brand.Append(sub)
	side.Append(brand)

	nav := []struct{ id, name, icon string }{
		{"dashboard", "Dashboard", "speedometer-symbolic"},
		{"fans", "Fan Curve", "weather-windy-symbolic"},
		{"power", "Power", "battery-full-charging-symbolic"},
		{"lighting", "Lighting", "display-brightness-symbolic"},
		{"battery", "Battery & Display", "video-display-symbolic"},
	}
	for _, n := range nav {
		btn := gtk.NewButton()
		btn.AddCSSClass("nav-item")
		btn.SetHAlign(gtk.AlignFill)
		row := gtk.NewBox(gtk.OrientationHorizontal, 12)
		icon := gtk.NewImageFromIconName(n.icon)
		lbl := gtk.NewLabel(n.name)
		lbl.SetHAlign(gtk.AlignStart)
		lbl.SetHExpand(true)
		row.Append(icon)
		row.Append(lbl)
		btn.SetChild(row)
		id := n.id
		btn.ConnectClicked(func() { a.navigate(id) })
		a.navButtons[id] = btn
		side.Append(btn)
	}

	// Spacer pushes the connection pill to the bottom.
	spacer := gtk.NewBox(gtk.OrientationVertical, 0)
	spacer.SetVExpand(true)
	side.Append(spacer)

	return side
}

// buildContent builds the header + page stack on the right.
func (a *App) buildContent() gtk.Widgetter {
	outer := gtk.NewBox(gtk.OrientationVertical, 0)
	outer.SetHExpand(true)
	outer.AddCSSClass("content")

	// Connection banner (revealed only when the daemon is offline).
	a.connLabel = gtk.NewLabel("")
	a.connLabel.AddCSSClass("conn-label")
	a.connLabel.SetHAlign(gtk.AlignStart)
	a.connBanner = gtk.NewRevealer()
	a.connBanner.SetChild(a.connLabel)
	a.connBanner.SetRevealChild(false)
	outer.Append(a.connBanner)

	// Header row — doubles as the window's drag handle and window controls,
	// since the server title bar is disabled.
	header := gtk.NewBox(gtk.OrientationHorizontal, 12)
	header.AddCSSClass("header")
	a.headerTitle = gtk.NewLabel("Dashboard")
	a.headerTitle.AddCSSClass("header-title")
	a.headerTitle.SetHAlign(gtk.AlignStart)
	a.headerTitle.SetHExpand(true)
	a.headerRead = gtk.NewLabel("—")
	a.headerRead.AddCSSClass("header-read")
	a.headerRead.SetHAlign(gtk.AlignEnd)

	minBtn := gtk.NewButtonFromIconName("window-minimize-symbolic")
	minBtn.AddCSSClass("win-btn")
	minBtn.ConnectClicked(func() { a.win.Minimize() })
	closeBtn := gtk.NewButtonFromIconName("window-close-symbolic")
	closeBtn.AddCSSClass("win-btn")
	closeBtn.AddCSSClass("win-close")
	closeBtn.ConnectClicked(func() { a.win.Close() })

	header.Append(a.headerTitle)
	header.Append(a.headerRead)
	header.Append(minBtn)
	header.Append(closeBtn)

	handle := gtk.NewWindowHandle()
	handle.SetChild(header)
	outer.Append(handle)

	// Page stack.
	a.stack = gtk.NewStack()
	a.stack.SetTransitionType(gtk.StackTransitionTypeCrossfade)
	a.stack.SetVExpand(true)
	a.stack.AddNamed(a.wrapScroll(a.buildDashboard()), "dashboard")
	a.stack.AddNamed(a.wrapScroll(a.buildFanPage()), "fans")
	a.stack.AddNamed(a.wrapScroll(a.buildPowerPage()), "power")
	a.stack.AddNamed(a.wrapScroll(a.buildLightingPage()), "lighting")
	a.stack.AddNamed(a.wrapScroll(a.buildBatteryPage()), "battery")
	outer.Append(a.stack)

	a.navigate("dashboard")
	return outer
}

// wrapScroll puts a page in a vertically scrolling container with padding.
func (a *App) wrapScroll(child gtk.Widgetter) gtk.Widgetter {
	pad := gtk.NewBox(gtk.OrientationVertical, 0)
	pad.AddCSSClass("page")
	pad.Append(child)
	scroll := gtk.NewScrolledWindow()
	scroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroll.SetOverlayScrolling(true) // bar floats over content, reserves no space
	scroll.SetVExpand(true)
	scroll.SetChild(pad)
	return scroll
}

// navigate switches the visible page and highlights the nav item.
func (a *App) navigate(id string) {
	if old, ok := a.navButtons[a.navActive]; ok {
		old.RemoveCSSClass("active")
	}
	a.navActive = id
	if btn, ok := a.navButtons[id]; ok {
		btn.AddCSSClass("active")
	}
	a.stack.SetVisibleChildName(id)
	titles := map[string]string{
		"dashboard": "Dashboard", "fans": "Fan Curve", "power": "Power",
		"lighting": "Lighting", "battery": "Battery & Display",
	}
	a.headerTitle.SetLabel(titles[id])
}

// refresh pulls a fresh snapshot and fans it out to every page.
func (a *App) refresh() {
	a.snap = a.backend.Load()
	a.syncing = true
	for _, fn := range a.syncFns {
		fn(a.snap)
	}
	a.syncing = false
	a.applyTelemetry(a.snap)
	a.updateConnBanner(a.snap.Connected)
}

// updateConnBanner shows/hides the offline notice.
func (a *App) updateConnBanner(connected bool) {
	if connected {
		a.connBanner.SetRevealChild(false)
		return
	}
	a.connLabel.SetLabel("  ⚠  z13ctl daemon offline — changes are simulated in preview mode")
	a.connBanner.SetRevealChild(true)
}

// do runs a backend mutation off the UI thread, then refreshes on the UI thread.
func (a *App) do(fn func() error) {
	a.doWithFeedback("", fn)
}

// doWithFeedback runs a mutation and keeps its result visible long enough to
// survive the one-second telemetry refresh.
func (a *App) doWithFeedback(success string, fn func() error) {
	go func() {
		if err := fn(); err != nil {
			glib.IdleAdd(func() { a.toast(err.Error()) })
		} else if success != "" {
			glib.IdleAdd(func() { a.toast(success) })
		}
		glib.IdleAdd(a.refresh)
	}()
}

// toast surfaces action feedback in the header readout for four seconds.
func (a *App) toast(msg string) {
	a.toastUntil = time.Now().Add(4 * time.Second)
	a.headerRead.SetLabel(msg)
	glib.TimeoutAdd(4000, func() bool {
		if !time.Now().Before(a.toastUntil) {
			a.applyTelemetry(a.snap)
		}
		return false
	})
}

// applyTelemetry pushes live temp/RPM into the gauges + header readout.
func (a *App) applyTelemetry(s Snapshot) {
	if a.tempGauge != nil {
		a.tempGauge.set(float64(s.Temperature))
	}
	if a.fanGauge != nil {
		a.fanGauge.set(float64(s.FanRPM))
	}
	if a.fanRedraw != nil {
		a.fanRedraw.QueueDraw()
	}
	if !time.Now().Before(a.toastUntil) {
		a.headerRead.SetLabel(fmt.Sprintf("%d°C   ·   %d RPM", s.Temperature, s.FanRPM))
	}
}

// startTelemetry polls the daemon once per second for temp + fan RPM.
func (a *App) startTelemetry() {
	glib.TimeoutAdd(1000, func() bool {
		go func() {
			snap := a.backend.Load()
			glib.IdleAdd(func() {
				a.snap = snap
				a.applyTelemetry(snap)
				a.updateConnBanner(snap.Connected)
			})
		}()
		return true // keep ticking
	})
}

// startDaemonEvents subscribes to the daemon's event stream and raises the
// window on the Armoury Crate "gui-toggle" event (no-op when daemon offline).
func (a *App) startDaemonEvents() {
	ch, cancel, err := a.backend.Subscribe([]string{"gui-toggle"})
	if err != nil || ch == nil {
		return
	}
	go func() {
		defer cancel()
		for ev := range ch {
			if ev == "gui-toggle" {
				glib.IdleAdd(func() { a.win.Present() })
			}
		}
	}()
}

// register adds a page sync callback.
func (a *App) register(fn func(Snapshot)) { a.syncFns = append(a.syncFns, fn) }
