package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"strings"
	"testing"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// TestEveryLauncherEntryConstructsAndRenders drives every demo the launcher
// lists. `task examples` is the only way most of these are ever run, so a
// demo that constructs but renders nothing — or panics on Init — otherwise
// stays broken until someone happens to open it. It does not prove a demo
// *behaves*: the focus example rendered perfectly while its tab cycling did
// nothing, which is what got it deleted.
func TestEveryLauncherEntryConstructsAndRenders(t *testing.T) {
	if len(entries) == 0 {
		t.Fatal("launcher has no entries")
	}
	for _, e := range entries {
		t.Run(e.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked: %v", r)
				}
			}()
			s := e.build(theme.Dark())
			if s == nil {
				t.Fatal("New returned nil")
			}
			if cmd := s.Init(); cmd != nil {
				cmd()
			}
			view := s.Layout().Render(geom.New(0, 0, 80, 24))
			if strings.TrimSpace(view) == "" {
				t.Error("rendered nothing")
			}
		})
	}
}

// TestDriveEveryEntry goes further than rendering: it sends the keys and the
// click a person tries first, so a demo that panics in Update is caught here
// rather than in the launcher. Returned tea.Cmds are deliberately not run —
// some of them spawn subprocesses.
func TestDriveEveryEntry(t *testing.T) {
	keys := []tea.KeyMsg{
		{Type: tea.KeyDown}, {Type: tea.KeyDown}, {Type: tea.KeyTab},
		{Type: tea.KeyRunes, Runes: []rune{'/'}},
		{Type: tea.KeyRunes, Runes: []rune{'a'}},
		{Type: tea.KeyEsc}, {Type: tea.KeyUp},
		{Type: tea.KeyRunes, Runes: []rune{' '}},
		{Type: tea.KeyShiftTab}, {Type: tea.KeyEnter},
	}
	for _, e := range entries {
		t.Run(e.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked: %v", r)
				}
			}()
			s := e.build(theme.Dark())
			s.Init()
			r := geom.New(0, 0, 90, 26)
			s.Layout().Render(r)
			for _, k := range keys {
				s, _ = s.Update(k)
				s.Layout().Render(r)
			}
			s, _ = s.Update(mouse.Msg{
				MouseMsg: tea.MouseMsg{X: 10, Y: 4, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft},
				Clicks:   1,
			})
			if v := s.Layout().Render(r); strings.TrimSpace(v) == "" {
				t.Error("rendered nothing after interaction")
			}
			_ = s.Help()
			_ = s.IsCapturingKeys()
			_ = s.Title()
		})
	}
}
