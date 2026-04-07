// Copyright (C) 2025-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"
	"testing"

	"github.com/luxfi/evm/core/parallel"
	"github.com/luxfi/evm/plugin/evm/config"
)

func TestEvmBackendAPI_Backend(t *testing.T) {
	api := NewEvmBackendAPI(&VM{config: config.Config{AdminAPIEnabled: true}})

	result, err := api.Backend(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Backend != string(parallel.ActiveBackend()) {
		t.Fatalf("expected %q, got %q", parallel.ActiveBackend(), result.Backend)
	}
}

func TestEvmBackendAPI_Backends(t *testing.T) {
	api := NewEvmBackendAPI(&VM{config: config.Config{AdminAPIEnabled: true}})

	result, err := api.Backends(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Backends) == 0 {
		t.Fatal("expected at least one backend")
	}
	found := false
	for _, b := range result.Backends {
		if b == "gevm" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected gevm in backends, got %v", result.Backends)
	}
}

func TestEvmBackendAPI_SetBackend(t *testing.T) {
	api := NewEvmBackendAPI(&VM{config: config.Config{AdminAPIEnabled: true}})

	result, err := api.SetBackend(context.Background(), SetBackendArgs{Backend: "gevm"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Active != "gevm" {
		t.Fatalf("expected gevm, got %q", result.Active)
	}

	result, err = api.SetBackend(context.Background(), SetBackendArgs{Backend: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Active != "gevm" {
		t.Fatalf("expected gevm after auto, got %q", result.Active)
	}
}

func TestEvmBackendAPI_InvalidBackend(t *testing.T) {
	api := NewEvmBackendAPI(&VM{config: config.Config{AdminAPIEnabled: true}})

	_, err := api.SetBackend(context.Background(), SetBackendArgs{Backend: "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid backend")
	}
}

func TestEvmBackendAPI_AdminDisabled(t *testing.T) {
	api := NewEvmBackendAPI(&VM{config: config.Config{AdminAPIEnabled: false}})

	_, err := api.SetBackend(context.Background(), SetBackendArgs{Backend: "gevm"})
	if err == nil {
		t.Fatal("expected error when admin disabled")
	}
}
