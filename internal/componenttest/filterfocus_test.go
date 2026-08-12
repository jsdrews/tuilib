// Package componenttest holds assertions that must hold identically across
// every component, run once against all of them.
//
// It exists because of a bug it would have caught: the "clicking the body
// hands input back from the filter" behaviour was added to pkg/list by hand
// and rolled out to the other five components by a script that omitted it.
// The only test for it lived in pkg/list — the one component already fixed —
// so five components shipped without the behaviour and nothing failed.
//
// Anything that is supposed to be true of *all* filterable components belongs
// here rather than in one component's package, so a component that skips the
// wiring fails immediately instead of waiting to be noticed in the app.
package componenttest

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/pkg/filter"
	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/inspector"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/logview"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/textview"
	"github.com/jsdrews/tuilib/pkg/tree"
)

// TestMain forces a colour profile. Without a TTY lipgloss falls back to the
// Ascii profile and strips every style, so an assertion about focus colouring
// would pass or fail for reasons unrelated to the code under test.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

// placed returns rect stamped with the current render generation. The
// literal below carries generation zero, and Rect.Hit rejects any rect from
// an earlier frame — so a harness built from it silently declines every
// event once any other test in this package has advanced the counter.
func placed() geom.Rect { return geom.New(rect.X, rect.Y, rect.W, rect.H) }

// rect is where every component under test is placed. With a 2-row inline
// filter header the interior is: y+1 filter, y+2 rule, y+3.. content.
var rect = geom.Rect{X: 0, Y: 0, W: 44, H: 14}

const (
	filterRowY  = 1
	ruleY       = 2
	firstRowY   = 3
	deepRowY    = 9  // well inside the content, past any items
	bottomEdgeY = 13 // the pane's bottom border
	bodyX       = 4

	// The vertical scrollbar is the column right of the viewport:
	// ContentRect.X (rect.X+1) + viewport width (rect.W-2-ScrollbarWidth).
	scrollbarX = 1 + (44 - 2 - 1)
)

// Components are built with explicit active/inactive colours because that is
// what every theme builder supplies — the focus cue is carried by those
// tokens, so a colourless component genuinely has nothing to show.
var (
	active     = lipgloss.Color("12")
	inactive   = lipgloss.Color("240")
	filterOpts = filter.Options{
		ActiveColor:   active,
		InactiveColor: inactive,
		PromptStyle:   lipgloss.NewStyle().Bold(true),
	}
)

// harness adapts one component to the shared assertions. Each component has
// its own concrete Model, so the closures do the type-specific part and the
// tests below stay written once.
type harness struct {
	name          string
	update        func(tea.Msg)
	updateCmd     func(tea.Msg) tea.Cmd
	filterFocused func() bool
	view          func() string
	focus         func()
	focusFilter   func()
	setRect       func()
}

var itemCount = 40

func items() []string {
	out := make([]string, itemCount)
	for i := range out {
		out[i] = strings.Repeat("item ", 1) + string(rune('a'+i%26))
	}
	return out
}

// node is a minimal tree.Node.
type node struct {
	label string
	kids  []tree.Node
}

func (n node) Label() string         { return n.label }
func (n node) Children() []tree.Node { return n.kids }

func harnesses() []harness {
	var hs []harness

	{
		m := list.New(list.Options{Items: items(), Filterable: true,
			ActiveColor: active, InactiveColor: inactive, Filter: filterOpts})
		m.SetRect(placed())
		hs = append(hs, harness{
			name:          "list",
			update:        func(msg tea.Msg) { m, _ = m.Update(msg) },
			updateCmd:     func(msg tea.Msg) tea.Cmd { var c tea.Cmd; m, c = m.Update(msg); return c },
			filterFocused: func() bool { return m.IsCapturingKeys() },
			view:          func() string { return m.View() },
			focus:         func() { m.Focus() },
			focusFilter:   func() { m.FocusFilter() },
			setRect:       func() { m.SetRect(placed()) },
		})
	}
	{
		rows := make([]table.Row, itemCount)
		for i := range rows {
			rows[i] = table.Row{string(rune('a' + i%26)), "eu-west-1"}
		}
		m := table.New(table.Options{
			Columns:     []table.Column{{Title: "Name", Flex: 2}, {Title: "Region", Width: 12}},
			Rows:        rows,
			Filterable:  true,
			ActiveColor: active, InactiveColor: inactive, Filter: filterOpts,
		})
		m.SetRect(placed())
		hs = append(hs, harness{
			name:          "table",
			update:        func(msg tea.Msg) { m, _ = m.Update(msg) },
			updateCmd:     func(msg tea.Msg) tea.Cmd { var c tea.Cmd; m, c = m.Update(msg); return c },
			filterFocused: func() bool { return m.IsCapturingKeys() },
			view:          func() string { return m.View() },
			focus:         func() { m.Focus() },
			focusFilter:   func() { m.FocusFilter() },
			setRect:       func() { m.SetRect(placed()) },
		})
	}
	{
		kids := make([]tree.Node, itemCount)
		for i := range kids {
			kids[i] = node{label: "child " + string(rune('a'+i%26))}
		}
		m := tree.New(tree.Options{
			Root: node{label: "root", kids: kids}, Searchable: true, InitialDepth: 2,
			ActiveColor: active, InactiveColor: inactive, Filter: filterOpts,
		})
		m.SetRect(placed())
		hs = append(hs, harness{
			name:          "tree",
			update:        func(msg tea.Msg) { m, _ = m.Update(msg) },
			updateCmd:     func(msg tea.Msg) tea.Cmd { var c tea.Cmd; m, c = m.Update(msg); return c },
			filterFocused: func() bool { return m.IsCapturingKeys() },
			view:          func() string { return m.View() },
			focus:         func() { m.Focus() },
			focusFilter:   func() { m.FocusFilter() },
			setRect:       func() { m.SetRect(placed()) },
		})
	}
	{
		fields := make([]inspector.Field, itemCount)
		for i := range fields {
			fields[i] = inspector.Field{Label: "field", Value: "value"}
		}
		m := inspector.New(inspector.Options{Fields: fields, Filterable: true,
			ActiveColor: active, InactiveColor: inactive, Filter: filterOpts})
		m.SetRect(placed())
		hs = append(hs, harness{
			name:          "inspector",
			update:        func(msg tea.Msg) { m, _ = m.Update(msg) },
			updateCmd:     func(msg tea.Msg) tea.Cmd { var c tea.Cmd; m, c = m.Update(msg); return c },
			filterFocused: func() bool { return m.IsCapturingKeys() },
			view:          func() string { return m.View() },
			focus:         func() { m.Focus() },
			focusFilter:   func() { m.FocusFilter() },
			setRect:       func() { m.SetRect(placed()) },
		})
	}
	{
		m := logview.New(logview.Options{Searchable: true,
			ActiveColor: active, InactiveColor: inactive, Filter: filterOpts})
		m.SetRect(placed())
		m.AppendLines(items())
		hs = append(hs, harness{
			name:          "logview",
			update:        func(msg tea.Msg) { m, _ = m.Update(msg) },
			updateCmd:     func(msg tea.Msg) tea.Cmd { var c tea.Cmd; m, c = m.Update(msg); return c },
			filterFocused: func() bool { return m.IsCapturingKeys() },
			view:          func() string { return m.View() },
			focus:         func() { m.Focus() },
			focusFilter:   func() { m.FocusFilter() },
			setRect:       func() { m.SetRect(placed()) },
		})
	}
	{
		m := textview.New(textview.Options{
			Content: strings.Join(items(), "\n"), Searchable: true,
			ActiveColor: active, InactiveColor: inactive, Filter: filterOpts,
		})
		m.SetRect(placed())
		hs = append(hs, harness{
			name:          "textview",
			update:        func(msg tea.Msg) { m, _ = m.Update(msg) },
			updateCmd:     func(msg tea.Msg) tea.Cmd { var c tea.Cmd; m, c = m.Update(msg); return c },
			filterFocused: func() bool { return m.IsCapturingKeys() },
			view:          func() string { return m.View() },
			focus:         func() { m.Focus() },
			focusFilter:   func() { m.FocusFilter() },
			setRect:       func() { m.SetRect(placed()) },
		})
	}
	return hs
}

// sparse builds the same components with only a couple of rows, so most of
// the content area is blank. The dense harnesses above never exercise a click
// that lands inside the pane but on no row — which is precisely where focus
// and blur were being skipped.
func sparse() []harness {
	saved := itemCount
	itemCount = 2
	defer func() { itemCount = saved }()
	return harnesses()
}

// hasFocusRequest reports whether cmd emits a focus request, flattening
// batches the way bubbletea would.
func hasFocusRequest(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if hasFocusRequest(c) {
				return true
			}
		}
		return false
	}
	_, ok := msg.(focus.RequestMsg)
	return ok
}

func press(x, y int) mouse.Msg {
	return mouse.Msg{
		MouseMsg: tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft},
		Clicks:   1,
	}
}

// Every region of the pane except the filter row hands input back to the
// body. This is the assertion that was missing from five components.
func TestBodyClickBlursFilterEverywhere(t *testing.T) {
	regions := map[string]int{
		"rule row":            ruleY,
		"first content row":   firstRowY,
		"deep in the content": deepRowY,
		"bottom border":       bottomEdgeY,
	}

	for _, h := range harnesses() {
		for label, y := range regions {
			t.Run(h.name+"/"+label, func(t *testing.T) {
				h.focus()
				h.focusFilter()
				if !h.filterFocused() {
					t.Fatalf("setup: filter did not take input")
				}

				h.update(press(bodyX, y))

				if h.filterFocused() {
					t.Errorf("clicking the %s left input in the filter — "+
						"it keeps swallowing keys with nothing on screen saying so", label)
				}
			})
		}
	}
}

func TestFilterRowClickTakesInput(t *testing.T) {
	for _, h := range harnesses() {
		t.Run(h.name, func(t *testing.T) {
			h.focus()
			if h.filterFocused() {
				t.Fatalf("setup: filter should start blurred")
			}

			h.update(press(bodyX, filterRowY))

			if !h.filterFocused() {
				t.Errorf("clicking the filter row did not give it input")
			}
		})
	}
}

// Scrolling never claims the keyboard (rule 23), however it's expressed —
// so dragging the scrollbar has to leave a half-typed query alive.
func TestScrollbarPressLeavesFilterAlone(t *testing.T) {
	for _, h := range harnesses() {
		t.Run(h.name, func(t *testing.T) {
			h.focus()
			h.focusFilter()
			if !h.filterFocused() {
				t.Fatalf("setup: filter did not take input")
			}

			h.update(press(scrollbarX, deepRowY))

			if !h.filterFocused() {
				t.Errorf("pressing the scrollbar blurred the filter; " +
					"scrolling should not take the keyboard")
			}
		})
	}
}

// A state change nothing draws is a state change the user cannot see. This
// is the other half of the bug: the blur worked but looked identical.
func TestFocusedAndBlurredFilterRenderDifferently(t *testing.T) {
	for _, h := range harnesses() {
		t.Run(h.name, func(t *testing.T) {
			h.focus()
			blurred := h.view()

			h.focusFilter()
			focused := h.view()

			if blurred == focused {
				t.Errorf("the filter renders identically focused and blurred — " +
					"nothing on screen says where input is going")
			}
		})
	}
}

// A press on blank space inside the pane is still a press on the component:
// it must claim focus and hand input back from the filter. Clicking a row
// works and clicking the gap below it doing nothing is the kind of
// inconsistency users read as the pane being broken.
func TestBlankSpaceClickClaimsFocusAndBlursFilter(t *testing.T) {
	for _, h := range sparse() {
		t.Run(h.name, func(t *testing.T) {
			h.focus()
			h.focusFilter()
			if !h.filterFocused() {
				t.Fatalf("setup: filter did not take input")
			}

			// deepRowY is inside the content area but well past two rows.
			cmd := h.updateCmd(press(bodyX, deepRowY))

			if h.filterFocused() {
				t.Errorf("clicking blank space left input in the filter")
			}
			if !hasFocusRequest(cmd) {
				t.Errorf("clicking blank space did not claim focus for the component")
			}
		})
	}
}
