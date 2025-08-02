// (c) 2020-2023, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package allowlist_test

import (
	"testing"
	"github.com/luxfi/evm/v2/v2/precompile/allowlist"
	"github.com/luxfi/evm/v2/v2/precompile/registry"
)

var testModule = registry.NewModule(
	"dummy",
	dummyAddr,
	allowlist.CreateAllowListPrecompile(dummyAddr),
	&dummyConfigurator{},
)

func TestVerifyAllowlist(t *testing.T) {
	allowlist.VerifyPrecompileWithAllowListTests(t, testModule, nil)
}

func TestEqualAllowList(t *testing.T) {
	allowlist.EqualPrecompileWithAllowListTests(t, testModule, nil)
}
