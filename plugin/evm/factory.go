// (c) 2020-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"github.com/luxfi/evm/iface"
	luxlog "github.com/luxfi/log"
	"github.com/luxfi/node/v2/vms"
)

var (
	// ID this VM should be referenced by
	IDStr = "subnetevm"
	ID    = iface.ID{'s', 'u', 'b', 'n', 'e', 't', 'e', 'v', 'm'}

	_ vms.Factory = &Factory{}
)

type Factory struct{}

func (*Factory) New(luxlog.Logger) (interface{}, error) {
	return &VM{}, nil
}
