// rendercheck dumps breadcrumb + statusbar variants to stdout so you can
// eyeball styles without launching a TUI.
package main

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/breadcrumb"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/statusbar"
)

type variant struct {
	name   string
	header breadcrumb.Model
	bar    statusbar.Model
}

func makeBar(barBG, barFG lipgloss.TerminalColor, keyFG lipgloss.TerminalColor) statusbar.Model {
	h := help.New(help.Options{
		KeyStyle:  lipgloss.NewStyle().Bold(true).Foreground(keyFG).Background(barBG),
		DescStyle: lipgloss.NewStyle().Foreground(barFG).Background(barBG),
	})
	h.SetBindings([]key.Binding{
		key.NewBinding(key.WithKeys("↑/↓"), key.WithHelp("↑/↓", "move")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	})
	return statusbar.New(statusbar.Options{
		Width:         80,
		BarBackground: barBG,
		BarForeground: barFG,
		Left:          h.ShortView(),
		Right:         "v0.1.0",
	})
}

func main() {
	crumbs := []string{"Regions", "North America", "San Francisco"}

	variants := []variant{
		{
			name: "1. Shared bg — header & footer match (the default, usually best)",
			header: breadcrumb.New(breadcrumb.Options{
				Width:         80,
				Crumbs:        crumbs,
				BarBackground: lipgloss.Color("236"),
				BarForeground: lipgloss.Color("252"),
			}),
			bar: makeBar(lipgloss.Color("236"), lipgloss.Color("252"), lipgloss.Color("75")),
		},
		{
			name: "2. Accent header — colored top strip over a neutral footer",
			header: breadcrumb.New(breadcrumb.Options{
				Width:         80,
				Crumbs:        crumbs,
				BarBackground: lipgloss.Color("24"), // teal-blue
				BarForeground: lipgloss.Color("195"),
			}),
			bar: makeBar(lipgloss.Color("236"), lipgloss.Color("252"), lipgloss.Color("75")),
		},
		{
			name: "3. Minimal — no background on either bar; pure fg",
			header: func() breadcrumb.Model {
				bar := lipgloss.NewStyle().Padding(0, 1)
				crumb := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
				current := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
				sep := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
				return breadcrumb.New(breadcrumb.Options{
					Width:          80,
					Crumbs:         crumbs,
					BarStyle:       &bar,
					CrumbStyle:     &crumb,
					CurrentStyle:   &current,
					SeparatorStyle: &sep,
				})
			}(),
			bar: func() statusbar.Model {
				left := lipgloss.NewStyle().Padding(0, 1)
				right := lipgloss.NewStyle().Padding(0, 1)
				neutral := lipgloss.NewStyle().Padding(0, 1)
				h := help.New(help.Options{
					KeyStyle:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75")),
					DescStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("250")),
				})
				h.SetBindings([]key.Binding{
					key.NewBinding(key.WithKeys("↑/↓"), key.WithHelp("↑/↓", "move")),
					key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
					key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
					key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
				})
				return statusbar.New(statusbar.Options{
					Width:        80,
					Left:         h.ShortView(),
					Right:        "v0.1.0",
					LeftStyle:    &left,
					RightStyle:   &right,
					NeutralStyle: &neutral,
				})
			}(),
		},
		{
			name: "4. Inverted — light header, dark footer",
			header: breadcrumb.New(breadcrumb.Options{
				Width:         80,
				Crumbs:        crumbs,
				BarBackground: lipgloss.Color("254"),
				BarForeground: lipgloss.Color("236"),
				CrumbStyle: func() *lipgloss.Style {
					s := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Background(lipgloss.Color("254"))
					return &s
				}(),
				CurrentStyle: func() *lipgloss.Style {
					s := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("236")).Background(lipgloss.Color("254"))
					return &s
				}(),
				SeparatorStyle: func() *lipgloss.Style {
					s := lipgloss.NewStyle().Foreground(lipgloss.Color("248")).Background(lipgloss.Color("254"))
					return &s
				}(),
			}),
			bar: makeBar(lipgloss.Color("236"), lipgloss.Color("252"), lipgloss.Color("75")),
		},
	}

	for _, v := range variants {
		fmt.Printf("--- %s ---\n", v.name)
		fmt.Println(v.header.View())
		fmt.Println("  <body renders here>")
		fmt.Println(v.bar.View())
		fmt.Println()
	}
}
