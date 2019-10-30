package api

import (
	"context"
	"fmt"
)

// DefaultCluster initializes a client given a Config.
func DefaultCluster(cfg *Config) (Client, error) {
	ctx := context.Background()
	client := &defaultCluster{
		ctx:    ctx,
		config: cfg,
	}

	if client.config.Port == "" {
		client.config.Port = fmt.Sprintf("%d", DefaultPort)
	}

	err := client.setupAPIAddr()
	if err != nil {
		return nil, err
	}

	err = client.resolveAPIAddr()
	if err != nil {
		return nil, err
	}

	err = client.setupHTTPClient()
	if err != nil {
		return nil, err
	}

	err = client.setupHostname()
	if err != nil {
		return nil, err
	}

	err = client.setupProxy()
	if err != nil {
		return nil, err
	}

	return client, nil
}
