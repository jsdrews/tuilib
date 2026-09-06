package main

import (
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
