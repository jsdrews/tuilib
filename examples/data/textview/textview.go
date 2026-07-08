// Package textview demonstrates pkg/textview: a read-only viewer for
// static text content with search, wrap toggle, and vim-style navigation.
//
// Two synthetic documents cycle via `d` — a rendered README and a git
// diff. `/` focuses the search bar, `n`/`N` step matches, `w` toggles
// word-wrap, `g`/`G` and `ctrl+u/d` scroll. The bottom-left slot shows
// the current match position.
package textview

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/screen"
	ttextview "github.com/jsdrews/tuilib/pkg/textview"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// New returns the textview demo's root screen.
func New(t theme.Theme) screen.Screen {
	s := &textviewScreen{docIdx: 0}
	s.SetTheme(t)
	return s
}

type textviewScreen struct {
	t      theme.Theme
	view   ttextview.Model
	docIdx int
}

var docs = []struct {
	title, body string
}{
	{
		"README.md",
		`# tuilib

tuilib is a set of Bubble Tea components for building TUIs where the
non-domain work — layout math, theme swap, focus routing, statusbar
messages — is handled by the framework so screens can focus on data.

## What it gives you

- pkg/app: the root shell (breadcrumb, statusbar, esc-pop, theme cycle).
- pkg/layout: declarative VStack/HStack/ZStack layout — no "m.h - 2" math.
- pkg/list, pkg/table, pkg/tree, pkg/inspector: cursor-driven viewers.
- pkg/logview: streaming log tail with search and follow.
- pkg/textview: static text viewer with search + wrap.
- pkg/form, pkg/input, pkg/toggle, pkg/filter: bordered inputs.
- pkg/confirm, pkg/alert: modal dialogs with typed result messages.
- pkg/poll: interval ticker for auto-refresh screens.
- pkg/runner: run an editor / pager / TUI subprocess and get a Result back.
- pkg/theme: one palette, many components, live cycling.

## Getting started

    go get github.com/jsdrews/tuilib

Then compose a screen inside the app shell — see examples/app/stack for
the canonical "one master list pushing a detail screen" shape, or
examples/launcher for how the examples themselves are wired.

## Design principles

1. Data-first components — pass a slice, get a view; no template layer.
2. Layout is declarative — Fixed/Flex siblings, not hand-written math.
3. Themes are palettes, not per-component overrides — swap live.
4. Every scrollable component uses the same arrow/hjkl bindings (rule 23).
5. Focus routes automatically — the shell owns the shape, screens fill it.

Read the docs at pkg.go.dev/github.com/jsdrews/tuilib.
`,
	},
	{
		"deploy.patch",
		`diff --git a/services/api/handlers/users.go b/services/api/handlers/users.go
index 4a8e21c..b3f5d1a 100644
--- a/services/api/handlers/users.go
+++ b/services/api/handlers/users.go
@@ -12,6 +12,8 @@ import (
 	"database/sql"
 	"encoding/json"
 	"net/http"
+	"strings"
+	"time"

 	"github.com/pkg/errors"

@@ -34,7 +36,15 @@ func (h *UsersHandler) List(w http.ResponseWriter, r *http.Request) {
 		return
 	}

-	rows, err := h.db.Query("SELECT id, email, created_at FROM users")
+	q := "SELECT id, email, created_at FROM users"
+	if search := strings.TrimSpace(r.URL.Query().Get("q")); search != "" {
+		q += " WHERE email ILIKE $1"
+	}
+
+	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
+	defer cancel()
+
+	rows, err := h.db.QueryContext(ctx, q)
 	if err != nil {
 		http.Error(w, err.Error(), http.StatusInternalServerError)
 		return
diff --git a/services/api/handlers/sessions.go b/services/api/handlers/sessions.go
index 9c2e444..f8a1b09 100644
--- a/services/api/handlers/sessions.go
+++ b/services/api/handlers/sessions.go
@@ -8,6 +8,7 @@ import (
 	"encoding/json"
 	"net/http"
 	"time"
+	"context"

 	"github.com/pkg/errors"

@@ -45,6 +46,9 @@ func (h *SessionsHandler) Create(w http.ResponseWriter, r *http.Request) {
 		return
 	}

+	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
+	defer cancel()
+
 	tx, err := h.db.BeginTx(ctx, nil)
 	if err != nil {
 		http.Error(w, err.Error(), http.StatusInternalServerError)
`,
	},
}

func (s *textviewScreen) Title() string         { return "TextView" }
func (s *textviewScreen) Init() tea.Cmd         { return nil }
func (s *textviewScreen) OnEnter(any) tea.Cmd   { return nil }
func (s *textviewScreen) IsCapturingKeys() bool { return s.view.Searching() }

func (s *textviewScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && !s.view.Searching() && k.String() == "d" {
		s.docIdx = (s.docIdx + 1) % len(docs)
		s.view.SetTitle(docs[s.docIdx].title)
		s.view.SetContent(docs[s.docIdx].body)
		return s, nil
	}
	var cmd tea.Cmd
	s.view, cmd = s.view.Update(msg)
	return s, cmd
}

func (s *textviewScreen) Layout() layout.Node {
	return layout.Sized(&s.view)
}

func (s *textviewScreen) Help() []key.Binding {
	out := s.view.Help()
	if !s.view.Searching() {
		out = append(out, key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "next doc")))
	}
	return out
}

func (s *textviewScreen) SetTheme(t theme.Theme) {
	s.t = t

	query := s.view.Query()
	wrap := s.view.Wrap()

	tvOpts := t.TextView()
	tvOpts.Title = docs[s.docIdx].title
	tvOpts.Content = docs[s.docIdx].body
	tvOpts.Searchable = true
	tvOpts.Filter.Placeholder = "search…"
	tvOpts.Wrap = wrap || tvOpts.Wrap
	s.view = ttextview.New(tvOpts)
	if query != "" {
		s.view.SetQuery(query)
	}
}
