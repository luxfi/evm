// Copyright (C) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package validators

import (
	"fmt"
	"time"

	validatorinterfaces "github.com/luxfi/evm/plugin/evm/validators/interfaces"
	stateinterfaces "github.com/luxfi/evm/plugin/evm/validators/state/interfaces"
	"github.com/luxfi/node/ids"
)

type RLocker interface {
	RLock()
	RUnlock()
}

type lockedReader struct {
	manager validatorinterfaces.Manager
	lock    RLocker
}

// NewLockedValidatorReader returns a ValidatorReader that holds the given lock
// while accessing validator state.
func NewLockedValidatorReader(
	manager validatorinterfaces.Manager,
	lock RLocker,
) validatorinterfaces.ValidatorReader {
	return &lockedReader{
		manager: manager,
		lock:    lock,
	}
}

// GetValidatorAndUptime returns the calculated uptime of the validator specified by validationID
// and the last updated time.
// GetValidatorAndUptime holds the lock while performing the operation and can be called concurrently.
func (l *lockedReader) GetValidatorAndUptime(validationID ids.ID) (stateinterfaces.Validator, time.Duration, time.Time, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()

	vdr, err := l.manager.GetValidator(validationID)
	if err != nil {
		return stateinterfaces.Validator{}, 0, time.Time{}, fmt.Errorf("failed to get validator: %w", err)
	}

	uptime, lastUpdated, err := l.manager.CalculateUptime(vdr.NodeID)
	if err != nil {
		return stateinterfaces.Validator{}, 0, time.Time{}, fmt.Errorf("failed to get uptime: %w", err)
	}

	return vdr, uptime, lastUpdated, nil
}
