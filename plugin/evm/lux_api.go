// Copyright (C) 2025-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"
	"fmt"

	"github.com/luxfi/evm/core/parallel"
	log "github.com/luxfi/log"
)

// EvmBackendAPI exposes EVM backend switching via RPC.
// Generic — works on any chain that embeds this EVM.
//
// RPC methods (namespace "evm"):
//   - evm_backend      — returns the active EVM backend name
//   - evm_backends     — returns all available backend names
//   - evm_setBackend   — switches the active backend (admin-only)
type EvmBackendAPI struct {
	vm *VM
}

func NewEvmBackendAPI(vm *VM) *EvmBackendAPI {
	return &EvmBackendAPI{vm: vm}
}

type BackendResult struct {
	Backend string `json:"backend"`
}

type BackendsResult struct {
	Backends []string `json:"backends"`
	Active   string   `json:"active"`
}

type SetBackendArgs struct {
	Backend string `json:"backend"`
}

type SetBackendResult struct {
	Previous string `json:"previous"`
	Active   string `json:"active"`
}

// Backend returns the currently active EVM backend.
// RPC: evm_backend
func (api *EvmBackendAPI) Backend(_ context.Context) (*BackendResult, error) {
	return &BackendResult{
		Backend: string(parallel.ActiveBackend()),
	}, nil
}

// Backends returns all available EVM backends and the active one.
// RPC: evm_backends
func (api *EvmBackendAPI) Backends(_ context.Context) (*BackendsResult, error) {
	available := parallel.AvailableBackends()
	names := make([]string, len(available))
	for i, b := range available {
		names[i] = string(b)
	}
	return &BackendsResult{
		Backends: names,
		Active:   string(parallel.ActiveBackend()),
	}, nil
}

// SetBackend switches the active EVM backend at runtime.
// Valid: "gevm", "revm", "cevm", "auto".
// Requires admin API enabled.
// RPC: evm_setBackend
func (api *EvmBackendAPI) SetBackend(_ context.Context, args SetBackendArgs) (*SetBackendResult, error) {
	if !api.vm.config.AdminAPIEnabled {
		return nil, fmt.Errorf("admin API disabled; evm_setBackend requires admin access")
	}

	requested := parallel.EVMBackend(args.Backend)
	switch requested {
	case parallel.GoEVM, parallel.RustEVM, parallel.CppEVM, parallel.AutoEVM:
	default:
		return nil, fmt.Errorf("unknown backend %q; valid: gevm, revm, cevm, auto", args.Backend)
	}

	previous := parallel.ActiveBackend()
	parallel.SetBackend(requested)
	active := parallel.ActiveBackend()

	log.Info("evm_setBackend", "previous", string(previous), "requested", args.Backend, "active", string(active))

	return &SetBackendResult{
		Previous: string(previous),
		Active:   string(active),
	}, nil
}
