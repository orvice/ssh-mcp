package main

import (
	"fmt"

	"github.com/hashicorp/consul/api"
)

// LoadSSHKeyFromConsul reads an SSH private key from Consul KV store.
// The key path is the Consul KV key (e.g. "ssh/keys/my-server").
// Consul address can be configured via CONSUL_HTTP_ADDR environment variable (default: 127.0.0.1:8500).
func LoadSSHKeyFromConsul(keyPath string) ([]byte, error) {
	client, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create Consul client: %w", err)
	}

	pair, _, err := client.KV().Get(keyPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to read Consul KV key %s: %w", keyPath, err)
	}
	if pair == nil {
		return nil, fmt.Errorf("Consul KV key %s not found", keyPath)
	}

	return pair.Value, nil
}
