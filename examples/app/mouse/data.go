package mouse

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/tree"
)

const alignRight = lipgloss.Right

var fileNames = []string{
	"main.go", "server.go", "server_test.go", "router.go",
	"handler_users.go", "handler_auth.go", "middleware.go",
	"config.go", "config_test.go", "logging.go", "metrics.go",
	"store.go", "store_postgres.go", "store_memory.go",
	"migrate.go", "seed.go", "healthz.go", "version.go",
}

var deployments = []table.Row{
	{"api-gateway", "eu-west-1", "12"},
	{"auth-service", "us-east-1", "6"},
	{"billing", "eu-west-1", "3"},
	{"search-indexer", "ap-south-1", "8"},
	{"notifier", "us-east-1", "2"},
	{"webhooks", "eu-central-1", "4"},
	{"reporting", "ap-south-1", "1"},
	{"scheduler", "us-west-2", "2"},
}

// node is a minimal tree.Node over a label plus children.
type node struct {
	label string
	kids  []tree.Node
}

func (n node) Label() string         { return n.label }
func (n node) Children() []tree.Node { return n.kids }

func leaf(label string) tree.Node { return node{label: label} }

var serviceTree tree.Node = node{label: "cluster", kids: []tree.Node{
	node{label: "namespace: default", kids: []tree.Node{
		node{label: "deployment/api-gateway", kids: []tree.Node{
			leaf("pod/api-gateway-7d4f"),
			leaf("pod/api-gateway-9b2c"),
			leaf("service/api-gateway"),
		}},
		node{label: "deployment/auth-service", kids: []tree.Node{
			leaf("pod/auth-service-1a8e"),
			leaf("configmap/auth-env"),
		}},
	}},
	node{label: "namespace: billing", kids: []tree.Node{
		node{label: "deployment/billing", kids: []tree.Node{
			leaf("pod/billing-4f31"),
			leaf("secret/stripe-key"),
		}},
		leaf("cronjob/invoice-sweep"),
	}},
	node{label: "namespace: kube-system", kids: []tree.Node{
		leaf("daemonset/node-exporter"),
		leaf("deployment/coredns"),
	}},
}}
