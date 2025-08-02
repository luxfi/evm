// (c) 2020-2023, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package deployerallowlist_test

import (
	"testing"
	"github.com/luxfi/evm/core/extstate/testhelpers"
	"github.com/luxfi/evm/precompile/allowlist"
	"github.com/luxfi/evm/precompile/contracts/deployerallowlist"
)

func TestContractDeployerAllowListRun(t *testing.T) {
	allowlist.RunPrecompileWithAllowListTests(t, deployerallowlist.Module, testhelpers.NewTestStateDB, nil)
}

func BenchmarkContractDeployerAllowList(b *testing.B) {
	allowlist.BenchPrecompileWithAllowList(b, deployerallowlist.Module, testhelpers.NewTestStateDB, nil)
}
