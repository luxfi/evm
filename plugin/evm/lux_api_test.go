// Copyright (C) 2025-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"
	"testing"

	"github.com/luxfi/evm/core/parallel"
	"github.com/luxfi/evm/plugin/evm/config"
)

func TestLuxAPIEvmBackend(t *testing.T) {
	vm := &VM{config: config.Config{AdminAPIEnabled: true}}
	api := NewLuxAPI(vm)

	result, err := api.EvmBackend(context.Background())
	if err != nil {
		t.Fatalf("EvmBackend returned error: %v", err)
	}
	if result.Backend != string(parallel.ActiveBackend()) {
		t.Fatalf("expected %q, got %q", parallel.ActiveBackend(), result.Backend)
	}
}

func TestLuxAPIEvmBackends(t *testing.T) {
	vm := &VM{config: config.Config{AdminAPIEnabled: true}}
	api := NewLuxAPI(vm)

	result, err := api.EvmBackends(context.Background())
	if err != nil {
		t.Fatalf("EvmBackends returned error: %v", err)
	}
	if len(result.Backends) == 0 {
		t.Fatal("expected at least one backend (gevm)")
	}
	// gevm is always present
	found := false
	for _, b := range result.Backends {
		if b == "gevm" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected gevm in backends, got %v", result.Backends)
	}
	if result.Active == "" {
		t.Fatal("active backend must not be empty")
	}
}

func TestLuxAPISetEvmBackend(t *testing.T) {
	vm := &VM{config: config.Config{AdminAPIEnabled: true}}
	api := NewLuxAPI(vm)

	// Switch to gevm (always available)
	result, err := api.SetEvmBackend(context.Background(), SetEvmBackendArgs{Backend: "gevm"})
	if err != nil {
		t.Fatalf("SetEvmBackend returned error: %v", err)
	}
	if result.Active != "gevm" {
		t.Fatalf("expected active=gevm, got %q", result.Active)
	}

	// auto should resolve to gevm when no other backends registered
	result, err = api.SetEvmBackend(context.Background(), SetEvmBackendArgs{Backend: "auto"})
	if err != nil {
		t.Fatalf("SetEvmBackend(auto) returned error: %v", err)
	}
	if result.Active != "gevm" {
		t.Fatalf("expected active=gevm after auto, got %q", result.Active)
	}
}

func TestLuxAPISetEvmBackendInvalidName(t *testing.T) {
	vm := &VM{config: config.Config{AdminAPIEnabled: true}}
	api := NewLuxAPI(vm)

	_, err := api.SetEvmBackend(context.Background(), SetEvmBackendArgs{Backend: "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid backend name")
	}
}

func TestLuxAPISetEvmBackendAdminDisabled(t *testing.T) {
	vm := &VM{config: config.Config{AdminAPIEnabled: false}}
	api := NewLuxAPI(vm)

	_, err := api.SetEvmBackend(context.Background(), SetEvmBackendArgs{Backend: "gevm"})
	if err == nil {
		t.Fatal("expected error when admin API is disabled")
	}
}
