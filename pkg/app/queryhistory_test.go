package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jsdrews/tuilib/pkg/output"
	"github.com/jsdrews/tuilib/pkg/source"
	"github.com/jsdrews/tuilib/pkg/statusbar"
)

// A remote table's query history lands in the console with no screen code:
// the source's neutral messages are the shell's to format, as runner's are.
func TestQueryHistoryIsLogged(t *testing.T) {
	m := newOutputApp(t, &stubScreen{name: "Apps"})
	eu := source.Query{Raw: "eu", Sort: "Region", Desc: true}

	m = send(t, m, source.QueryCancelledMsg{Query: source.Query{Raw: "e"}, By: eu})
	m = send(t, m, source.QueryAnsweredMsg{Query: eu, Elapsed: 4200 * time.Millisecond})

	recs := m.outBuf.Records()
	if len(recs) != 2 {
		t.Fatalf("buffered %d records, want 2", len(recs))
	}
	if want := "filter e → cancelled, superseded by filter eu · sort Region▼"; recs[0].Text != want {
		t.Errorf("cancelled = %q, want %q", recs[0].Text, want)
	}
	if want := "filter eu · sort Region▼ → answered in 4.2s"; recs[1].Text != want {
		t.Errorf("answered = %q, want %q", recs[1].Text, want)
	}
	if msg, _ := m.sb.Message(); msg != "" {
		t.Errorf("history reached the statusbar: %q — only failures should", msg)
	}
}

func TestQueryFailureLogsChainAndPaintsBar(t *testing.T) {
	m := newOutputApp(t, &stubScreen{name: "Apps"})
	err := fmt.Errorf("fetch page: %w", errors.New("503 Service Unavailable"))

	m = send(t, m, source.QueryFailedMsg{Query: source.Query{Raw: "eu"}, Err: err, Elapsed: time.Second})

	msg, kind := m.sb.Message()
	if kind != statusbar.MessageError || !strings.Contains(msg, "filter eu failed") {
		t.Errorf("statusbar = %q (%v), want the failure summary", msg, kind)
	}
	recs := m.outBuf.Records()
	if len(recs) != 3 || recs[0].Level != output.LevelError || recs[2].Text != "503 Service Unavailable" {
		t.Fatalf("records = %+v, want a head and the unwrapped chain", recs)
	}
}

func TestWindowFailureNamesTheRows(t *testing.T) {
	m := newOutputApp(t, &stubScreen{name: "Apps"})
	m = send(t, m, source.QueryFailedMsg{
		Query:  source.Query{Offset: 200, Limit: 100},
		Err:    errors.New("timeout"),
		Window: true,
	})
	if got := m.outBuf.Records()[0].Text; !strings.HasPrefix(got, "rows 201–300 of all failed") {
		t.Errorf("record = %q", got)
	}
}
