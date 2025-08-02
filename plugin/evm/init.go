// (c) 2020-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"github.com/luxfi/evm/v2/iface"
)

func init() {
	// Register the EVM plugin
	iface.RegisterPlugin("evm", &vmFactory{})
}

type vmFactory struct{}

func (f *vmFactory) New() (interface{}, error) {
	return &VM{}, nil
}
