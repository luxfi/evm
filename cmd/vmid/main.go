// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// vmid prints the canonical EVM VM ID derived from luxfi/constants.EVMID.
// Used by build/install scripts so no shell file hardcodes the base58 string.
package main

import (
	"fmt"

	"github.com/luxfi/constants"
)

func main() {
	fmt.Println(constants.EVMID)
}
