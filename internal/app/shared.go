package app

// shared.go — small helpers and tunables shared across the app package.

import (
	"strconv"
	"strings"

	"github.com/dahui/z13ctl/api"
)

// TDP limits mirror the daemon safety constants (internal/cli/tdp.go).
const (
	tdpMin     = 5
	tdpBasic   = 70 // basic slider ceiling
	tdpSafeMax = 75 // above this the daemon requires force=true
	tdpMax     = 93 // absolute ceiling for the 2025 GZ302E
)

// Undervolt Curve Optimizer range (0 = stock, -40 = deepest supported).
const (
	uvMin = -40
	uvMax = 0
)

// profiles is the ordered list of selectable performance profiles. "custom"
// re-applies saved fan curve + TDP + undervolt and is driven from those panels.
var profiles = []profileMeta{
	{ID: "quiet", Name: "Quiet", Icon: "weather-clear-night-symbolic", Blurb: "Silent · low power"},
	{ID: "balanced", Name: "Balanced", Icon: "weather-few-clouds-symbolic", Blurb: "Everyday default"},
	{ID: "performance", Name: "Performance", Icon: "power-profile-performance-symbolic", Blurb: "Max clocks · loud"},
	{ID: "custom", Name: "Custom", Icon: "applications-engineering-symbolic", Blurb: "Your fan + power curve"},
}

type profileMeta struct {
	ID, Name, Icon, Blurb string
}

// lightingModes are the Aura effects exposed in the lighting panel. IDs must
// match aura.ModeFromString except palette, which is managed by the daemon.
var lightingModes = []struct{ ID, Name string }{
	{"static", "Static"},
	{"breathe", "Breathe"},
	{"cycle", "Cycle"},
	{"rainbow", "Rainbow"},
	{"strobe", "Strobe"},
	{"palette", "Palette Cycle"},
}

// Aura animation speed accepted by the daemon (aura.SpeedFromString):
// one of slow|normal|fast.
const defaultLightingSpeed = "normal"

// swatches are the quick-pick colors in the lighting panel (RRGGBB).
var swatches = []string{
	"FF0066", "FF3B00", "FFB300", "9BFF00",
	"00E5FF", "2979FF", "7C4DFF", "FFFFFF",
}

// defaultFanCurve returns a sensible 8-point starting curve.
func defaultFanCurve() [8]api.FanCurvePoint {
	return [8]api.FanCurvePoint{
		{Temp: 35, PWM: 0},
		{Temp: 45, PWM: 25},
		{Temp: 50, PWM: 50},
		{Temp: 60, PWM: 80},
		{Temp: 70, PWM: 120},
		{Temp: 80, PWM: 170},
		{Temp: 90, PWM: 220},
		{Temp: 100, PWM: 255},
	}
}

// fanCurveString encodes points as "temp:pwm,temp:pwm,..." for the daemon.
func fanCurveString(pts [8]api.FanCurvePoint) string {
	parts := make([]string, 0, len(pts))
	for _, p := range pts {
		parts = append(parts, itoa(p.Temp)+":"+itoa(p.PWM))
	}
	return strings.Join(parts, ",")
}

func itoa(v int) string { return strconv.Itoa(v) }

// clampInt constrains v to [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
