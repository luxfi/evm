// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package parallel

import (
	"runtime"
	"sync"

	log "github.com/luxfi/log"
)

// selectedBackend records the auto-detected backend at startup.
// Set once during init and never modified.
var (
	selectedOnce    sync.Once
	selectedResult  EVMBackend
	selectedGPUName string
)

func init() {
	selectedOnce.Do(detectBackend)
}

// detectBackend selects the parallel-EVM backend.
//
// LP-108 (2026-05-04): collapsed to GoEVM only. The previous code
// auto-selected CppEVM when a GPU was detected, but the
// cevmExecutor.ExecuteTransaction body returned `nil, nil` (always
// fell through to Go EVM regardless of selection). That was a
// pretense, not an acceleration path.
//
// CppEVM and RustEVM are kept behind their respective build tags
// (`//go:build cevm` / `//go:build revm`) so the registrations are
// only present when those backends are wired through the cgo
// bridge and the parity gate against Go EVM is met. Until then,
// auto-detect returns Go EVM honestly.
//
// To re-enable CppEVM auto-selection: complete the cevm bridge in
// backend_cevm.go::ExecuteTransaction and add a parity test against
// the Go EVM Block-STM path.
func detectBackend() {
	selectedResult = GoEVM
	if gpu := DefaultGPU(); gpu.Available() {
		switch runtime.GOOS {
		case "darwin":
			selectedGPUName = "Metal (detected; cevm dispatch wiring pending — runs Go EVM)"
		case "linux":
			selectedGPUName = "CUDA (detected; cevm dispatch wiring pending — runs Go EVM)"
		default:
			selectedGPUName = "GPU (detected; cevm dispatch wiring pending — runs Go EVM)"
		}
	} else {
		selectedGPUName = "none"
	}
	log.Info("parallel backend: Go EVM (Block-STM across all cores)",
		"backend", selectedResult,
		"cores", runtime.NumCPU(),
		"gpu_status", selectedGPUName,
	)
}

// SelectedBackend returns the auto-detected backend for metrics/observability.
func SelectedBackend() EVMBackend {
	return selectedResult
}

// SelectedGPU returns the detected GPU name ("Metal", "CUDA", "none").
func SelectedGPU() string {
	return selectedGPUName
}

// IsGPUAvailable returns true if a GPU backend was detected at startup.
func IsGPUAvailable() bool {
	return selectedGPUName != "none" && selectedGPUName != ""
}
