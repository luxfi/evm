// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package dummy

import (
	"math/big"
	"testing"

	"github.com/luxfi/evm/commontype"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/evm/plugin/evm/vmerrors"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
	"github.com/stretchr/testify/require"
)

// coinbaseReader is the minimum ChainHeaderReader verifyCoinbase needs: the
// configured fee destination at the parent, and whether per-block fee recipients
// are allowed. On a real chain both come from BlockChain.GetCoinbaseAt, i.e. from
// the RewardManager precompile's state.
type coinbaseReader struct {
	configured         common.Address
	allowFeeRecipients bool
}

func (r *coinbaseReader) Config() *params.ChainConfig                 { return nil }
func (r *coinbaseReader) CurrentHeader() *types.Header                { return nil }
func (r *coinbaseReader) GetHeader(common.Hash, uint64) *types.Header { return nil }
func (r *coinbaseReader) GetHeaderByNumber(uint64) *types.Header      { return nil }
func (r *coinbaseReader) GetHeaderByHash(common.Hash) *types.Header   { return nil }
func (r *coinbaseReader) GetFeeConfigAt(*types.Header) (commontype.FeeConfig, *big.Int, error) {
	return commontype.FeeConfig{}, nil, nil
}
func (r *coinbaseReader) GetCoinbaseAt(*types.Header) (common.Address, bool, error) {
	return r.configured, r.allowFeeRecipients, nil
}

// TestVerifyCoinbaseEnforcesConfiguredDestination is the reason the fee split may
// credit the header coinbase without creating a validator-capture hole: the
// coinbase is not the proposer's choice, it is part of header validity. A block
// whose header names any address other than the one configured at its parent is
// rejected with ErrInvalidCoinbase, so a validator cannot point the reward half
// of the split at itself.
//
// The single exception is allowFeeRecipients mode, which is exactly the opt-in
// "block builders keep the fees" policy — also governed, by the same precompile.
func TestVerifyCoinbaseEnforcesConfiguredDestination(t *testing.T) {
	configured := common.HexToAddress("0x8E29b816c6C35b13cE1ff68D33E245C2bda8ac3D")
	attacker := common.HexToAddress("0x00000000000000000000000000000000BadC0DE1")
	parent := &types.Header{Number: big.NewInt(1)}

	eng := NewFaker() // no ModeSkipCoinbase: coinbase verification is ON

	t.Run("matching coinbase accepted", func(t *testing.T) {
		err := eng.verifyCoinbase(
			&types.Header{Number: big.NewInt(2), Coinbase: configured},
			parent,
			&coinbaseReader{configured: configured},
		)
		require.NoError(t, err)
	})

	t.Run("proposer cannot redirect the reward half to itself", func(t *testing.T) {
		err := eng.verifyCoinbase(
			&types.Header{Number: big.NewInt(2), Coinbase: attacker},
			parent,
			&coinbaseReader{configured: configured},
		)
		require.ErrorIs(t, err, vmerrors.ErrInvalidCoinbase)
	})

	t.Run("allowFeeRecipients mode permits any coinbase, by governed policy", func(t *testing.T) {
		err := eng.verifyCoinbase(
			&types.Header{Number: big.NewInt(2), Coinbase: attacker},
			parent,
			&coinbaseReader{configured: common.Address{}, allowFeeRecipients: true},
		)
		require.NoError(t, err)
	})
}
