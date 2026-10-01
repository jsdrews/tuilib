package demoapi

import (
	"net/http"
	"testing"
	"time"
)

type awxPage struct {
	Count    int        `json:"count"`
	Next     *string    `json:"next"`
	Previous *string    `json:"previous"`
	Results  []awxEvent `json:"results"`
}

func newAWX(t *testing.T) (*http.Client, *clock) {
	t.Helper()
	c := newClock()
	return Client(New(Options{Seed: 1, Now: c.Now})), c
}

func TestAWXCounterRange(t *testing.T) {
	c, _ := newAWX(t)
	var p awxPage
	getJSON(t, c, "/api/v2/jobs/4187/job_events/?order_by=counter&counter__gt=100&counter__lte=150&page_size=200", &p)
	if len(p.Results) != 50 || p.Results[0].Counter != 101 || p.Results[49].Counter != 150 {
		t.Fatalf("got %d events from %d", len(p.Results), p.Results[0].Counter)
	}
}

func TestAWXPageSizeIsCappedNotRefused(t *testing.T) {
	c, _ := newAWX(t)
	var p awxPage
	if code := getJSON(t, c, "/api/v2/jobs/4187/job_events/?order_by=counter&page_size=500", &p); code != 200 {
		t.Fatalf("status %d", code)
	}
	if len(p.Results) != 200 || p.Count != 8000 || p.Next == nil {
		t.Errorf("results %d count %d next %v", len(p.Results), p.Count, p.Next)
	}
}

func TestAWXMaxCounterIsTheTotal(t *testing.T) {
	c, _ := newAWX(t)
	var p awxPage
	getJSON(t, c, "/api/v2/jobs/4187/job_events/?order_by=-counter&page_size=1", &p)
	if p.Results[0].Counter != 8000 {
		t.Errorf("highest counter = %d", p.Results[0].Counter)
	}
}

func TestAWXLateEventsLeaveHolesThatFill(t *testing.T) {
	c, clk := newAWX(t)
	var p awxPage
	getJSON(t, c, "/api/v2/jobs/4242/job_events/?order_by=counter&counter__gt=0&counter__lte=40&page_size=200", &p)
	have := map[int]bool{}
	for _, e := range p.Results {
		have[e.Counter] = true
	}
	if have[26] || !have[25] {
		t.Fatalf("event 26 should be a hole until saved, 25 present: %v %v", have[26], have[25])
	}
	clk.advance(awxLate)
	getJSON(t, c, "/api/v2/jobs/4242/job_events/?order_by=counter&counter__gt=0&counter__lte=40&page_size=200", &p)
	found := false
	for _, e := range p.Results {
		found = found || e.Counter == 26
	}
	if !found {
		t.Error("a late event should appear on a later read")
	}
}

func TestAWXRunningJobGrowsAndProcessingTrails(t *testing.T) {
	c, clk := newAWX(t)
	var j map[string]any
	getJSON(t, c, "/api/v2/jobs/4242/", &j)
	if j["status"] != "running" {
		t.Fatalf("status = %v", j["status"])
	}
	var a, b awxPage
	getJSON(t, c, "/api/v2/jobs/4242/job_events/?order_by=-counter&page_size=1", &a)
	clk.advance(10 * time.Second)
	getJSON(t, c, "/api/v2/jobs/4242/job_events/?order_by=-counter&page_size=1", &b)
	if b.Results[0].Counter <= a.Results[0].Counter {
		t.Errorf("the running job should grow: %d then %d", a.Results[0].Counter, b.Results[0].Counter)
	}
	// To a second past the last event: the job has finished, its events
	// have not all been processed.
	clk.advance(time.Duration(awxLiveLength-40)*time.Second/awxRate - 10*time.Second + time.Second)
	getJSON(t, c, "/api/v2/jobs/4242/", &j)
	if j["status"] != "successful" || j["event_processing_finished"] != false {
		t.Errorf("finished but still processing: %v %v", j["status"], j["event_processing_finished"])
	}
	clk.advance(awxTrail)
	getJSON(t, c, "/api/v2/jobs/4242/", &j)
	if j["event_processing_finished"] != true {
		t.Error("processing should finish after the trail")
	}
}

func TestAWXFindNextMatch(t *testing.T) {
	c, _ := newAWX(t)
	var p awxPage
	getJSON(t, c, "/api/v2/jobs/4187/job_events/?stdout__icontains=FATAL&counter__gt=37&order_by=counter&page_size=1", &p)
	if len(p.Results) != 1 || p.Results[0].Counter != 74 {
		t.Fatalf("next fatal after 37 = %+v", p.Results)
	}
	if !p.Results[0].Failed || p.Results[0].Event != "runner_on_failed" {
		t.Errorf("event = %+v", p.Results[0])
	}
}

func TestAWXFilters(t *testing.T) {
	c, _ := newAWX(t)
	var p awxPage
	getJSON(t, c, "/api/v2/jobs/4187/job_events/?failed=true&page_size=200", &p)
	for _, e := range p.Results {
		if !e.Failed {
			t.Fatalf("failed=true returned %+v", e)
		}
	}
	getJSON(t, c, "/api/v2/jobs/4187/job_events/?host_name=db-1&page_size=5", &p)
	for _, e := range p.Results {
		if e.HostName != "db-1" {
			t.Fatalf("host_name filter returned %+v", e)
		}
	}
	if code := getJSON(t, c, "/api/v2/jobs/4187/job_events/?order_by=start_line", nil); code != 400 {
		t.Errorf("an unsupported order should be refused, got %d", code)
	}
}

func TestAWXNoOutputEvents(t *testing.T) {
	c, _ := newAWX(t)
	var p awxPage
	getJSON(t, c, "/api/v2/jobs/4187/job_events/?counter__gte=11&counter__lte=11", &p)
	if len(p.Results) != 1 || p.Results[0].Stdout != "" || p.Results[0].Event != "runner_on_start" {
		t.Errorf("event 11 = %+v", p.Results)
	}
}
