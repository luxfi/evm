// Copyright (C) 2025-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"
	"fmt"

	"github.com/luxfi/evm/core/parallel"
	log "github.com/luxfi/log"
)

// LuxAPI exposes Lux-specific RPC methods under the "lux" namespace.
// Read methods are always available; write methods require admin API enabled.
//
// RPC methods:
//   - lux_evmBackend     — returns the active EVM backend name
//   - lux_evmBackends    — returns all available backend names
//   - lux_setEvmBackend  — switches the active backend (admin-only)
type LuxAPI struct {
	vm *VM
}

// NewLuxAPI creates a new LuxAPI instance.
func NewLuxAPI(vm *VM) *LuxAPI {
	return &LuxAPI{vm: vm}
}

// EvmBackendResult is the response for lux_evmBackend.
type EvmBackendResult struct {
	Backend string `json:"backend"`
}

// EvmBackendsResult is the response for lux_evmBackends.
type EvmBackendsResult struct {
	Backends []string `json:"backends"`
	Active   string   `json:"active"`
}

// SetEvmBackendArgs is the argument for lux_setEvmBackend.
type SetEvmBackendArgs struct {
	Backend string `json:"backend"`
}

// SetEvmBackendResult is the response for lux_setEvmBackend.
type SetEvmBackendResult struct {
	Previous string `json:"previous"`
	Active   string `json:"active"`
}

// EvmBackend returns the currently active EVM backend.
// RPC: lux_evmBackend
func (api *LuxAPI) EvmBackend(_ context.Context) (*EvmBackendResult, error) {
	return &EvmBackendResult{
		Backend: string(parallel.ActiveBackend()),
	}, nil
}

// EvmBackends returns all available EVM backends and the active one.
// RPC: lux_evmBackends
func (api *LuxAPI) EvmBackends(_ context.Context) (*EvmBackendsResult, error) {
	available := parallel.AvailableBackends()
	names := make([]string, len(available))
	for i, b := range available {
		names[i] = string(b)
	}
	return &EvmBackendsResult{
		Backends: names,
		Active:   string(parallel.ActiveBackend()),
	}, nil
}

// SetEvmBackend switches the active EVM backend at runtime.
// Only valid backend names are accepted: "gevm", "revm", "cevm", "auto".
// Requires admin API to be enabled.
// RPC: lux_setEvmBackend
func (api *LuxAPI) SetEvmBackend(_ context.Context, args SetEvmBackendArgs) (*SetEvmBackendResult, error) {
	if !api.vm.config.AdminAPIEnabled {
		return nil, fmt.Errorf("admin API is disabled; lux_setEvmBackend requires admin access")
	}

	requested := parallel.EVMBackend(args.Backend)
	switch requested {
	case parallel.GoEVM, parallel.RustEVM, parallel.CppEVM, parallel.AutoEVM:
		// valid
	default:
		return nil, fmt.Errorf("unknown backend %q; valid: gevm, revm, cevm, auto", args.Backend)
	}

	previous := parallel.ActiveBackend()
	parallel.SetBackend(requested)
	active := parallel.ActiveBackend()

	log.Info("lux_setEvmBackend", "previous", string(previous), "requested", args.Backend, "active", string(active))

	return &SetEvmBackendResult{
		Previous: string(previous),
		Active:   string(active),
	}, nil
}
