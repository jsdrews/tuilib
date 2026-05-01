// Package confirm demonstrates pkg/confirm in the standard modal pattern:
// a list of files, press d to delete; a confirm dialog overlays the list
// via layout.ZStack(base, layout.Center(...)). The parent screen owns
// show/hide state and gates IsCapturingKeys; the modal owns input while up.
//
// Result handling is fully bubbletea-idiomatic — the modal returns a
// confirm.ConfirmedMsg or confirm.CancelledMsg via tea.Cmd, the parent
// matches in its own Update, dismisses the modal, and reports the outcome
// to the statusbar via app.Info.
package confirm

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/app"
	tconfirm "github.com/jsdrews/tuilib/pkg/confirm"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// New returns the confirm demo's root screen.
func New(t theme.Theme) screen.Screen {
	s := &confirmScreen{
		files: []string{
			"users.go", "users_test.go",
			"auth.go", "auth_test.go",
			"router.go", "config.yaml",
			"README.md", "TODO.md",
		},
	}
	s.SetTheme(t)
	return s
}

type confirmScreen struct {
	t       theme.Theme
	files   []string
	list    list.Model
	modal   tconfirm.Model
	pending string // file targeted by the open modal
	modalUp bool
}

func (s *confirmScreen) Title() string         { return "Confirm" }
func (s *confirmScreen) Init() tea.Cmd         { return nil }
func (s *confirmScreen) OnEnter(any) tea.Cmd   { return nil }
func (s *confirmScreen) IsCapturingKeys() bool { return s.modalUp || s.list.Filtering() }

func (s *confirmScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	// Result messages from the modal — handle these before forwarding so the
	// dismissed modal doesn't see a stale follow-up key.
	switch msg.(type) {
	case tconfirm.ConfirmedMsg:
		target := s.pending
		s.dismissModal()
		s.removeFile(target)
		return s, app.Info("Deleted " + target + ".")
	case tconfirm.CancelledMsg:
		target := s.pending
		s.dismissModal()
		return s, app.Info("Kept " + target + ".")
	}

	if s.modalUp {
		var cmd tea.Cmd
		s.modal, cmd = s.modal.Update(msg)
		return s, cmd
	}

	if k, ok := msg.(tea.KeyMsg); ok && !s.list.Filtering() {
		if k.String() == "d" {
			if file, ok := s.list.Selected(); ok {
				s.openModalFor(file)
				return s, nil
			}
		}
	}

	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

func (s *confirmScreen) Layout() layout.Node {
	body := layout.Sized(&s.list)
	if !s.modalUp {
		return body
	}
	return layout.ZStack(body, layout.Center(48, 7, layout.Sized(&s.modal)))
}

func (s *confirmScreen) Help() []key.Binding {
	if s.modalUp {
		return s.modal.Help()
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "move")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
}

func (s *confirmScreen) SetTheme(t theme.Theme) {
	s.t = t

	cursor, value := s.list.Cursor(), s.list.Value()
	lOpts := t.List()
	lOpts.Title = "files"
	lOpts.Filterable = true
	lOpts.Filter.Placeholder = "filter…"
	lOpts.Items = s.files
	s.list = list.New(lOpts)
	if value != "" {
		s.list.SetValue(value)
	}
	s.list.SetCursor(cursor)

	cOpts := t.Confirm()
	cOpts.Title = "confirm delete"
	cOpts.Confirm = "Delete"
	cOpts.Cancel = "Keep"
	if s.modalUp {
		cOpts.Message = "Permanently delete " + s.pending + "?"
		cOpts.Initial = s.modal.Value()
	}
	s.modal = tconfirm.New(cOpts)
}

func (s *confirmScreen) openModalFor(file string) {
	s.pending = file
	s.modalUp = true

	cOpts := s.t.Confirm()
	cOpts.Title = "confirm delete"
	cOpts.Confirm = "Delete"
	cOpts.Cancel = "Keep"
	cOpts.Message = "Permanently delete " + file + "?"
	s.modal = tconfirm.New(cOpts)
}

func (s *confirmScreen) dismissModal() {
	s.modalUp = false
}

func (s *confirmScreen) removeFile(name string) {
	out := s.files[:0]
	for _, f := range s.files {
		if f != name {
			out = append(out, f)
		}
	}
	s.files = out
	cursor := s.list.Cursor()
	if cursor >= len(s.files) && cursor > 0 {
		cursor = len(s.files) - 1
	}
	lOpts := s.t.List()
	lOpts.Title = "files"
	lOpts.Filterable = true
	lOpts.Filter.Placeholder = "filter…"
	lOpts.Items = s.files
	s.list = list.New(lOpts)
	s.list.SetCursor(cursor)
}
