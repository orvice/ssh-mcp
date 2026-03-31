package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	configPath := flag.String("config", "", "path to config file")
	sshKeyPath := flag.String("ssh-key", "", "path to SSH private key (overrides config file)")
	opSSHKey := flag.String("op-ssh-key", "", "1Password secret reference for SSH key (e.g. op://vault/item/private_key)")
	listen := flag.String("listen", "", "listen address (overrides config file, default :8080)")
	flag.Parse()

	var cfg *Config

	// Load config file if specified
	if *configPath != "" {
		var err error
		cfg, err = LoadConfig(*configPath)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
	} else {
		cfg = &Config{Listen: ":8080"}
	}

	// Flag overrides
	if *listen != "" {
		cfg.Listen = *listen
	}
	if *sshKeyPath != "" {
		cfg.PrivateKey = *sshKeyPath
	}

	// Determine SSH client
	var sshClient *SSHClient
	var err error

	switch {
	case *opSSHKey != "":
		keyData, err := LoadSSHKeyFromOnePassword(*opSSHKey)
		if err != nil {
			log.Fatalf("Failed to load SSH key from 1Password: %v", err)
		}
		sshClient, err = NewSSHClientFromKeyData(keyData)
		if err != nil {
			log.Fatalf("Failed to initialize SSH client from 1Password key: %v", err)
		}
	case cfg.PrivateKey != "":
		sshClient, err = NewSSHClient(cfg.PrivateKey)
		if err != nil {
			log.Fatalf("Failed to initialize SSH client: %v", err)
		}
	default:
		log.Fatal("SSH key must be provided via --ssh-key, --op-ssh-key, or config file private_key")
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
