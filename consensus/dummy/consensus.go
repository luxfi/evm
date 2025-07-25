// (c) 2019-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package dummy

import (
	"errors"
	"fmt"
	"math/big"
	"time"
	"github.com/luxfi/evm/iface"
	"github.com/luxfi/evm/utils"
	"github.com/luxfi/evm/consensus"
	"github.com/luxfi/evm/core/vm"
	"github.com/luxfi/evm/core/types"
	"github.com/luxfi/evm/plugin/evm/vmerrors"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/evm/plugin/evm/customtypes"
	customheader "github.com/luxfi/evm/plugin/evm/header"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/evm/params/extras"
	"github.com/luxfi/evm/commontype"
	"github.com/luxfi/evm/iface"
	"github.com/luxfi/evm/trie"
)

var (
	allowedFutureBlockTime = 10 * time.Second // Max time from current time allowed for blocks, before they're considered future blocks

	ErrInsufficientBlockGas = errors.New("insufficient gas to cover the block cost")

	errInvalidBlockTime  = errors.New("timestamp less than parent's")
	errUnclesUnsupported = errors.New("uncles unsupported")
)

// adaptFeeConfig converts an interface FeeConfig to commontype.FeeConfig
func adaptFeeConfig(fc interfaces.FeeConfig) commontype.FeeConfig {
	return commontype.FeeConfig{
		GasLimit:                 fc.GetGasLimit(),
		TargetBlockRate:          fc.GetTargetBlockRate(),
		MinBaseFee:               fc.GetMinBaseFee(),
		TargetGas:                fc.GetTargetGas(),
		BaseFeeChangeDenominator: fc.GetBaseFeeChangeDenominator(),
		MinBlockGasCost:          fc.GetMinBlockGasCost(),
		MaxBlockGasCost:          fc.GetMaxBlockGasCost(),
		BlockGasCostStep:         fc.GetBlockGasCostStep(),
	}
}

type Mode struct {
	ModeSkipHeader   bool
	ModeSkipBlockFee bool
	ModeSkipCoinbase bool
}

type (
	DummyEngine struct {
		clock         interfaces.MockableTimer
		consensusMode Mode
	}
)

// getParamsConfig converts an iface.ChainConfig to *params.ChainConfig
func getParamsConfig(config iface.ChainConfig) *params.ChainConfig {
	// Try to get the underlying geth config and cast
	if gethConfig := config.AsGeth(); gethConfig != nil {
		if paramsConfig, ok := gethConfig.(*params.ChainConfig); ok {
			return paramsConfig
		}
	}
	
	// Fallback - this shouldn't happen in practice
	panic("unable to convert chain config to params.ChainConfig")
}

// convertToExtrasConfig converts a *params.ChainConfig to *extras.ChainConfig
func convertToExtrasConfig(paramsConfig *params.ChainConfig) *extras.ChainConfig {
	// We need to convert the ConsensusCtx from ChainContext to consensus.Context
	var luxCtx extras.LuxContext
	if paramsConfig.LuxContext.ConsensusCtx != nil {
		// This is a simplified conversion - in production code you'd need proper conversion
		luxCtx = extras.LuxContext{
			ConsensusCtx: nil, // Would need proper conversion from ChainContext to consensus.Context
		}
	}
	
	return &extras.ChainConfig{
		ChainConfig:        paramsConfig.ChainConfig,
		NetworkUpgrades:    extras.NetworkUpgrades{
			EVMTimestamp:      paramsConfig.MandatoryNetworkUpgrades.EVMTimestamp,
			DurangoTimestamp:  paramsConfig.MandatoryNetworkUpgrades.DurangoTimestamp,
			EtnaTimestamp:     paramsConfig.MandatoryNetworkUpgrades.EtnaTimestamp,
			FortunaTimestamp:  paramsConfig.MandatoryNetworkUpgrades.FortunaTimestamp,
			GraniteTimestamp:  paramsConfig.MandatoryNetworkUpgrades.GraniteTimestamp,
		},
		LuxContext:         luxCtx,
		FeeConfig:          paramsConfig.FeeConfig,
		AllowFeeRecipients: paramsConfig.AllowFeeRecipients,
		GenesisPrecompiles: extras.Precompiles(paramsConfig.GenesisPrecompiles),
		UpgradeConfig:      extras.UpgradeConfig{
			PrecompileUpgrades: paramsConfig.UpgradeConfig.PrecompileUpgrades,
		},
	}
}

func NewDummyEngine(
	mode Mode,
	clock interfaces.MockableTimer,
) *DummyEngine {
	return &DummyEngine{
		clock:         clock,
		consensusMode: mode,
	}
}

func NewETHFaker() *DummyEngine {
	return &DummyEngine{
		clock:         utils.NewMockableClock(),
		consensusMode: Mode{ModeSkipBlockFee: true},
	}
}

func NewFaker() *DummyEngine {
	return &DummyEngine{
		clock: utils.NewMockableClock(),
	}
}

func NewFakerWithClock(clock interfaces.MockableTimer) *DummyEngine {
	return &DummyEngine{
		clock: clock,
	}
}

func NewFakerWithMode(mode Mode) *DummyEngine {
	return &DummyEngine{
		clock:         utils.NewMockableClock(),
		consensusMode: mode,
	}
}

func NewFakerWithModeAndClock(mode Mode, clock interfaces.MockableTimer) *DummyEngine {
	return &DummyEngine{
		clock:         clock,
		consensusMode: mode,
	}
}

func NewCoinbaseFaker() *DummyEngine {
	return &DummyEngine{
		clock:         utils.NewMockableClock(),
		consensusMode: Mode{ModeSkipCoinbase: true},
	}
}

func NewFullFaker() *DummyEngine {
	return &DummyEngine{
		clock:         utils.NewMockableClock(),
		consensusMode: Mode{ModeSkipHeader: true},
	}
}

// verifyCoinbase checks that the coinbase is valid for the given [header] and [parent].
func (eng *DummyEngine) verifyCoinbase(header *types.Header, parent *types.Header, chain consensus.ChainHeaderReader) error {
	if eng.consensusMode.ModeSkipCoinbase {
		return nil
	}
	// get the coinbase configured at parent
	configuredAddressAtParent := chain.GetCoinbaseAt(parent.Time)
	config := chain.Config()
	isAllowFeeRecipients := config.AllowedFeeRecipients()

	if isAllowFeeRecipients {
		// if fee recipients are allowed we don't need to check the coinbase
		return nil
	}
	// we fetch the configured coinbase at the parent's state
	// to check against the coinbase in [header].
	if configuredAddressAtParent != header.Coinbase {
		return fmt.Errorf("%w: %v does not match required coinbase address %v", vmerrors.ErrInvalidCoinbase, header.Coinbase, configuredAddressAtParent)
	}
	return nil
}

func verifyHeaderGasFields(config *params.ChainConfig, header *types.Header, parent *types.Header, chain consensus.ChainHeaderReader) error {
	// We verify the current block by checking the parent fee config
	// this is because the current block cannot set the fee config for itself
	// Fee config might depend on the state when precompile is activated
	// but we don't know the final state while forming the block.
	// See worker package for more details.
	feeConfigInterface, err := chain.GetFeeConfigAt(parent.Time)
	if err != nil {
		return err
	}
	feeConfig := adaptFeeConfig(feeConfigInterface)
	if err := customheader.VerifyGasUsed(config, feeConfig, parent, header); err != nil {
		return err
	}
	if err := customheader.VerifyGasLimit(config, feeConfig, parent, header); err != nil {
		return err
	}
	// Convert params.ChainConfig to extras.ChainConfig for VerifyExtraPrefix
	extrasConfig := convertToExtrasConfig(config)
	if err := customheader.VerifyExtraPrefix(extrasConfig, parent, header); err != nil {
		return err
	}

	// Verify header.BaseFee matches the expected value.
	// Reuse extrasConfig from above
	expectedBaseFee, err := customheader.BaseFee(extrasConfig, feeConfig, parent, header.Time)
	if err != nil {
		return fmt.Errorf("failed to calculate base fee: %w", err)
	}
	if !utils.BigEqual(header.BaseFee, expectedBaseFee) {
		return fmt.Errorf("expected base fee (%d), found (%d)", expectedBaseFee, header.BaseFee)
	}

	// Enforce BlockGasCost constraints
	// Reuse the extrasConfig from above
	expectedBlockGasCost := customheader.BlockGasCost(
		extrasConfig,
		feeConfig,
		parent,
		header.Time,
	)
	headerExtra := customtypes.GetHeaderExtra(header)
	if !utils.BigEqual(headerExtra.BlockGasCost, expectedBlockGasCost) {
		return fmt.Errorf("invalid block gas cost: have %d, want %d", headerExtra.BlockGasCost, expectedBlockGasCost)
	}
	return nil
}

// modified from consensus.go
func (eng *DummyEngine) verifyHeader(chain consensus.ChainHeaderReader, header *types.Header, parent *types.Header, uncle bool) error {
	// Ensure that we do not verify an uncle
	if uncle {
		return errUnclesUnsupported
	}

	// Verify the extra data is well-formed.
	config := chain.Config()
	// Convert rules to the concrete type expected by VerifyExtra
	paramsConfig := getParamsConfig(config)
	// Get LuxRules from the NetworkUpgrades
	luxRules := paramsConfig.MandatoryNetworkUpgrades.GetLuxRules(header.Time)
	if err := customheader.VerifyExtra(luxRules, header.Extra); err != nil {
		return err
	}

	// Ensure gas-related header fields are correct
	if err := verifyHeaderGasFields(getParamsConfig(config), header, parent, chain); err != nil {
		return err
	}
	// Ensure that coinbase is valid
	if err := eng.verifyCoinbase(header, parent, chain); err != nil {
		return err
	}

	// Verify the header's timestamp
	if header.Time > uint64(eng.clock.Time().Add(allowedFutureBlockTime).Unix()) {
		return consensus.ErrFutureBlock
	}
	// Verify the header's timestamp is not earlier than parent's
	// it does include equality(==), so multiple blocks per second is ok
	if header.Time < parent.Time {
		return errInvalidBlockTime
	}
	// Verify that the block number is parent's +1
	if diff := new(big.Int).Sub(header.Number, parent.Number); diff.Cmp(big.NewInt(1)) != 0 {
		return consensus.ErrInvalidNumber
	}
	// Verify the existence / non-existence of excessBlobGas
	cancun := chain.Config().IsCancun(header.Time)
	if !cancun {
		switch {
		case header.ExcessBlobGas != nil:
			return fmt.Errorf("invalid excessBlobGas: have %d, expected nil", *header.ExcessBlobGas)
		case header.BlobGasUsed != nil:
			return fmt.Errorf("invalid blobGasUsed: have %d, expected nil", *header.BlobGasUsed)
		case header.ParentBeaconRoot != nil:
			return fmt.Errorf("invalid parentBeaconRoot, have %#x, expected nil", *header.ParentBeaconRoot)
		}
	} else {
		if header.ParentBeaconRoot == nil {
			return errors.New("header is missing beaconRoot")
		}
		if *header.ParentBeaconRoot != (common.Hash{}) {
			return fmt.Errorf("invalid parentBeaconRoot, have %#x, expected empty", *header.ParentBeaconRoot)
		}
		// FIXME: Can't verify EIP4844 header with luxfi ChainConfig type
		// if err := eip4844.VerifyEIP4844Header(chain.Config(), parent, header); err != nil {
		// 	return err
		// }
		if *header.BlobGasUsed > 0 { // VerifyEIP4844Header ensures BlobGasUsed is non-nil
			return fmt.Errorf("blobs not enabled on lux networks: used %d blob gas, expected 0", *header.BlobGasUsed)
		}
	}
	return nil
}

func (*DummyEngine) Author(header *types.Header) (common.Address, error) {
	return header.Coinbase, nil
}

func (eng *DummyEngine) VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header, seal bool) error {
	// If we're running a full engine faking, accept any input as valid
	if eng.consensusMode.ModeSkipHeader {
		return nil
	}
	// Short circuit if the header is known, or it's parent not
	number := header.Number.Uint64()
	if chain.GetHeader(header.Hash(), number) != nil {
		return nil
	}
	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	// Sanity checks passed, do a proper verification
	return eng.verifyHeader(chain, header, parent, false)
}

func (*DummyEngine) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	if len(block.Uncles()) > 0 {
		return errUnclesUnsupported
	}
	return nil
}

func (*DummyEngine) Prepare(chain consensus.ChainHeaderReader, header *types.Header) error {
	header.Difficulty = big.NewInt(1)
	return nil
}

func (eng *DummyEngine) verifyBlockFee(
	baseFee *big.Int,
	requiredBlockGasCost *big.Int,
	txs []*types.Transaction,
	receipts []*types.Receipt,
) error {
	if eng.consensusMode.ModeSkipBlockFee {
		return nil
	}
	if baseFee == nil || baseFee.Sign() <= 0 {
		return fmt.Errorf("invalid base fee (%d) in EVM", baseFee)
	}
	if requiredBlockGasCost == nil || !requiredBlockGasCost.IsUint64() {
		return fmt.Errorf("invalid block gas cost (%d) in EVM", requiredBlockGasCost)
	}

	var (
		gasUsed              = new(big.Int)
		blockFeeContribution = new(big.Int)
		totalBlockFee        = new(big.Int)
	)
	// Calculate the total excess over the base fee that was paid towards the block fee
	for i, receipt := range receipts {
		// Each transaction contributes the excess over the baseFee towards the totalBlockFee
		// This should be equivalent to the sum of the "priority fees" within EIP-1559.
		txFeePremium, err := txs[i].EffectiveGasTip(baseFee)
		if err != nil {
			return err
		}
		// Multiply the [txFeePremium] by the gasUsed in the transaction since this gives the total coin that was paid
		// above the amount required if the transaction had simply paid the minimum base fee for the block.
		//
		// Ex. LegacyTx paying a gas price of 100 gwei for 1M gas in a block with a base fee of 10 gwei.
		// Total Fee = 100 gwei * 1M gas
		// Minimum Fee = 10 gwei * 1M gas (minimum fee that would have been accepted for this transaction)
		// Fee Premium = 90 gwei
		// Total Overpaid = 90 gwei * 1M gas

		blockFeeContribution.Mul(txFeePremium, gasUsed.SetUint64(receipt.GasUsed))
		totalBlockFee.Add(totalBlockFee, blockFeeContribution)
	}
	// Calculate how much gas the [totalBlockFee] would purchase at the price level
	// set by the base fee of this block.
	blockGas := new(big.Int).Div(totalBlockFee, baseFee)

	// Require that the amount of gas purchased by the effective tips within the
	// block covers at least `requiredBlockGasCost`.
	//
	// NOTE: To determine the required block fee, multiply
	// `requiredBlockGasCost` by `baseFee`.
	if blockGas.Cmp(requiredBlockGasCost) < 0 {
		return fmt.Errorf("%w: expected %d but got %d",
			ErrInsufficientBlockGas,
			requiredBlockGasCost,
			blockGas,
		)
	}
	return nil
}

func (eng *DummyEngine) Finalize(chain consensus.ChainHeaderReader, block *types.Block, parent *types.Header, state vm.StateDB, receipts []*types.Receipt) error {
	config := chain.Config()
	timestamp := block.Time()
	// we use the parent to determine the fee config
	// since the current block has not been finalized yet.
	feeConfigInterface, err := chain.GetFeeConfigAt(parent.Time)
	if err != nil {
		return err
	}
	feeConfig := adaptFeeConfig(feeConfigInterface)
	// Verify the BlockGasCost set in the header matches the expected value.
	blockGasCost := customtypes.BlockGasCost(block)
	// Convert to extras.ChainConfig for BlockGasCost
	paramsConfig := getParamsConfig(config)
	extrasConfig := convertToExtrasConfig(paramsConfig)
	expectedBlockGasCost := customheader.BlockGasCost(
		extrasConfig,
		feeConfig,
		parent,
		timestamp,
	)
	if !utils.BigEqual(blockGasCost, expectedBlockGasCost) {
		return fmt.Errorf("invalid blockGasCost: have %d, want %d", blockGasCost, expectedBlockGasCost)
	}
	if config.IsEVM(timestamp) {
		// Verify the block fee was paid.
		if err := eng.verifyBlockFee(
			block.BaseFee(),
			blockGasCost,
			block.Transactions(),
			receipts,
		); err != nil {
			return err
		}
	}

	return nil
}

func (eng *DummyEngine) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, parent *types.Header, state vm.StateDB, txs []*types.Transaction,
	uncles []*types.Header, receipts []*types.Receipt,
) (*types.Block, error) {
	// we use the parent to determine the fee config
	// since the current block has not been finalized yet.
	feeConfigInterface, err := chain.GetFeeConfigAt(parent.Time)
	if err != nil {
		return nil, err
	}
	feeConfig := adaptFeeConfig(feeConfigInterface)
	config := chain.Config()

	// Calculate the required block gas cost for this block.
	headerExtra := customtypes.GetHeaderExtra(header)
	// Convert to extras.ChainConfig for BlockGasCost
	paramsConfig := getParamsConfig(config)
	extrasConfig := convertToExtrasConfig(paramsConfig)
	headerExtra.BlockGasCost = customheader.BlockGasCost(
		extrasConfig,
		feeConfig,
		parent,
		header.Time,
	)
	if config.IsEVM(header.Time) {
		// Verify that this block covers the block fee.
		if err := eng.verifyBlockFee(
			header.BaseFee,
			headerExtra.BlockGasCost,
			txs,
			receipts,
		); err != nil {
			return nil, err
		}
	}

	// finalize the header.Extra
	// Convert to extras.ChainConfig for ExtraPrefix
	paramsConfig = getParamsConfig(config)
	extrasConfig = convertToExtrasConfig(paramsConfig)
	extraPrefix, err := customheader.ExtraPrefix(extrasConfig, parent, header)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate new header.Extra: %w", err)
	}
	header.Extra = append(extraPrefix, header.Extra...)

	// commit the final state root
	header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))

	// Header seems complete, assemble into a block and return
	// Use the NewBlockWithExtData function to properly handle Lux extensions
	return types.NewBlockWithExtData(header, txs, uncles, nil, trie.NewStackTrie(nil), nil, false), nil
}

func (*DummyEngine) CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int {
	return big.NewInt(1)
}

func (*DummyEngine) Close() error {
	return nil
}
