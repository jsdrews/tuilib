package poll

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Ticks are armed at this interval so a test can run the command rather than
// wait out a realistic cadence.
const tick = time.Millisecond

func newModel() Model { return New(Options{Interval: tick}) }

// run executes cmd once and returns every message it and its batches produced.
//
// Once is not a style choice. tea.Tick builds its timer when the *command* is
// constructed and drains the channel on the first call, so calling the same Cmd
// a second time blocks forever. Collect the messages, then assert against the
// slice.
func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range batch {
		out = append(out, run(c)...)
	}
	return out
}

// tickOf is the tick that was armed, which is the thing worth holding on to:
// its tag decides whether the next delivery is honoured.
func tickOf(t *testing.T, msgs []tea.Msg) tickMsg {
	t.Helper()
	for _, msg := range msgs {
		if tm, ok := msg.(tickMsg); ok {
			return tm
		}
	}
	t.Fatal("no tick was scheduled")
	return tickMsg{}
}

func ticks(msgs []tea.Msg) int {
	n := 0
	for _, msg := range msgs {
		if _, ok := msg.(tickMsg); ok {
			n++
		}
	}
	return n
}

func refreshes(msgs []tea.Msg) int {
	n := 0
	for _, msg := range msgs {
		if _, ok := msg.(RefreshMsg); ok {
			n++
		}
	}
	return n
}

func TestInitSchedulesATickThatIsAccepted(t *testing.T) {
	m := newModel()
	got := run(m.Update(tickOf(t, run(m.Init()))))

	if refreshes(got) != 1 {
		t.Errorf("refreshes = %d, want 1 from the first tick", refreshes(got))
	}
	if ticks(got) != 1 {
		t.Errorf("ticks = %d, want the schedule re-armed exactly once", ticks(got))
	}
}

func TestPausedModelSchedulesNothingFromInit(t *testing.T) {
	m := New(Options{Interval: tick, Paused: true})
	if m.Init() != nil {
		t.Error("a model constructed paused armed a tick anyway")
	}
}

// The headline. Update re-arms on every tick it accepts, so a tick honoured
// twice returns two RefreshMsg and arms two successors — and the chain doubles
// every interval until the screen is fetching continuously. The two easy ways to
// deliver one tick twice (Update called twice in a pass, one screen instance
// twice in the message path) look like nothing at the call site, so the guard
// belongs here rather than in a rule nobody reads at the right moment.
func TestADuplicateTickIsIgnored(t *testing.T) {
	m := newModel()
	first := tickOf(t, run(m.Init()))

	run(m.Update(first))
	again := run(m.Update(first))

	if refreshes(again) != 0 {
		t.Errorf("refreshes = %d, want the duplicate to produce none", refreshes(again))
	}
	if ticks(again) != 0 {
		t.Errorf("ticks = %d; the chain doubled, and would keep doubling", ticks(again))
	}
}

// Init twice is the other spelling of the same mistake — batched into a screen's
// Init and again into OnEnter, say.
func TestTwoInitsLeaveOneChain(t *testing.T) {
	m := newModel()
	a, b := tickOf(t, run(m.Init())), tickOf(t, run(m.Init()))

	if n := refreshes(run(m.Update(a))); n != 1 {
		t.Fatalf("refreshes = %d, want the first tick honoured", n)
	}
	if n := ticks(run(m.Update(b))); n != 0 {
		t.Errorf("ticks = %d, want the second Init's tick dropped", n)
	}
}

// Each accepted tick must arm exactly one successor, or the guard above would
// be trading a doubling chain for a dead one.
func TestTheChainSurvivesManyIntervals(t *testing.T) {
	m := newModel()
	next := tickOf(t, run(m.Init()))
	for i := 0; i < 10; i++ {
		got := run(m.Update(next))
		if refreshes(got) != 1 || ticks(got) != 1 {
			t.Fatalf("interval %d: refreshes=%d ticks=%d, want 1 and 1",
				i, refreshes(got), ticks(got))
		}
		next = tickOf(t, got)
	}
}

func TestPausedModelDropsAnInFlightTick(t *testing.T) {
	m := newModel()
	inFlight := tickOf(t, run(m.Init()))
	m.Pause()

	if n := refreshes(run(m.Update(inFlight))); n != 0 {
		t.Errorf("refreshes = %d, want a paused model to emit none", n)
	}
	if !m.Paused() {
		t.Error("Paused() disagrees with Pause()")
	}
}

// The tick in flight when the pause began must stay stale afterwards, or
// resuming runs two chains.
func TestResumeRetiresATickFromBeforeThePause(t *testing.T) {
	m := newModel()
	stale := tickOf(t, run(m.Init()))
	m.Pause()
	resumed := run(m.Resume())

	if ticks(resumed) != 1 {
		t.Fatalf("ticks = %d, want Resume to arm the next one", ticks(resumed))
	}
	if n := ticks(run(m.Update(stale))); n != 0 {
		t.Errorf("ticks = %d, want the pre-pause tick dropped", n)
	}
}

func TestResumeOnARunningModelIsANoOp(t *testing.T) {
	m := newModel()
	if m.Resume() != nil {
		t.Error("Resume armed a second chain on a model that was already running")
	}
}

func TestSetIntervalRetiresTheOldCadencesTick(t *testing.T) {
	m := newModel()
	stale := tickOf(t, run(m.Init()))
	rescheduled := run(m.SetInterval(2 * tick))

	if m.Interval() != 2*tick {
		t.Errorf("Interval = %v, want the new cadence recorded", m.Interval())
	}
	if ticks(rescheduled) != 1 {
		t.Fatalf("ticks = %d, want SetInterval to reschedule", ticks(rescheduled))
	}
	if n := ticks(run(m.Update(stale))); n != 0 {
		t.Errorf("ticks = %d, want the old cadence's tick dropped", n)
	}
}

// Paused, SetInterval records the cadence and schedules nothing — but the tick
// already in flight still has to be retired, or it fires under the new interval
// the moment the model resumes.
func TestSetIntervalWhilePausedStillRetiresTheOldTick(t *testing.T) {
	m := newModel()
	stale := tickOf(t, run(m.Init()))
	m.Pause()

	if cmd := m.SetInterval(2 * tick); cmd != nil {
		t.Error("SetInterval scheduled a tick on a paused model")
	}
	run(m.Resume())
	if n := ticks(run(m.Update(stale))); n != 0 {
		t.Errorf("ticks = %d, want the tick from before the change dropped", n)
	}
}

func TestSetIntervalRejectsANonPositiveCadence(t *testing.T) {
	m := newModel()
	if cmd := m.SetInterval(0); cmd != nil {
		t.Error("SetInterval(0) rescheduled")
	}
	if m.Interval() != tick {
		t.Errorf("Interval = %v, want it unchanged", m.Interval())
	}
}

// Refresh is an out-of-band request, so it must not touch the schedule: the
// tick already in flight stays valid.
func TestRefreshDoesNotDisturbTheSchedule(t *testing.T) {
	m := newModel()
	scheduled := tickOf(t, run(m.Init()))

	if n := refreshes(run(m.Refresh())); n != 1 {
		t.Fatalf("refreshes = %d, want exactly one", n)
	}
	if n := refreshes(run(m.Update(scheduled))); n != 1 {
		t.Errorf("refreshes = %d, want the scheduled tick still honoured", n)
	}
}

func TestNonTickMessagesPassThrough(t *testing.T) {
	m := newModel()
	if cmd := m.Update(struct{}{}); cmd != nil {
		t.Error("an unrelated message produced a command")
	}
}

func TestMarkRefreshed(t *testing.T) {
	m := newModel()
	if !m.LastRefresh().IsZero() {
		t.Error("LastRefresh should be the zero time before the first refresh")
	}
	at := time.Now().Add(-time.Hour)
	m.MarkRefreshedAt(at)
	if !m.LastRefresh().Equal(at) {
		t.Errorf("LastRefresh = %v, want %v", m.LastRefresh(), at)
	}
	m.MarkRefreshed()
	if !m.LastRefresh().After(at) {
		t.Error("MarkRefreshed did not stamp the current time")
	}
}

func TestNewPanicsOnANonPositiveInterval(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("New(0) did not panic; a zero interval is always a bug")
		}
	}()
	New(Options{})
}
