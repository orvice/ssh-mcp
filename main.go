package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	sshClient, err := NewSSHClient(cfg.PrivateKey)
	if err != nil {
		log.Fatalf("Failed to initialize SSH client: %v", err)
	}

	store := NewConnectionStore()

	s := mcp.NewServer(
		&mcp.Implementation{
			Name:    "ssh-mcp",
			Version: "1.0.0",
		},
		nil,
	)

	registerTools(s, store, sshClient)

	handler := mcp.NewSSEHandler(func(r *http.Request) *mcp.Server {
		return s
	}, nil)

	log.Printf("SSH MCP server listening on %s", cfg.Listen)
	if err := http.ListenAndServe(cfg.Listen, handler); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
