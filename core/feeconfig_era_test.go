package core

import (
	"math/big"
	"testing"

	"github.com/luxfi/evm/commontype"
)

// TestMainnetEraFeeConfig validates the era-aware BlockGasCostStep override that reproduces
// the historical fleet-wide feeConfig persist-bug drop (200000->0) at T_36 on the Lux mainnet
// C-Chain (96369), per the scientist's byte-validated spec:
//   - RAW timestamp gate (parent.Time >= 1783900524), NOT config.IsGranite.
//   - step 200000 while parent < T_36, step 0 from T_36 onward.
//   - ONLY BlockGasCostStep is overridden; every other field is preserved.
//   - scoped strictly to chainID 96369.
func TestMainnetEraFeeConfig(t *testing.T) {
	base := commontype.FeeConfig{
		GasLimit:                 big.NewInt(12_000_000),
		TargetBlockRate:          2,
		MinBaseFee:               big.NewInt(25_000_000_000),
		TargetGas:                big.NewInt(500_000_000),
		BaseFeeChangeDenominator: big.NewInt(36),
		MinBlockGasCost:          big.NewInt(0),
		MaxBlockGasCost:          big.NewInt(1_000_000),
		BlockGasCostStep:         big.NewInt(0), // simulates the persisted/dropped step
	}
	mainnet := big.NewInt(96369)

	assertOtherFieldsPreserved := func(t *testing.T, got commontype.FeeConfig) {
		t.Helper()
		if got.GasLimit.Cmp(base.GasLimit) != 0 || got.MinBaseFee.Cmp(base.MinBaseFee) != 0 ||
			got.TargetGas.Cmp(base.TargetGas) != 0 || got.BaseFeeChangeDenominator.Cmp(base.BaseFeeChangeDenominator) != 0 ||
			got.MinBlockGasCost.Cmp(base.MinBlockGasCost) != 0 || got.MaxBlockGasCost.Cmp(base.MaxBlockGasCost) != 0 ||
			got.TargetBlockRate != base.TargetBlockRate {
			t.Fatalf("non-step fields must be preserved; got %+v", got)
		}
	}

	// pre-T_36 on the mainnet C-Chain: step overridden to 200000, all other fields preserved.
	pre := mainnetEraFeeConfig(mainnet, t36FeeStepDropTime-1, base)
	if pre.BlockGasCostStep.Cmp(big.NewInt(200_000)) != 0 {
		t.Fatalf("pre-T_36 step = %s, want 200000", pre.BlockGasCostStep)
	}
	assertOtherFieldsPreserved(t, pre)

	// boundary exactness: parent.Time == T_36 -> 0 (parent >= T_36); parent == T_36-1 -> 200000.
	if s := mainnetEraFeeConfig(mainnet, t36FeeStepDropTime, base).BlockGasCostStep; s.Sign() != 0 {
		t.Fatalf("parent==T_36 step = %s, want 0", s)
	}
	if s := mainnetEraFeeConfig(mainnet, t36FeeStepDropTime-1, base).BlockGasCostStep; s.Cmp(big.NewInt(200_000)) != 0 {
		t.Fatalf("parent==T_36-1 step = %s, want 200000", s)
	}

	// post-T_36: step 0, other fields preserved.
	post := mainnetEraFeeConfig(mainnet, t36FeeStepDropTime+1000, base)
	if post.BlockGasCostStep.Sign() != 0 {
		t.Fatalf("post-T_36 step = %s, want 0", post.BlockGasCostStep)
	}
	assertOtherFieldsPreserved(t, post)

	// any other chain is untouched, even after T_36 (must not misfire on testnet/devnet/L2s).
	if s := mainnetEraFeeConfig(big.NewInt(43114), t36FeeStepDropTime+1000, base).BlockGasCostStep; s.Cmp(base.BlockGasCostStep) != 0 {
		t.Fatalf("non-mainnet chain must be untouched, got step %s", s)
	}
	if s := mainnetEraFeeConfig(nil, t36FeeStepDropTime-1, base).BlockGasCostStep; s.Cmp(base.BlockGasCostStep) != 0 {
		t.Fatal("nil chainID must be untouched")
	}

	// the input feeConfig must never be mutated.
	if base.BlockGasCostStep.Sign() != 0 {
		t.Fatal("input feeConfig BlockGasCostStep was mutated")
	}
}
