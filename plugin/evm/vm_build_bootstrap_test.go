// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildBlockBeforeNormalOperationsFailsClosed(t *testing.T) {
	vm := &VM{}

	block, err := vm.buildBlockWithContext(context.Background(), nil)
	require.ErrorIs(t, err, errBlockBuildingNotReady)
	require.Nil(t, block)
}
