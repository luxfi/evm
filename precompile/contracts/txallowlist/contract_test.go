// (c) 2020-2023, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package txallowlist

import (
	"testing"
	"github.com/luxfi/evm/v2/core/extstate/testhelpers"
	"github.com/luxfi/evm/v2/precompile/allowlist"
)

func TestTxAllowListRun(t *testing.T) {
	allowlist.RunPrecompileWithAllowListTests(t, Module, testhelpers.NewTestStateDB, nil)
}

func BenchmarkTxAllowList(b *testing.B) {
	allowlist.BenchPrecompileWithAllowList(b, Module, testhelpers.NewTestStateDB, nil)
}
