// (c) 2023, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensus

// BlockContext defines the block context that will be optionally provided by the
// proposervm to an underlying vm.
type BlockContext struct {
	// PChainHeight is the height that this block will use to verify it's state.
	// In the proposervm, blocks verify the proposer based on the P-chain height
	// recorded in the parent block. However, the P-chain height provided here
	// is the P-chain height encoded into this block.
	PChainHeight uint64
}