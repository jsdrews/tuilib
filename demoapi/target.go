package demoapi

import (
	"net/http"
	"os"
	"strings"
	"time"
)

// EnvBase names the environment variable that points a client at a running
// demoapi instead of an in-process one.
const EnvBase = "TUILIB_DEMO_API"

// Target is where a client should talk: a client, and the base URL to build
// request paths onto.
type Target struct {
	Client *http.Client

	// Base has no trailing slash. For an in-process target it is a hostname
	// the transport never resolves, and exists only so request URLs are valid.
	Base string

	// Live reports whether this talks to a separate process. A screen that
	// wants to say so on its border can read it; nothing else behaves
	// differently.
	Live bool
}

// From returns a Target chosen by the environment.
//
// With TUILIB_DEMO_API set, requests go over the network to that base URL and
// opts is ignored — the server already has a world, and a second one generated
// here would be a different set of applications answering none of the
// requests. Unset, it builds an in-process handler from opts and dispatches
// into it with no socket, which is what keeps `task examples` free of a
// sidecar.
//
// The point of the live path is the workflow the wall-clock rule exists for:
// one world, a TUI watching it, and a curl poking it from another terminal.
func From(opts Options) Target {
	if base := strings.TrimRight(os.Getenv(EnvBase), "/"); base != "" {
		if !strings.Contains(base, "://") {
			base = "http://" + base
		}
		return Target{
			// A timeout, because the in-process transport cannot hang and a
			// real one can — and a TUI whose fetch never returns looks like a
			// TUI that is broken.
			Client: &http.Client{Timeout: 10 * time.Second},
			Base:   base,
			Live:   true,
		}
	}
	return Target{Client: Client(New(opts)), Base: "http://demoapi"}
}

// URL joins a path onto the target's base.
func (t Target) URL(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return t.Base + path
}
