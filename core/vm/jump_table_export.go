// Copyright 2023 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package vm

import (
	"github.com/luxfi/evm/params"
)

// LookupInstructionSet returns the instruction set for the fork configured by
// the rules.
func LookupInstructionSet(rules params.Rules) (JumpTable, error) {
	// For Lux v1, all upgrades are active from genesis
	// Use the most modern instruction set available
	if rules.IsCancun {
		// Use Cancun if available (latest Ethereum fork)
		return newCancunInstructionSet(), nil
	}
	// Otherwise use Durango (latest Avalanche fork, includes all prior upgrades)
	return newDurangoInstructionSet(), nil
}

