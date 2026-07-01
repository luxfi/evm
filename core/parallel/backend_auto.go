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
// GoEVM (Block-STM across all cores) is the only real backend. Stub backends
// are not allowed in the tree — a backend is wired end-to-end and parity-proven
// against GoEVM before it exists here. GPU is detected for observability only;
// it does not change execution until a real GPU-dispatching backend is wired
// and passes the parity gate.
func detectBackend() {
	selectedResult = GoEVM
	if gpu := DefaultGPU(); gpu.Available() {
		switch runtime.GOOS {
		case "darwin":
			selectedGPUName = "Metal (present; informational — GoEVM in use)"
		case "linux":
			selectedGPUName = "CUDA (present; informational — GoEVM in use)"
		default:
			selectedGPUName = "GPU (present; informational — GoEVM in use)"
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
