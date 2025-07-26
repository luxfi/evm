// (c) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package forks provides a centralized registry for network upgrades and forks.
// This replaces the scattered IsXYZ() checks throughout the codebase with a
// single, data-driven approach.
package forks

import (
	"math/big"
	"sync"
)

// ForkID uniquely identifies a network upgrade or fork
type ForkID string

// Standard Ethereum forks
const (
	ForkHomestead       ForkID = "homestead"
	ForkDAO             ForkID = "dao"
	ForkTangerineWhistle ForkID = "tangerine-whistle" // EIP-150
	ForkSpuriousDragon  ForkID = "spurious-dragon"    // EIP-155/158
	ForkByzantium       ForkID = "byzantium"
	ForkConstantinople  ForkID = "constantinople"
	ForkPetersburg      ForkID = "petersburg"
	ForkIstanbul        ForkID = "istanbul"
	ForkMuirGlacier     ForkID = "muir-glacier"
	ForkBerlin          ForkID = "berlin"
	ForkLondon          ForkID = "london"
	ForkArrowGlacier    ForkID = "arrow-glacier"
	ForkGrayGlacier     ForkID = "gray-glacier"
	ForkShanghai        ForkID = "shanghai"
	ForkCancun          ForkID = "cancun"
)

// Lux-specific forks
const (
	ForkEVM     ForkID = "evm"      // Base EVM compatibility
	ForkDurango ForkID = "durango"  // Dynamic fees
	ForkEtna    ForkID = "etna"     // State sync improvements
	ForkFortuna ForkID = "fortuna"  // Already activated
	ForkGranite ForkID = "granite"  // Already activated
)

// Fork represents a network upgrade point
type Fork struct {
	ID           ForkID
	Block        *big.Int // nil for time-based forks
	Timestamp    *uint64  // nil for block-based forks
	AlwaysActive bool     // true for historical forks that are always enabled
	Description  string   // Human-readable description
}

// Registry manages all fork definitions
type Registry struct {
	mu    sync.RWMutex
	forks map[ForkID]*Fork
}

// DefaultRegistry contains all known forks
var DefaultRegistry = &Registry{
	forks: map[ForkID]*Fork{
		// Ethereum forks - all enabled from genesis
		ForkHomestead:       {ID: ForkHomestead, AlwaysActive: true, Description: "Homestead"},
		ForkDAO:             {ID: ForkDAO, AlwaysActive: true, Description: "DAO fork"},
		ForkTangerineWhistle: {ID: ForkTangerineWhistle, AlwaysActive: true, Description: "EIP-150 gas repricing"},
		ForkSpuriousDragon:  {ID: ForkSpuriousDragon, AlwaysActive: true, Description: "EIP-155/158 replay protection"},
		ForkByzantium:       {ID: ForkByzantium, AlwaysActive: true, Description: "Byzantium"},
		ForkConstantinople:  {ID: ForkConstantinople, AlwaysActive: true, Description: "Constantinople"},
		ForkPetersburg:      {ID: ForkPetersburg, AlwaysActive: true, Description: "Petersburg"},
		ForkIstanbul:        {ID: ForkIstanbul, AlwaysActive: true, Description: "Istanbul"},
		ForkMuirGlacier:     {ID: ForkMuirGlacier, AlwaysActive: true, Description: "Muir Glacier"},
		ForkBerlin:          {ID: ForkBerlin, AlwaysActive: true, Description: "Berlin"},
		ForkLondon:          {ID: ForkLondon, AlwaysActive: true, Description: "London (EIP-1559)"},
		ForkShanghai:        {ID: ForkShanghai, AlwaysActive: true, Description: "Shanghai"},
		ForkCancun:          {ID: ForkCancun, AlwaysActive: true, Description: "Cancun"},

		// Lux forks - all enabled from genesis
		ForkEVM:     {ID: ForkEVM, AlwaysActive: true, Description: "Base EVM compatibility"},
		ForkDurango: {ID: ForkDurango, AlwaysActive: true, Description: "Dynamic fees"},
		ForkEtna:    {ID: ForkEtna, AlwaysActive: true, Description: "State sync improvements"},
		ForkFortuna: {ID: ForkFortuna, AlwaysActive: true, Description: "Fortuna upgrade"},
		ForkGranite: {ID: ForkGranite, AlwaysActive: true, Description: "Granite upgrade"},
	},
}

// IsActive checks if a fork is active at the given block number and timestamp
func (r *Registry) IsActive(id ForkID, blockNum *big.Int, timestamp uint64) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fork, exists := r.forks[id]
	if !exists {
		return false
	}

	if fork.AlwaysActive {
		return true
	}

	// Check block-based activation
	if fork.Block != nil && blockNum != nil {
		return blockNum.Cmp(fork.Block) >= 0
	}

	// Check time-based activation
	if fork.Timestamp != nil {
		return timestamp >= *fork.Timestamp
	}

	return false
}

// Register adds a new fork to the registry
func (r *Registry) Register(fork *Fork) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.forks[fork.ID] = fork
}

// Get returns a fork definition by ID
func (r *Registry) Get(id ForkID) (*Fork, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	fork, exists := r.forks[id]
	return fork, exists
}

// ScheduleFork schedules a future fork at the given timestamp
func (r *Registry) ScheduleFork(id ForkID, timestamp uint64, description string) {
	r.Register(&Fork{
		ID:          id,
		Timestamp:   &timestamp,
		Description: description,
	})
}

// Global convenience functions

// IsActive checks if a fork is active using the default registry
func IsActive(id ForkID, blockNum *big.Int, timestamp uint64) bool {
	return DefaultRegistry.IsActive(id, blockNum, timestamp)
}

// ScheduleFork schedules a future fork using the default registry
func ScheduleFork(id ForkID, timestamp uint64, description string) {
	DefaultRegistry.ScheduleFork(id, timestamp, description)
}