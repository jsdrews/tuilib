package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/internal/cmdtest"
	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/menu"
	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/surface"
	"github.com/jsdrews/tuilib/pkg/theme"
)

var update = flag.Bool("update", false, "rewrite the golden frames from the current output")

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

// TestOutlinedFramesAreByteIdentical holds the promise every surface decision
// leans on (docs/surfaces.md): under Outlined, an app looks exactly as it did
// before surfaces existed. Each launcher entry is rendered through the full
// app shell and compared byte for byte, escapes included, against the frame
// captured before the surface work began.
//
// Only the first frame is compared, with no commands run: commands start
// timers, subprocesses and fetches, and the frame after them depends on when
// they land. The first frame is what every entry draws deterministically.
//
// Regenerate with `go test ./examples/launcher -run Outlined -update` only
// when a change to Outlined output is intended, and say so in the commit.
func TestOutlinedFramesAreByteIdentical(t *testing.T) {
	checkEntries(t, "outlined", outlinedFrame)
}

// TestOutlinedOverlayFramesAreByteIdentical does the same with an overlay up:
// the help overlay, the actions menu, an alert and a confirm. Each is composited
// over the screen by layout.ZStack, so these frames are what hold Outlined
// modals byte-identical when compositing changes.
func TestOutlinedOverlayFramesAreByteIdentical(t *testing.T) {
	checkOverlays(t, "outlined", outlinedFrame)
}

// TestOutlinedTabFramesAreByteIdentical holds each tab of a tabbed showcase
// to its first frame, as every entry's first frame is held above.
func TestOutlinedTabFramesAreByteIdentical(t *testing.T) {
	checkTabs(t, "outlined", outlinedFrame)
}

// The Filled goldens hold the default look: the same frames, built the way
// an app that names no surface is built. Regenerate with
// `go test ./examples/launcher -run Filled -update` when a change to Filled
// output is intended.
func TestFilledFramesAreByteIdentical(t *testing.T) {
	requireFilledDefault(t)
	checkEntries(t, "filled", defaultFrame)
}

func TestFilledOverlayFramesAreByteIdentical(t *testing.T) {
	requireFilledDefault(t)
	checkOverlays(t, "filled", defaultFrame)
}

func TestFilledTabFramesAreByteIdentical(t *testing.T) {
	requireFilledDefault(t)
	checkTabs(t, "filled", defaultFrame)
}

// requireFilledDefault stops the Filled goldens from silently holding some
// other surface once the default changes.
func requireFilledDefault(t *testing.T) {
	t.Helper()
	if got := theme.New(palette.Dark()).Surface.Name(); got != surface.Filled.Name {
		t.Fatalf("theme.New draws %s; testdata/filled holds the default and expects %s", got, surface.Filled.Name)
	}
}

type frameFunc func(e entry, keys ...tea.KeyMsg) string

func checkEntries(t *testing.T, dir string, frame frameFunc) {
	for _, e := range entries {
		t.Run(e.name, func(t *testing.T) {
			compareGolden(t, filepath.Join("testdata", dir, goldenName(e)+".golden"), frame(e))
		})
	}
}

func checkOverlays(t *testing.T, dir string, frame frameFunc) {
	back, enter := tea.KeyMsg{Type: tea.KeyShiftTab}, tea.KeyMsg{Type: tea.KeyEnter}
	for _, sc := range []struct {
		name, dir string
		keys      []tea.KeyMsg
	}{
		{"help", "workbench", []tea.KeyMsg{runes("?")}},
		{"actions", "workbench", []tea.KeyMsg{runes("a")}},
		// shift+tab wraps focus to the operations menu.
		{"alert", "workbench", []tea.KeyMsg{back, runes("G"), enter}},
		{"confirm", "workbench", []tea.KeyMsg{back, runes("]"), enter}},
	} {
		t.Run(sc.name, func(t *testing.T) {
			e, ok := entryIn(sc.dir)
			if !ok {
				t.Fatalf("no launcher entry in %s", sc.dir)
			}
			compareGolden(t, filepath.Join("testdata", dir, "overlay_"+sc.name+".golden"), frame(e, sc.keys...))
		})
	}
}

func checkTabs(t *testing.T, dir string, frame frameFunc) {
	for _, sc := range []struct{ name, dir, key string }{
		{"events", "remotedata", "2"},
		{"finished", "remotedata", "3"},
		{"anchored", "remotedata", "4"},
		{"podlogs", "remotedata", "5"},
		{"form", "appearance", "2"},
		{"text", "appearance", "3"},
		{"panes", "appearance", "4"},
		{"layouts", "appearance", "5"},
	} {
		t.Run(sc.dir+"/"+sc.name, func(t *testing.T) {
			e, ok := entryIn(sc.dir)
			if !ok {
				t.Fatalf("no launcher entry in %s", sc.dir)
			}
			compareGolden(t, filepath.Join("testdata", dir, sc.dir+"_"+sc.name+".golden"), frame(e, runes(sc.key)))
		})
	}
}

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func entryIn(dir string) (entry, bool) {
	for _, e := range entries {
		if e.dir == dir {
			return e, true
		}
	}
	return entry{}, false
}

func compareGolden(t *testing.T, path, got string) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no golden frame (run with -update to capture one): %v", err)
	}
	if got != string(want) {
		t.Errorf("frame differs from %s\n%s", path, firstDiff(string(want), got))
	}
}

// outlinedFrame renders e inside the app shell, configured as the launcher
// configures it but pinned to Outlined, after keys.
func outlinedFrame(e entry, keys ...tea.KeyMsg) string {
	th := theme.Compose(palette.Dark(), surface.Outlined, surface.Caps{Profile: termenv.TrueColor, BlockGlyphs: true})
	return shellFrame(e.build(th), []surface.Surface{surface.Outlined}, keys)
}

// defaultFrame renders e the way an app that names no surface draws it.
func defaultFrame(e entry, keys ...tea.KeyMsg) string {
	return shellFrame(e.build(theme.New(palette.Dark())), nil, keys)
}

func shellFrame(root screen.Screen, surfaces []surface.Surface, keys []tea.KeyMsg) string {
	// Capabilities are fixed so the frame doesn't depend on the terminal the
	// test runs in: a filled surface falls back without colour.
	caps := surface.Caps{Profile: termenv.TrueColor, BlockGlyphs: true}
	m := app.New(app.Options{
		Root:         root,
		Palettes:     []palette.Palette{palette.Dark()},
		Surfaces:     surfaces,
		Capabilities: &caps,
		SkipConfig:   true,
		Version:      "examples",
		ThemeKey:     key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
		Mouse:        app.MouseClick,
		ActionsKey:   key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "actions")),
		OutputKey:    key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "output")),
	})
	m.Init()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	for _, k := range keys {
		var cmd tea.Cmd
		next, cmd = next.Update(k)
		// A menu reports a choice by command. Deliver that and nothing else:
		// the rest are timers, subprocesses and fetches whose timing would
		// make the frame nondeterministic.
		for _, msg := range cmdtest.Run(cmd) {
			if _, chosen := msg.(menu.ChosenMsg); chosen {
				next, _ = next.Update(msg)
			}
		}
	}
	return next.View()
}

// goldenName is the entry's directory, plus its name when two entries share
// one (the eventlog demo has a live and a finished variant).
func goldenName(e entry) string {
	name := strings.ReplaceAll(e.dir, "/", "_")
	shared := 0
	for _, o := range entries {
		if o.dir == e.dir {
			shared++
		}
	}
	if shared > 1 {
		name += "_" + strings.ToLower(strings.NewReplacer(" ", "-", "(", "", ")", "").Replace(e.name))
	}
	return name
}

// firstDiff names the first line that differs, both versions quoted so the
// escapes are visible.
func firstDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < max(len(w), len(g)); i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			return fmt.Sprintf("line %d:\nwant %q\n got %q", i+1, wl, gl)
		}
	}
	return "lengths differ"
}
