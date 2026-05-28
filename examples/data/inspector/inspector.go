// Package inspector demonstrates pkg/inspector — a two-column label/value
// viewer with expand/collapse for nested structured records. The synthetic
// payload is a k8s-pod-shaped JSON document fed through inspector.FromMap,
// which is the path most callers will hit when displaying REST or
// kubectl-style responses.
package inspector

import (
	"encoding/json"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/ansi"
	insp "github.com/jsdrews/tuilib/pkg/inspector"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// statusRunning demonstrates that foreground-only ANSI (pkg/ansi.CellColor)
// flowing into a Field.Value renders in color without breaking the
// current-line background highlight.
var statusRunning = ansi.CellColor(2, "● Running")

// samplePodJSON is a synthetic kubectl-get-pod-style payload, mirrored from
// real shapes. We keep it as JSON rather than hand-built Fields so the
// example shows the FromAny / FromMap path most callers will use.
const samplePodJSON = `{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
    "name": "api-gateway-7d8b6c9f44-x7p2k",
    "namespace": "production",
    "labels": {
      "app": "api-gateway",
      "version": "1.4.2",
      "tier": "frontend"
    },
    "annotations": {
      "prometheus.io/scrape": "true",
      "prometheus.io/port": "9090",
      "kubectl.kubernetes.io/last-applied-configuration": "{\"apiVersion\":\"v1\",\"kind\":\"Pod\",\"metadata\":{\"name\":\"api-gateway-7d8b6c9f44-x7p2k\",\"namespace\":\"production\",\"labels\":{\"app\":\"api-gateway\",\"version\":\"1.4.2\",\"tier\":\"frontend\"}},\"spec\":{\"serviceAccountName\":\"api-gateway\",\"restartPolicy\":\"Always\",\"terminationGracePeriodSeconds\":30}}",
      "checksum/config": "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08-rollout-2026-04-29T10:14:33Z-by-deploy-bot@example.com"
    }
  },
  "spec": {
    "serviceAccountName": "api-gateway",
    "containers": [
      {
        "name": "gateway",
        "image": "registry.example.com/platform/api-gateway@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
        "command": ["/usr/local/bin/gateway", "--config=/etc/gateway/config.yaml", "--log-level=info", "--upstream-timeout=30s", "--metrics-addr=0.0.0.0:9090", "--tracing-endpoint=https://otel-collector.observability.svc.cluster.local:4317"],
        "ports": [
          {"containerPort": 8080, "protocol": "TCP"},
          {"containerPort": 9090, "protocol": "TCP", "name": "metrics"}
        ],
        "env": [
          {"name": "DATABASE_URL", "value": "postgres://gateway-user:REDACTED@postgres-primary.production.svc.cluster.local:5432/gateway?sslmode=require&pool_max_conns=25&statement_cache_capacity=512"},
          {"name": "FEATURE_FLAGS", "value": "rate-limit-v2,circuit-breaker,distributed-tracing,request-coalescing,async-egress,websocket-upgrade,grpc-passthrough,zstd-compression"}
        ],
        "resources": {
          "requests": {"cpu": "100m", "memory": "256Mi"},
          "limits":   {"cpu": "500m", "memory": "512Mi"}
        }
      },
      {
        "name": "envoy",
        "image": "envoyproxy/envoy:v1.28.0",
        "ports": [{"containerPort": 15000, "protocol": "TCP"}]
      }
    ],
    "restartPolicy": "Always",
    "terminationGracePeriodSeconds": 30
  },
  "status": {
    "phase": "Running",
    "podIP": "10.244.3.17",
    "hostIP": "10.0.1.42",
    "startTime": "2026-04-29T10:14:33Z"
  }
}`

func sampleFields() []insp.Field {
	var v any
	_ = json.Unmarshal([]byte(samplePodJSON), &v)
	fields := insp.FromAny(v)
	// Inject a styled status row at the top so the example exercises ANSI
	// values flowing through the value column.
	hdr := insp.Field{Label: "STATUS", Value: statusRunning}
	return append([]insp.Field{hdr}, fields...)
}

type Screen struct {
	t   theme.Theme
	ins insp.Model
}

func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	return s
}

func (s *Screen) Title() string         { return "Inspector" }
func (s *Screen) Init() tea.Cmd         { return textinput.Blink }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) IsCapturingKeys() bool { return s.ins.Searching() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.ins, cmd = s.ins.Update(msg)
	return s, cmd
}

func (s *Screen) Layout() layout.Node { return layout.Sized(&s.ins) }

func (s *Screen) Help() []key.Binding {
	base := []key.Binding{
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
	return append(base, s.ins.Help()...)
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t
	cursor, query := s.ins.Cursor(), s.ins.Query()
	opts := t.Inspector()
	opts.Title = "pod / api-gateway-7d8b6c9f44-x7p2k"
	opts.Fields = sampleFields()
	opts.Filterable = true
	opts.InitialDepth = 2
	opts.Filter.Placeholder = "search labels and values…"
	s.ins = insp.New(opts)
	if query != "" {
		s.ins.SetQuery(query)
	}
	s.ins.SetCursor(cursor)
}
