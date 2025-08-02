// Copyright (C) 2023, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package precompile

import (
	"os"
	"testing"

	ginkgo "github.com/onsi/ginkgo/v2"

	// Import the solidity package, so that ginkgo maps out the tests declared within the package
	"github.com/luxfi/evm/v2/tests/precompile/solidity"
)

func TestE2E(t *testing.T) {
	if basePath := os.Getenv("TEST_SOURCE_ROOT"); basePath != "" {
		os.Chdir(basePath)
	}
	solidity.RegisterAsyncTests()
	ginkgo.RunSpecs(t, "evm precompile ginkgo test suite")
}
