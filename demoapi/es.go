package demoapi

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jsdrews/tuilib/demoapi/api"
)

// # Elasticsearch search
//
// POST /logs-app/_search answers like Elasticsearch's search API over one
// log index, closely enough that a client paging it is shaped like one
// paging a real cluster behind ECK. What is reproduced, because a timeline
// over Anchored data has to survive it:
//
//   - from + size past 10,000 is refused (index.max_result_window): deep
//     data is reachable only with search_after.
//   - search_after takes the sort values of the last hit. Several documents
//     share each @timestamp, so a sort without the _shard_doc tiebreaker is
//     refused — with it a page can't lose the documents at its edge.
//   - Paging backwards is the same query with the sort reversed.
//   - hits.total counts to 10,000 and then says "gte", unless
//     track_total_hits asks for exact (true) or none (false).
//   - The index keeps receiving documents. A new one is searchable only
//     after the next refresh (refresh_interval 1s), and every seventeenth
//     arrives three seconds after its own @timestamp — so a client tailing
//     with search_after and no overlap never sees it.
//   - query is match_all or query_string with field:value terms joined by
//     AND (log.level, service.name, or bare words against message).
//
//	POST /logs-app/_search
//	{"size":200,"sort":[{"@timestamp":"desc"},{"_shard_doc":"desc"}],
//	 "search_after":[1790000000000,4211],
//	 "query":{"query_string":{"query":"log.level:error AND timeout"}},
//	 "track_total_hits":false}

const (
	esSeeded       = 20000
	esRate         = 5 // documents a second, after the seeded ones
	esMaxWindow    = 10000
	esTrackDefault = 10000
	esRefresh      = time.Second
	esIngestLag    = 3 * time.Second
	// esPerStamp documents share each @timestamp, so the tiebreaker is
	// required for paging that loses nothing.
	esPerStamp = 2
)

var (
	esLevels   = []string{"info", "info", "info", "warn", "info", "error"}
	esServices = []string{"gateway", "orders", "payments", "search"}
)

// esDoc is document seq of the index (0-based, in @timestamp order).
type esDoc struct {
	seq     int
	ts      time.Time
	level   string
	service string
	msg     string
}

// esFirst is the @timestamp of the first document: far enough back that
// the seeded ones end at the moment the fixture was built.
func (s *server) esFirst() time.Time {
	return s.started.Add(-time.Duration(esSeeded/esPerStamp) * time.Second / esRate * esPerStamp)
}

func (s *server) esDocAt(seq int) esDoc {
	d := esDoc{
		seq:     seq,
		ts:      s.esFirst().Add(time.Duration(seq/esPerStamp) * esPerStamp * time.Second / esRate),
		level:   esLevels[(seq*7)%len(esLevels)],
		service: esServices[(seq*3)%len(esServices)],
	}
	switch d.level {
	case "error":
		d.msg = fmt.Sprintf("upstream timeout after 5000ms (request %d)", seq)
	case "warn":
		d.msg = fmt.Sprintf("slow query: %dms", 200+seq%900)
	default:
		d.msg = fmt.Sprintf("handled request %d in %dms", seq, 3+seq%40)
	}
	return d
}

// esVisible reports whether document seq is searchable at now: ingested
// (late for every seventeenth) and then refreshed.
func (s *server) esVisible(seq int, now time.Time) bool {
	if seq < esSeeded {
		return true
	}
	at := s.esDocAt(seq).ts
	if seq%17 == 0 {
		at = at.Add(esIngestLag)
	}
	// Searchable from the first refresh after ingestion.
	at = at.Truncate(esRefresh).Add(esRefresh)
	return !at.After(now)
}

// esCount is how many documents could be searchable by now: the seeded
// ones plus those whose time has come, before refresh and lag.
func (s *server) esCount(now time.Time) int {
	n := esSeeded + int(now.Sub(s.started)*esRate/time.Second)
	return max(esSeeded, n)
}

func esErr(w http.ResponseWriter, code int, kind, reason string) {
	writeJSON(w, code, esError(code, kind, reason))
}

func esError(code int, kind, reason string) api.EsError {
	e := api.EsError{Status: code}
	e.Error.Type, e.Error.Reason = kind, reason
	return e
}

// EsSearch answers POST /logs-app/_search.
func (s *server) EsSearch(_ context.Context, r api.EsSearchRequestObject) (api.EsSearchResponseObject, error) {
	refuse := func(kind, reason string) (api.EsSearchResponseObject, error) {
		return api.EsSearch400JSONResponse(esError(http.StatusBadRequest, kind, reason)), nil
	}
	var req api.EsSearchRequest
	if r.Body != nil {
		req = *r.Body
	}
	size, from := 10, deref(req.From)
	if req.Size != nil {
		size = *req.Size
	}
	if from+size > esMaxWindow {
		return refuse("illegal_argument_exception",
			fmt.Sprintf("Result window is too large, from + size must be less than or equal to: [%d] but was [%d]. See the scroll or search_after api for a more efficient way to request large data sets.",
				esMaxWindow, from+size))
	}

	desc, ok := esParseSort(deref(req.Sort))
	if !ok {
		return refuse("illegal_argument_exception",
			`sort must be [{"@timestamp": order}, {"_shard_doc": order}] in one direction: @timestamp alone is not unique, and search_after would skip or repeat documents at page edges`)
	}
	if req.SearchAfter != nil && len(*req.SearchAfter) != 2 {
		return refuse("illegal_argument_exception", "search_after must have one value per sort field")
	}
	match, err := esParseQuery(deref(req.Query))
	if err != nil {
		return refuse("query_shard_exception", err.Error())
	}

	now := s.w.now()
	n := s.esCount(now)
	var matched []esDoc
	for seq := 0; seq < n; seq++ {
		if !s.esVisible(seq, now) {
			continue
		}
		if d := s.esDocAt(seq); match(d) {
			matched = append(matched, d)
		}
	}
	key := func(d esDoc) (int64, int64) { return d.ts.UnixMilli(), int64(d.seq) }
	sort.Slice(matched, func(a, b int) bool {
		ta, sa := key(matched[a])
		tb, sb := key(matched[b])
		if ta != tb {
			return (ta < tb) != desc
		}
		return (sa < sb) != desc
	})
	total := len(matched)
	if req.SearchAfter != nil {
		at, as := (*req.SearchAfter)[0], (*req.SearchAfter)[1]
		cut := sort.Search(len(matched), func(i int) bool {
			t, sq := key(matched[i])
			if desc {
				return t < at || (t == at && sq < as)
			}
			return t > at || (t == at && sq > as)
		})
		matched = matched[cut:]
	}
	from = min(from, len(matched))
	page := matched[from:min(len(matched), from+size)]

	out := api.EsSearch200JSONResponse{Took: 1}
	out.Hits.Hits = make([]api.EsHit, 0, len(page))
	for _, d := range page {
		t, sq := key(d)
		out.Hits.Hits = append(out.Hits.Hits, api.EsHit{
			UnderscoreIndex: "logs-app",
			UnderscoreId:    fmt.Sprintf("doc-%d", d.seq),
			UnderscoreSource: api.LogDoc{
				Timestamp: d.ts.UTC(), Level: d.level, Service: d.service, Message: d.msg,
			},
			Sort: []int64{t, sq},
		})
	}

	// track_total_hits: absent counts to 10,000, true counts exactly, false
	// not at all, and a number counts to it.
	limit, exact := esTrackDefault, false
	if req.TrackTotalHits != nil {
		if track, err := req.TrackTotalHits.AsEsSearchRequestTrackTotalHits0(); err == nil {
			if !track {
				return out, nil
			}
			exact = true
		} else if n, err := req.TrackTotalHits.AsEsSearchRequestTrackTotalHits1(); err == nil {
			limit = n
		}
	}
	rel := api.EsSearchResultHitsTotalRelationEq
	if !exact && total > limit {
		total, rel = limit, api.EsSearchResultHitsTotalRelationGte
	}
	out.Hits.Total = &api.EsSearchResult_Hits_Total{Value: total, Relation: rel}
	return out, nil
}

// esParseSort accepts exactly @timestamp then _shard_doc, one direction.
func esParseSort(sorts []map[string]any) (desc, ok bool) {
	if len(sorts) != 2 {
		return false, false
	}
	order := func(m map[string]any, field string) (string, bool) {
		v, has := m[field]
		if !has {
			return "", false
		}
		switch o := v.(type) {
		case string:
			return o, true
		case map[string]any:
			s, _ := o["order"].(string)
			return s, s != ""
		}
		return "", false
	}
	a, ok1 := order(sorts[0], "@timestamp")
	b, ok2 := order(sorts[1], "_shard_doc")
	if !ok1 || !ok2 || a != b || (a != "asc" && a != "desc") {
		return false, false
	}
	return a == "desc", true
}

// esParseQuery supports match_all and query_string of AND-ed terms.
func esParseQuery(q map[string]any) (func(esDoc) bool, error) {
	if q == nil {
		return func(esDoc) bool { return true }, nil
	}
	if _, ok := q["match_all"]; ok {
		return func(esDoc) bool { return true }, nil
	}
	qs, ok := q["query_string"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("only match_all and query_string are supported")
	}
	text, _ := qs["query"].(string)
	var preds []func(esDoc) bool
	for _, term := range strings.Split(text, " AND ") {
		term = strings.TrimSpace(term)
		if term == "" || term == "*" {
			continue
		}
		field, value, scoped := strings.Cut(term, ":")
		value = strings.ToLower(strings.Trim(value, `"`))
		switch {
		case scoped && field == "log.level":
			preds = append(preds, func(d esDoc) bool { return d.level == value })
		case scoped && field == "service.name":
			preds = append(preds, func(d esDoc) bool { return d.service == value })
		case scoped && field == "message":
			preds = append(preds, func(d esDoc) bool { return strings.Contains(strings.ToLower(d.msg), value) })
		case scoped:
			return nil, fmt.Errorf("unknown field [%s]", field)
		default:
			word := strings.ToLower(strings.Trim(term, `"`))
			preds = append(preds, func(d esDoc) bool { return strings.Contains(strings.ToLower(d.msg), word) })
		}
	}
	return func(d esDoc) bool {
		for _, p := range preds {
			if !p(d) {
				return false
			}
		}
		return true
	}, nil
}
