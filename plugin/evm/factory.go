// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"github.com/luxfi/ids"
	log "github.com/luxfi/log"
	"github.com/luxfi/vm"
)

var (
	// ID this VM should be referenced by
	IDStr = "evm"
	ID    = ids.ID{'e', 'v', 'm'}

	_ vm.Factory = (*Factory)(nil)
)

type Factory struct{}

func (*Factory) New(log.Logger) (interface{}, error) {
	return &VM{}, nil
}
