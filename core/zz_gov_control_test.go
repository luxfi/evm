package core

import (
	"testing"

	"github.com/luxfi/evm/consensus/dummy"
	"github.com/luxfi/evm/gov"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/vm"
	"github.com/stretchr/testify/require"
)

// CONTROL: identical shape, governance OFF. If this also dies at 2017 the
// failure is the harness, not the mechanism.
func TestZZControlNoGovernance2026Blocks(t *testing.T) {
	config := forkOf96369(t)
	params.GetExtra(config).GovernanceTimestamp = nil
	gspec := govGenesis(config)
	engine := dummy.NewCoinbaseFaker()
	total := int(gov.EpochLength) + 10
	db, blocks, _, err := GenerateChainWithGenesis(gspec, engine, total, 2, func(i int, b *BlockGen) {})
	require.NoError(t, err)
	chain, err := NewBlockChain(db, DefaultCacheConfig, gspec, engine, vm.Config{}, common.Hash{}, false, nil)
	require.NoError(t, err)
	defer chain.Stop()
	for start := 0; start < len(blocks); start += 256 {
		end := min(start+256, len(blocks))
		n, err := chain.InsertChain(blocks[start:end])
		require.NoErrorf(t, err, "CONTROL inserted %d of %d in chunk [%d,%d)", n, end-start, start, end)
		for _, b := range blocks[start:end] {
			require.NoError(t, chain.Accept(b))
		}
		chain.DrainAcceptorQueue()
	}
	t.Logf("CONTROL inserted all %d blocks with governance OFF", len(blocks))
}
