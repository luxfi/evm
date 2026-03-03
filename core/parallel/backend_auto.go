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

// detectBackend probes for GPU availability and selects the best backend.
//
// Priority:
//  1. GPU Metal (darwin + CGo + Metal device present)
//  2. GPU CUDA (linux + CGo + NVIDIA device present)
//  3. CPU Parallel (Block-STM multi-core)
//
// This runs exactly once at process startup. No user flag required.
func detectBackend() {
	gpu := DefaultGPU()
	if gpu.Available() {
		switch runtime.GOOS {
		case "darwin":
			selectedResult = CppEVM
			selectedGPUName = "Metal"
		case "linux":
			selectedResult = CppEVM
			selectedGPUName = "CUDA"
		default:
			selectedResult = CppEVM
			selectedGPUName = "GPU"
		}
		log.Info("parallel backend: GPU detected",
			"backend", selectedResult,
			"gpu", selectedGPUName,
			"os", runtime.GOOS,
			"arch", runtime.GOARCH,
		)
		return
	}

	// CPU parallel (Block-STM across all cores).
	selectedResult = GoEVM
	selectedGPUName = "none"
	log.Info("parallel backend: CPU parallel (no GPU)",
		"backend", selectedResult,
		"cores", runtime.NumCPU(),
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
