package demoapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

type esResp struct {
	Hits struct {
		Total *struct {
			Value    int    `json:"value"`
			Relation string `json:"relation"`
		} `json:"total"`
		Hits []struct {
			ID     string         `json:"_id"`
			Source map[string]any `json:"_source"`
			Sort   []int64        `json:"sort"`
		} `json:"hits"`
	} `json:"hits"`
}

func search(t *testing.T, c *http.Client, body map[string]any, into *esResp) int {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := c.Post("http://demo/logs-app/_search", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if into != nil {
		*into = esResp{}
		_ = json.NewDecoder(resp.Body).Decode(into)
	}
	return resp.StatusCode
}

var esSortDesc = []map[string]any{{"@timestamp": "desc"}, {"_shard_doc": "desc"}}
var esSortAsc = []map[string]any{{"@timestamp": "asc"}, {"_shard_doc": "asc"}}

func newES(t *testing.T) (*http.Client, *clock) {
	t.Helper()
	c := newClock()
	return Client(New(Options{Seed: 1, Now: c.Now})), c
}

func TestESWindowLimit(t *testing.T) {
	c, _ := newES(t)
	if code := search(t, c, map[string]any{"from": 9950, "size": 100, "sort": esSortDesc}, nil); code != 400 {
		t.Errorf("from+size past 10,000 should be refused, got %d", code)
	}
}

func TestESSortNeedsATiebreaker(t *testing.T) {
	c, _ := newES(t)
	if code := search(t, c, map[string]any{"size": 5, "sort": []map[string]any{{"@timestamp": "desc"}}}, nil); code != 400 {
		t.Errorf("a sort without _shard_doc should be refused, got %d", code)
	}
}

func TestESSearchAfterWalksBothWaysWithoutLossOrRepeat(t *testing.T) {
	c, _ := newES(t)
	seen := map[string]bool{}
	var r esResp
	search(t, c, map[string]any{"size": 7, "sort": esSortDesc, "track_total_hits": false}, &r)
	newest := r.Hits.Hits[0].ID
	for page := 0; page < 20; page++ {
		for _, h := range r.Hits.Hits {
			if seen[h.ID] {
				t.Fatalf("%s repeated", h.ID)
			}
			seen[h.ID] = true
		}
		last := r.Hits.Hits[len(r.Hits.Hits)-1].Sort
		search(t, c, map[string]any{"size": 7, "sort": esSortDesc, "search_after": last, "track_total_hits": false}, &r)
	}
	if len(seen) != 140 {
		t.Errorf("walked %d documents, want 140 with none lost", len(seen))
	}
	// Back the other way from the oldest seen: the same documents, ascending.
	var back esResp
	search(t, c, map[string]any{"size": 140, "sort": esSortAsc, "search_after": []int64{0, 0}}, &back)
	if back.Hits.Hits[0].ID != "doc-0" {
		t.Errorf("ascending from the start = %s", back.Hits.Hits[0].ID)
	}
	if newest != "doc-19999" {
		t.Errorf("newest = %s", newest)
	}
}

func TestESTotals(t *testing.T) {
	c, _ := newES(t)
	var r esResp
	search(t, c, map[string]any{"size": 1, "sort": esSortDesc}, &r)
	if r.Hits.Total == nil || r.Hits.Total.Value != 10000 || r.Hits.Total.Relation != "gte" {
		t.Errorf("default total = %+v, want 10000 gte", r.Hits.Total)
	}
	search(t, c, map[string]any{"size": 1, "sort": esSortDesc, "track_total_hits": true}, &r)
	if r.Hits.Total.Value != 20000 || r.Hits.Total.Relation != "eq" {
		t.Errorf("exact total = %+v", r.Hits.Total)
	}
	search(t, c, map[string]any{"size": 1, "sort": esSortDesc, "track_total_hits": false}, &r)
	if r.Hits.Total != nil {
		t.Error("track_total_hits false should report no total")
	}
}

func TestESQueryString(t *testing.T) {
	c, _ := newES(t)
	var r esResp
	search(t, c, map[string]any{"size": 50, "sort": esSortDesc,
		"query": map[string]any{"query_string": map[string]any{"query": "log.level:error AND service.name:orders"}}}, &r)
	if len(r.Hits.Hits) == 0 {
		t.Fatal("no hits")
	}
	for _, h := range r.Hits.Hits {
		if h.Source["log.level"] != "error" || h.Source["service.name"] != "orders" {
			t.Fatalf("hit outside the query: %v", h.Source)
		}
	}
}

func TestESGrowsWithRefreshAndLateIngest(t *testing.T) {
	c, clk := newES(t)
	var r esResp
	search(t, c, map[string]any{"size": 1, "sort": esSortDesc}, &r)
	tail := r.Hits.Hits[0].Sort
	ids := func() map[string]bool {
		// Tail strictly after the old newest: a client doing this with no
		// overlap is the one that misses late documents.
		search(t, c, map[string]any{"size": 500, "sort": esSortAsc, "search_after": tail}, &r)
		got := map[string]bool{}
		for _, h := range r.Hits.Hits {
			got[h.ID] = true
		}
		return got
	}
	// doc-20026 (a multiple of 17) is stamped 5.2s in and ingested 3s
	// late; its neighbour doc-20024, stamped 4.8s in, is on time.
	clk.advance(7 * time.Second)
	at7 := ids()
	if !at7["doc-20024"] {
		t.Error("an on-time document should be searchable after the next refresh")
	}
	if at7["doc-20026"] {
		t.Error("a late document must not be searchable inside its ingest lag")
	}
	clk.advance(3 * time.Second)
	if !ids()["doc-20026"] {
		t.Error("a late document should be searchable once ingested and refreshed")
	}
}
