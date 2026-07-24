package app

import (
	"testing"

	"github.com/dahui/z13ctl/api"
)

func TestLightingForDevice(t *testing.T) {
	t.Parallel()
	canonical := api.LightingState{Mode: "static", Color: "FF0000"}
	keyboard := api.LightingState{Mode: "palette", Color: "00FF00"}
	snapshot := Snapshot{State: api.State{
		Lighting: canonical,
		Devices:  map[string]api.LightingState{"keyboard": keyboard},
	}}

	if got := lightingForDevice(snapshot, "keyboard"); got.Mode != "palette" {
		t.Fatalf("keyboard mode = %q, want palette", got.Mode)
	}
	if got := lightingForDevice(snapshot, "lightbar"); got.Color != canonical.Color {
		t.Fatalf("lightbar color = %q, want canonical %q", got.Color, canonical.Color)
	}
}

func TestPaletteForDevice(t *testing.T) {
	t.Parallel()
	snapshot := Snapshot{
		State: api.State{
			Devices: map[string]api.LightingState{
				"keyboard": {Mode: "palette"},
			},
		},
		Palettes: map[string][]string{
			"":         {"FF0000", "0000FF"},
			"keyboard": {"00FF00", "FF00FF"},
		},
	}
	if got := paletteForDevice(snapshot, "keyboard"); got[0] != "00FF00" {
		t.Fatalf("keyboard palette = %v", got)
	}
	if got := paletteForDevice(snapshot, "lightbar"); got[0] != "FF0000" {
		t.Fatalf("lightbar palette = %v", got)
	}
}

func TestColorConversions(t *testing.T) {
	t.Parallel()
	for _, want := range []string{"000000", "123456", "FFFFFF", "FF0066"} {
		rgba := hexRGBA(want)
		if got := rgbaHex(&rgba); got != want {
			t.Errorf("round trip %s = %s", want, got)
		}
	}
}

func TestHasCapability(t *testing.T) {
	t.Parallel()
	if !hasCapability([]string{"other", paletteCapability}, paletteCapability) {
		t.Error("palette capability not found")
	}
	if hasCapability([]string{"other"}, paletteCapability) {
		t.Error("missing palette capability reported present")
	}
}
