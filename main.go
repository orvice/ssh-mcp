package main

import (
	"crypto/subtle"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const version = "1.0.0"

func main() {
	configPath := flag.String("config", "", "path to config file")
	sshKeyPath := flag.String("ssh-key", "", "path to SSH private key (overrides config file)")
	opSSHKey := flag.String("op-ssh-key", "", "1Password secret reference for SSH key (e.g. op://vault/item/private_key)")
	consulSSHKey := flag.String("consul-ssh-key", "", "Consul KV path for SSH key (e.g. ssh/keys/my-server)")
	listen := flag.String("listen", "", "listen address (overrides config file, default 127.0.0.1:8080)")
	authToken := flag.String("auth-token", "", "Bearer token required for MCP HTTP requests")
	knownHosts := flag.String("known-hosts", "", "known_hosts file for SSH host key verification")
	insecureIgnoreHostKey := flag.Bool("insecure-ignore-host-key", false, "disable SSH host key verification (unsafe)")
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
		cfg = DefaultConfig()
	}

	// Flag overrides
	if *listen != "" {
		cfg.Listen = *listen
	}
	if *authToken != "" {
		cfg.AuthToken = *authToken
	}
	if *knownHosts != "" {
		cfg.KnownHosts = *knownHosts
	}
	if *insecureIgnoreHostKey {
		cfg.InsecureIgnoreHostKey = true
	}
	if *sshKeyPath != "" {
		cfg.PrivateKey = *sshKeyPath
	}
	cfg.Normalize()

	sshOpts := SSHClientOptions{
		KnownHosts:            cfg.KnownHosts,
		InsecureIgnoreHostKey: cfg.InsecureIgnoreHostKey,
		Timeout:               cfg.SSHTimeout(),
		PrivateKeyPassphrase:  cfg.PrivateKeyPassphrase,
	}

	if cfg.PrivateKey == "" && *opSSHKey == "" && *consulSSHKey == "" && len(cfg.Connections) == 0 {
		log.Fatal("SSH key must be provided via --ssh-key, --op-ssh-key, --consul-ssh-key, config file private_key, or connection-level private_key")
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
		sshClient, err = NewSSHClientFromKeyData(keyData, sshOpts)
		if err != nil {
			log.Fatalf("Failed to initialize SSH client from 1Password key: %v", err)
		}
	case *consulSSHKey != "":
		keyData, err := LoadSSHKeyFromConsul(*consulSSHKey)
		if err != nil {
			log.Fatalf("Failed to load SSH key from Consul: %v", err)
		}
		sshClient, err = NewSSHClientFromKeyData(keyData, sshOpts)
		if err != nil {
			log.Fatalf("Failed to initialize SSH client from Consul key: %v", err)
		}
	case cfg.PrivateKey != "":
		sshClient, err = NewSSHClient(cfg.PrivateKey, sshOpts)
		if err != nil {
			log.Fatalf("Failed to initialize SSH client: %v", err)
		}
	default:
		sshClient, err = NewSSHClientFromKeyData(nil, sshOpts)
		if err != nil {
			log.Fatalf("Failed to initialize SSH client: %v", err)
		}
	}

	store, err := NewConfiguredConnectionStore(cfg.ConnectionStore)
	if err != nil {
		log.Fatalf("Failed to initialize connection store: %v", err)
	}
	if err := AddConfiguredConnections(store, cfg.Connections); err != nil {
		log.Fatalf("Failed to add configured connections: %v", err)
	}

	policy := NewPolicy(cfg.Policy)
	auditLogger, err := NewAuditLogger(cfg.Audit)
	if err != nil {
		log.Fatalf("Failed to initialize audit logger: %v", err)
	}
	metrics := NewMetrics()

	s := mcp.NewServer(
		&mcp.Implementation{
			Name:    "ssh-mcp",
			Version: version,
		},
		nil,
	)

	registerTools(s, store, sshClient, cfg, policy, auditLogger, metrics)

	handler := mcp.NewSSEHandler(func(r *http.Request) *mcp.Server {
		return s
	}, nil)

	mux := http.NewServeMux()
	mux.Handle("/", handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(metrics.Snapshot())
	})

	var httpHandler http.Handler = metrics.Middleware(mux)
	if cfg.AuthToken != "" {
		httpHandler = bearerAuth(httpHandler, cfg.AuthToken)
		log.Print("MCP HTTP Bearer authentication enabled")
	} else {
		log.Print("WARNING: MCP HTTP Bearer authentication is disabled")
	}
	if cfg.InsecureIgnoreHostKey {
		log.Print("WARNING: SSH host key verification is disabled")
	}

	log.Printf("SSH MCP server listening on %s", cfg.Listen)
	if err := http.ListenAndServe(cfg.Listen, httpHandler); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func bearerAuth(next http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		provided, ok := strings.CutPrefix(auth, "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
