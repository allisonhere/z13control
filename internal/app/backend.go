package app

// backend.go — thin wrapper over the z13ctl daemon api.
//
// Every mutation goes through the daemon socket when it is reachable. When the
// daemon is not running (fresh machine, z13ctl not installed yet, or Wayland
// session without the user service) the backend keeps an in-memory mock state
// so the whole UI stays interactive for development and preview. The Connected
// flag lets the UI show an honest "daemon offline — changes are simulated"
// banner instead of silently doing nothing.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/user"
	"strings"
	"sync"
	"time"

	"github.com/dahui/z13ctl/api"
)

// Backend mediates all communication with the z13ctl daemon.
type Backend struct {
	mu           sync.Mutex
	mock         api.State // authoritative only while the daemon is offline
	mockPalettes map[string][]string
}

// NewBackend returns a Backend seeded with a plausible idle-Z13 mock state.
func NewBackend() *Backend {
	return &Backend{
		mock: defaultMockState(),
		mockPalettes: map[string][]string{
			"": {"FF0066", "00E5FF", "7C4DFF"},
		},
	}
}

// Snapshot is an immutable view handed to the UI on each refresh.
type Snapshot struct {
	api.State
	Connected    bool // true when the live daemon answered
	Capabilities []string
	Palettes     map[string][]string // "" is canonical; named keys are overrides
}

var dialLightingDaemon = func() (net.Conn, error) {
	return net.DialTimeout("unix", api.SocketPath(), time.Second)
}

var lightingDaemonLive = func() bool {
	ok, _, err := api.SendGetState()
	return ok && err == nil
}

// Load fetches the full daemon state, falling back to the mock when offline.
func (b *Backend) Load() Snapshot {
	if ok, snap, err := loadExtendedState(); ok && err == nil {
		return snap
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return Snapshot{
		State:        b.mock,
		Connected:    false,
		Capabilities: []string{"palette-cycle"},
		Palettes:     clonePalettes(b.mockPalettes),
	}
}

// loadExtendedState decodes additive daemon fields that are newer than the
// currently released z13ctl API module. It can be replaced by api.SendGetState
// once a release containing palette-cycle support is available.
func loadExtendedState() (bool, Snapshot, error) {
	conn, err := dialLightingDaemon()
	if err != nil {
		return false, Snapshot{}, nil
	}
	defer func() { _ = conn.Close() }()
	if _, err := fmt.Fprintln(conn, `{"cmd":"get-state"}`); err != nil {
		return true, Snapshot{}, err
	}
	var response struct {
		OK    bool            `json:"ok"`
		Error string          `json:"error,omitempty"`
		State json.RawMessage `json:"state,omitempty"`
	}
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return true, Snapshot{}, fmt.Errorf("no response from daemon")
	}
	if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
		return true, Snapshot{}, err
	}
	if !response.OK {
		return true, Snapshot{}, fmt.Errorf("%s", response.Error)
	}
	var st api.State
	if err := json.Unmarshal(response.State, &st); err != nil {
		return true, Snapshot{}, err
	}
	var extended struct {
		Capabilities []string `json:"capabilities,omitempty"`
		Lighting     struct {
			Palette []string `json:"palette,omitempty"`
		} `json:"lighting"`
		Devices map[string]struct {
			Palette []string `json:"palette,omitempty"`
		} `json:"devices,omitempty"`
	}
	if err := json.Unmarshal(response.State, &extended); err != nil {
		return true, Snapshot{}, err
	}
	palettes := map[string][]string{"": append([]string(nil), extended.Lighting.Palette...)}
	for device, lighting := range extended.Devices {
		palettes[device] = append([]string(nil), lighting.Palette...)
	}
	return true, Snapshot{
		State:        st,
		Connected:    true,
		Capabilities: extended.Capabilities,
		Palettes:     palettes,
	}, nil
}

// live reports whether the daemon socket is currently answering.
func (b *Backend) live() bool {
	return lightingDaemonLive()
}

// SetProfile applies a performance profile.
func (b *Backend) SetProfile(p string) error {
	if b.live() {
		_, err := api.SendProfileSet(p)
		return err
	}
	b.mu.Lock()
	b.mock.Profile = p
	b.mu.Unlock()
	return nil
}

// SetBatteryLimit applies a battery charge-limit percentage.
func (b *Backend) SetBatteryLimit(pct int) error {
	if b.live() {
		_, err := api.SendBatteryLimitSet(pct)
		return err
	}
	b.mu.Lock()
	b.mock.Battery = pct
	b.mu.Unlock()
	return nil
}

// SetPanelOverdrive toggles panel refresh overdrive (0/1).
func (b *Backend) SetPanelOverdrive(v int) error {
	if b.live() {
		_, err := api.SendPanelOverdriveSet(v)
		return err
	}
	b.mu.Lock()
	b.mock.PanelOverdrive = v
	b.mu.Unlock()
	return nil
}

// SetBootSound toggles the POST boot sound (0/1).
func (b *Backend) SetBootSound(v int) error {
	if b.live() {
		_, err := api.SendBootSoundSet(v)
		return err
	}
	b.mu.Lock()
	b.mock.BootSound = v
	b.mu.Unlock()
	return nil
}

// SetTDP applies TDP power limits. When advanced is false only the sustained
// watts value is used and the daemon derives the boost limits.
func (b *Backend) SetTDP(watts, pl1, pl2, pl3 int, advanced bool) error {
	if b.live() {
		if advanced {
			force := pl1 > tdpSafeMax || pl2 > tdpSafeMax || pl3 > tdpSafeMax
			_, err := api.SendTdpSet("", itoa(pl1), itoa(pl2), itoa(pl3), force)
			return err
		}
		_, err := api.SendTdpSet(itoa(watts), "", "", "", false)
		return err
	}
	b.mu.Lock()
	if b.mock.TDP == nil {
		b.mock.TDP = &api.TDPState{}
	}
	if advanced {
		b.mock.TDP.PL1SPL, b.mock.TDP.PL2SPPT, b.mock.TDP.FPPT = pl1, pl2, pl3
	} else {
		b.mock.TDP.PL1SPL, b.mock.TDP.PL2SPPT, b.mock.TDP.FPPT = watts, watts, watts
	}
	b.mu.Unlock()
	return nil
}

// ResetTDP restores firmware default power limits.
func (b *Backend) ResetTDP() error {
	if b.live() {
		_, err := api.SendTdpReset()
		return err
	}
	b.mu.Lock()
	b.mock.TDP = &api.TDPState{PL1SPL: 45, PL2SPPT: 54, FPPT: 65}
	b.mu.Unlock()
	return nil
}

// SetFanCurve applies an 8-point fan curve to both fans.
func (b *Backend) SetFanCurve(pts [8]api.FanCurvePoint) error {
	if b.live() {
		_, err := api.SendFanCurveSet(fanCurveString(pts))
		return fanControlError(err)
	}
	b.mu.Lock()
	if b.mock.FanCurve == nil {
		b.mock.FanCurve = &api.FanCurveState{Mode: 1}
	}
	b.mock.FanCurve.Points = pts[:]
	b.mu.Unlock()
	return nil
}

// ResetFanCurve returns both fans to firmware auto mode.
func (b *Backend) ResetFanCurve() error {
	if b.live() {
		_, err := api.SendFanCurveReset()
		return fanControlError(err)
	}
	b.mu.Lock()
	b.mock.FanCurve = nil
	b.mu.Unlock()
	return nil
}

func fanControlError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "permission denied") {
		group := "users"
		if current, lookupErr := user.LookupGroupId(itoa(os.Getgid())); lookupErr == nil {
			group = current.Name
		}
		return fmt.Errorf("fan control needs system permissions; run: sudo z13ctl setup --group %s", group)
	}
	return err
}

// SetUndervolt applies an all-core CPU Curve Optimizer offset (0..-40).
func (b *Backend) SetUndervolt(co int) error {
	if b.live() {
		_, err := api.SendUndervoltSet(itoa(co))
		return err
	}
	b.mu.Lock()
	b.mock.Undervolt = &api.UndervoltState{CPUCO: co, Active: co != 0}
	b.mu.Unlock()
	return nil
}

// ResetUndervolt restores stock voltage (offset 0).
func (b *Backend) ResetUndervolt() error {
	if b.live() {
		_, err := api.SendUndervoltReset()
		return err
	}
	b.mu.Lock()
	b.mock.Undervolt = &api.UndervoltState{}
	b.mu.Unlock()
	return nil
}

// ApplyLighting sets a lighting effect on a device ("" = all zones).
func (b *Backend) ApplyLighting(device, color, color2, mode, speed string, brightness int) error {
	if b.live() {
		_, err := api.SendApply(device, color, color2, mode, speed, brightness)
		return err
	}
	b.mu.Lock()
	ls := api.LightingState{
		Enabled: true, Mode: mode, Color: color, Color2: color2,
		Speed: speed, Brightness: brightness,
	}
	b.setMockLighting(device, ls)
	if device == "" {
		b.mockPalettes = make(map[string][]string)
	} else {
		delete(b.mockPalettes, device)
	}
	b.mu.Unlock()
	return nil
}

// ApplyPalette starts a daemon-managed custom palette cycle.
func (b *Backend) ApplyPalette(device string, palette []string, speed string, brightness int) error {
	if b.live() {
		_, err := sendPalette(device, palette, speed, brightness)
		return err
	}
	b.mu.Lock()
	ls := api.LightingState{
		Enabled: true, Mode: "palette", Color: palette[0], Color2: palette[1],
		Speed: speed, Brightness: brightness,
	}
	b.setMockLighting(device, ls)
	if device == "" {
		b.mockPalettes = map[string][]string{"": append([]string(nil), palette...)}
	} else {
		b.mockPalettes[device] = append([]string(nil), palette...)
	}
	b.mu.Unlock()
	return nil
}

func sendPalette(device string, palette []string, speed string, brightness int) (bool, error) {
	conn, err := dialLightingDaemon()
	if err != nil {
		return false, nil
	}
	defer func() { _ = conn.Close() }()
	request := struct {
		Cmd        string   `json:"cmd"`
		Device     string   `json:"device,omitempty"`
		Palette    []string `json:"palette"`
		Speed      string   `json:"speed"`
		Brightness int      `json:"brightness"`
	}{
		Cmd: "palette", Device: device, Palette: palette,
		Speed: speed, Brightness: brightness,
	}
	data, err := json.Marshal(request)
	if err != nil {
		return true, err
	}
	if _, err := fmt.Fprintf(conn, "%s\n", data); err != nil {
		return true, err
	}
	var response struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return true, fmt.Errorf("no response from daemon")
	}
	if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
		return true, err
	}
	if !response.OK {
		return true, fmt.Errorf("%s", response.Error)
	}
	return true, nil
}

// LightingOff disables lighting on a device ("" = all zones).
func (b *Backend) LightingOff(device string) error {
	if b.live() {
		_, err := api.SendOff(device)
		return err
	}
	b.mu.Lock()
	if device == "" {
		b.mock.Lighting.Enabled = false
		b.mock.Devices = nil
	} else {
		ls := b.mock.Lighting
		if existing, ok := b.mock.Devices[device]; ok {
			ls = existing
		}
		ls.Enabled = false
		if b.mock.Devices == nil {
			b.mock.Devices = make(map[string]api.LightingState)
		}
		b.mock.Devices[device] = ls
	}
	b.mu.Unlock()
	return nil
}

// SetBrightness sets lighting brightness (0–3) on a device ("" = all zones).
func (b *Backend) SetBrightness(device string, level int) error {
	if b.live() {
		_, err := api.SendBrightness(device, level)
		return err
	}
	b.mu.Lock()
	if device == "" {
		b.mock.Lighting.Brightness = level
		b.mock.Lighting.Enabled = level > 0
	} else {
		ls := b.mock.Lighting
		if existing, ok := b.mock.Devices[device]; ok {
			ls = existing
		}
		ls.Brightness = level
		ls.Enabled = level > 0
		if b.mock.Devices == nil {
			b.mock.Devices = make(map[string]api.LightingState)
		}
		b.mock.Devices[device] = ls
	}
	b.mu.Unlock()
	return nil
}

func (b *Backend) setMockLighting(device string, ls api.LightingState) {
	if device == "" {
		b.mock.Lighting = ls
		b.mock.Devices = nil
		return
	}
	if b.mock.Devices == nil {
		b.mock.Devices = make(map[string]api.LightingState)
	}
	b.mock.Devices[device] = ls
}

func clonePalettes(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for device, palette := range in {
		out[device] = append([]string(nil), palette...)
	}
	return out
}

// Subscribe forwards the daemon event stream (e.g. "gui-toggle"). Returns a nil
// channel when the daemon is offline; callers must handle that.
func (b *Backend) Subscribe(events []string) (<-chan string, func(), error) {
	return api.Subscribe(events)
}

// defaultMockState returns an idle-looking GZ302EA state for offline preview.
func defaultMockState() api.State {
	fc := defaultFanCurve()
	return api.State{
		Lighting: api.LightingState{
			Enabled: true, Mode: "static", Color: "00E5FF",
			Color2: "FF0066", Speed: "normal", Brightness: 3,
		},
		Profile:            "balanced",
		Battery:            80,
		BootSound:          0,
		PanelOverdrive:     1,
		Temperature:        46,
		FanRPM:             1850,
		UndervoltAvailable: false, // ryzen_smu not loaded on this machine
		TDP:                &api.TDPState{PL1SPL: 45, PL2SPPT: 54, FPPT: 65},
		FanCurve:           &api.FanCurveState{Mode: 2, Points: fc[:]},
		Undervolt:          &api.UndervoltState{},
	}
}
