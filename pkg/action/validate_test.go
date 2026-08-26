package action

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

func errText(errs []error) string {
	var b strings.Builder
	for _, e := range errs {
		b.WriteString(e.Error())
		b.WriteString("\n")
	}
	return b.String()
}

func TestValidateAcceptsAWellFormedSet(t *testing.T) {
	if errs := Validate(threeActions()); len(errs) != 0 {
		t.Errorf("Validate reported %d errors on a valid set:\n%s", len(errs), errText(errs))
	}
}

func TestValidateRejectsNeitherRunNorDo(t *testing.T) {
	errs := Validate(Set{Actions: []Action{{Label: "Inert"}}})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "neither Run nor Do") {
		t.Errorf("want a neither-Run-nor-Do error, got:\n%s", errText(errs))
	}
}

func TestValidateRejectsBothRunAndDo(t *testing.T) {
	errs := Validate(Set{Actions: []Action{{
		Label: "Both",
		Run:   noop,
		Do:    func() tea.Cmd { return nil },
	}}})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "both Run and Do") {
		t.Errorf("want a both-set error, got:\n%s", errText(errs))
	}
}

func TestValidateRejectsAMissingLabel(t *testing.T) {
	errs := Validate(Set{Actions: []Action{{Run: noop}}})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "Label is required") {
		t.Errorf("want a missing-label error, got:\n%s", errText(errs))
	}
}

// A duplicate shortcut resolves to whichever action is listed first, which is
// exactly the kind of silent behaviour a test should catch instead of a user.
func TestValidateRejectsADuplicateShortcut(t *testing.T) {
	b := key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "run"))
	errs := Validate(Set{Actions: []Action{
		{Label: "Restart", Key: b, Run: noop},
		{Label: "Rebuild", Key: b, Run: noop},
	}})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), `key "r" already bound`) {
		t.Errorf("want a duplicate-key error, got:\n%s", errText(errs))
	}
}

// Two actions sharing an identity make one Exclusive action disable the other.
func TestValidateRejectsADuplicateIdentity(t *testing.T) {
	errs := Validate(Set{Actions: []Action{
		{Label: "Restart", Run: noop},
		{Label: "Restart", Run: noop},
	}})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "identity") {
		t.Errorf("want a duplicate-identity error, got:\n%s", errText(errs))
	}
}

func TestExplicitIDSeparatesTwoActionsSharingALabel(t *testing.T) {
	errs := Validate(Set{Actions: []Action{
		{Label: "Restart", ID: "restart-api", Run: noop},
		{Label: "Restart", ID: "restart-web", Run: noop},
	}})
	if len(errs) != 0 {
		t.Errorf("distinct IDs should separate identical labels, got:\n%s", errText(errs))
	}
}

func TestIdentDefaultsToLabel(t *testing.T) {
	if got := (Action{Label: "Restart"}).Ident(); got != "Restart" {
		t.Errorf("Ident() = %q, want Restart", got)
	}
	if got := (Action{Label: "Restart", ID: "x"}).Ident(); got != "x" {
		t.Errorf("Ident() = %q, want x", got)
	}
}

func TestRunKeySeparatesTargets(t *testing.T) {
	a := Action{Label: "Restart"}
	if RunKey(a, "web") == RunKey(a, "api") {
		t.Error("RunKey must distinguish targets, or exclusivity blocks unrelated runs")
	}
}
