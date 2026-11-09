package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestSendResultOnlyUpdatesRecipient(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newModel()
	m.state = stateChat
	// two contacts
	m.contacts = []Contact{{Number: "A"}, {Number: "B"}}
	m.selected = 0
	if m.history == nil {
		m.history = make(map[string][]Message)
	}
	tA := time.Unix(1000, 0)
	tB := time.Unix(2000, 0)
	m.history["A"] = []Message{{IsMe: true, Time: tA, Sent: false}}
	m.history["B"] = []Message{{IsMe: true, Time: tB, Sent: true}}
	m.messages = m.history["A"]

	newTs := int64(9999000)
	nm, _ := m.Update(sendResultMsg{timestamp: newTs, recipient: "A"})
	mm := nm.(model)

	if !mm.history["A"][0].Sent {
		t.Fatalf("A not marked Sent")
	}
	if mm.history["A"][0].Time.UnixMilli() != newTs {
		t.Fatalf("A timestamp not updated: got %d want %d", mm.history["A"][0].Time.UnixMilli(), newTs)
	}
	if mm.history["B"][0].Time != tB {
		t.Fatalf("B timestamp changed: got %v want %v", mm.history["B"][0].Time, tB)
	}
	if !mm.history["B"][0].Sent {
		t.Fatalf("B Sent changed")
	}

	histPath := filepath.Join(os.Getenv("HOME"), ".config", "signal-tui", "history.json")
	data, err := os.ReadFile(histPath)
	if err != nil {
		t.Fatalf("history file not written: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("invalid history json: %v", err)
	}
	if len(raw) != 2 {
		t.Fatalf("history has %d keys, want 2: %v", len(raw), raw)
	}
	for _, k := range []string{"A", "B"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("history missing key %q", k)
		}
	}
	_ = tea.Msg(nil)
}
