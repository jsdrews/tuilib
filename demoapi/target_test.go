package demoapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// From is the switch between "the examples need no sidecar" and "a TUI and a
// curl share one world". Both halves are cheap to get subtly wrong — a missing
// scheme, a doubled slash — and the symptom is a demo that opens on an error.

func TestFromDefaultsToInProcess(t *testing.T) {
	t.Setenv(EnvBase, "")

	tgt := From(Options{Apps: 3, Seed: 1})
	if tgt.Live {
		t.Error("Live is true with no base set")
	}

	resp, err := tgt.Client.Get(tgt.URL("/apps?limit=1"))
	if err != nil {
		t.Fatalf("in-process request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var p page
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Total != 3 {
		t.Errorf("total = %d, want the 3 apps Options asked for", p.Total)
	}
}

func TestFromReachesALiveServer(t *testing.T) {
	srv := httptest.NewServer(New(Options{Apps: 9, Seed: 2}))
	defer srv.Close()

	t.Setenv(EnvBase, srv.URL)
	tgt := From(Options{Apps: 3, Seed: 1})

	if !tgt.Live {
		t.Error("Live is false with a base set")
	}

	resp, err := tgt.Client.Get(tgt.URL("/apps?limit=1"))
	if err != nil {
		t.Fatalf("live request: %v", err)
	}
	defer resp.Body.Close()
	var p page
	_ = json.NewDecoder(resp.Body).Decode(&p)

	// The server's world, not the one Options describes: a second world
	// generated here would answer none of the requests.
	if p.Total != 9 {
		t.Errorf("total = %d, want the server's 9 — Options must be ignored when live", p.Total)
	}
}

func TestFromNormalisesTheBase(t *testing.T) {
	for _, tc := range []struct{ set, want string }{
		{"http://localhost:8099", "http://localhost:8099"},
		{"http://localhost:8099/", "http://localhost:8099"},
		{"localhost:8099", "http://localhost:8099"},
		{"127.0.0.1:8099/", "http://127.0.0.1:8099"},
	} {
		t.Setenv(EnvBase, tc.set)
		if got := From(Options{Apps: 1}).Base; got != tc.want {
			t.Errorf("From(%q).Base = %q, want %q", tc.set, got, tc.want)
		}
	}
}

func TestTargetURL(t *testing.T) {
	tgt := Target{Base: "http://host:1"}
	for _, tc := range []struct{ in, want string }{
		{"/apps", "http://host:1/apps"},
		{"apps", "http://host:1/apps"},
		{"/apps?limit=1", "http://host:1/apps?limit=1"},
	} {
		if got := tgt.URL(tc.in); got != tc.want {
			t.Errorf("URL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
