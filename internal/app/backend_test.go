package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"testing"
)

func TestLoadExtendedState(t *testing.T) {
	requests := serveOneDaemonRequest(t, func(request map[string]any) string {
		if request["cmd"] != "get-state" {
			t.Errorf("cmd = %v, want get-state", request["cmd"])
		}
		return `{"ok":true,"state":{"lighting":{"enabled":true,"mode":"palette","color":"FF0000","color2":"00FF00","palette":["FF0000","00FF00"],"speed":"fast","brightness":2},"capabilities":["palette-cycle"]}}`
	})

	ok, snapshot, err := loadExtendedState()
	if err != nil || !ok {
		t.Fatalf("loadExtendedState = %v, %v", ok, err)
	}
	if snapshot.Lighting.Mode != "palette" || snapshot.Lighting.Brightness != 2 {
		t.Fatalf("lighting = %+v", snapshot.Lighting)
	}
	if !hasCapability(snapshot.Capabilities, paletteCapability) {
		t.Fatalf("capabilities = %v", snapshot.Capabilities)
	}
	if got := snapshot.Palettes[""]; len(got) != 2 || got[1] != "00FF00" {
		t.Fatalf("palette = %v", got)
	}
	if err := <-requests; err != nil {
		t.Fatal(err)
	}
}

func TestSendPalette(t *testing.T) {
	requests := serveOneDaemonRequest(t, func(request map[string]any) string {
		if request["cmd"] != "palette" {
			t.Errorf("cmd = %v, want palette", request["cmd"])
		}
		if request["device"] != "keyboard" || request["speed"] != "slow" {
			t.Errorf("request = %#v", request)
		}
		palette, ok := request["palette"].([]any)
		if !ok || len(palette) != 2 || palette[0] != "123456" {
			t.Errorf("palette = %#v", request["palette"])
		}
		return `{"ok":true}`
	})

	handled, err := sendPalette("keyboard", []string{"123456", "ABCDEF"}, "slow", 3)
	if err != nil || !handled {
		t.Fatalf("sendPalette = %v, %v", handled, err)
	}
	if err := <-requests; err != nil {
		t.Fatal(err)
	}
}

func TestOfflineMockPalette(t *testing.T) {
	originalDial := dialLightingDaemon
	originalLive := lightingDaemonLive
	dialLightingDaemon = func() (net.Conn, error) {
		return nil, fmt.Errorf("offline")
	}
	lightingDaemonLive = func() bool { return false }
	t.Cleanup(func() {
		dialLightingDaemon = originalDial
		lightingDaemonLive = originalLive
	})
	backend := NewBackend()
	want := []string{"112233", "445566", "778899"}
	if err := backend.ApplyPalette("keyboard", want, "normal", 2); err != nil {
		t.Fatal(err)
	}
	snapshot := backend.Load()
	if snapshot.Connected {
		t.Fatal("offline mock unexpectedly connected")
	}
	if got := lightingForDevice(snapshot, "keyboard"); got.Mode != "palette" {
		t.Fatalf("mode = %q", got.Mode)
	}
	if got := paletteForDevice(snapshot, "keyboard"); len(got) != 3 || got[2] != want[2] {
		t.Fatalf("palette = %v", got)
	}
}

func serveOneDaemonRequest(t *testing.T, respond func(map[string]any) string) <-chan error {
	t.Helper()
	client, server := net.Pipe()
	originalDial := dialLightingDaemon
	used := false
	dialLightingDaemon = func() (net.Conn, error) {
		if used {
			return nil, fmt.Errorf("test daemon connection already used")
		}
		used = true
		return client, nil
	}
	t.Cleanup(func() {
		dialLightingDaemon = originalDial
		_ = client.Close()
		_ = server.Close()
	})

	done := make(chan error, 1)
	go func() {
		defer func() { _ = server.Close() }()
		scanner := bufio.NewScanner(server)
		if !scanner.Scan() {
			done <- fmt.Errorf("request missing")
			return
		}
		var request map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			done <- err
			return
		}
		_, err := fmt.Fprintln(server, respond(request))
		done <- err
	}()
	return done
}
