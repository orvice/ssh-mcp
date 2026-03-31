package main

import (
	"context"
	"fmt"
	"os"

	"github.com/1password/onepassword-sdk-go"
)

func LoadSSHKeyFromOnePassword(secretReference string) ([]byte, error) {
	token := os.Getenv("OP_SERVICE_ACCOUNT_TOKEN")

	client, err := onepassword.NewClient(
		context.Background(),
		onepassword.WithServiceAccountToken(token),
		onepassword.WithIntegrationInfo("ssh-mcp", "1.0.0"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create 1Password client: %w", err)
	}

	secret, err := client.Secrets().Resolve(context.Background(), secretReference)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve 1Password secret %s: %w", secretReference, err)
	}

	return []byte(secret), nil
}
