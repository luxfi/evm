package eth

import (
	"context"
	"math/big"

	"github.com/luxfi/evm/v2/core/state"
	"github.com/luxfi/evm/v2/core/types"
	"github.com/luxfi/evm/v2/rpc"
	"github.com/luxfi/geth/common"
	gethrpc "github.com/luxfi/geth/rpc"
)

// ethAPIBackendWrapper wraps EthAPIBackend to implement ethapi.Backend with geth RPC types
type ethAPIBackendWrapper struct {
	*EthAPIBackend
}

func (w *ethAPIBackendWrapper) BlockByNumber(ctx context.Context, number gethrpc.BlockNumber) (*types.Block, error) {
	// Convert geth rpc.BlockNumber to evm rpc.BlockNumber
	evmNumber := rpc.BlockNumber(number)
	return w.EthAPIBackend.BlockByNumber(ctx, evmNumber)
}

func (w *ethAPIBackendWrapper) BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	return w.EthAPIBackend.BlockByHash(ctx, hash)
}

func (w *ethAPIBackendWrapper) HeaderByNumber(ctx context.Context, number gethrpc.BlockNumber) (*types.Header, error) {
	// Convert geth rpc.BlockNumber to evm rpc.BlockNumber
	evmNumber := rpc.BlockNumber(number)
	return w.EthAPIBackend.HeaderByNumber(ctx, evmNumber)
}

func (w *ethAPIBackendWrapper) GetReceipts(ctx context.Context, hash common.Hash) (types.Receipts, error) {
	return w.EthAPIBackend.GetReceipts(ctx, hash)
}

func (w *ethAPIBackendWrapper) GetBody(ctx context.Context, hash common.Hash, number gethrpc.BlockNumber) (*types.Body, error) {
	// Convert geth rpc.BlockNumber to evm rpc.BlockNumber
	evmNumber := rpc.BlockNumber(number)
	return w.EthAPIBackend.GetBody(ctx, hash, evmNumber)
}

func (w *ethAPIBackendWrapper) BlockByNumberOrHash(ctx context.Context, blockNrOrHash gethrpc.BlockNumberOrHash) (*types.Block, error) {
	// Convert geth BlockNumberOrHash to evm BlockNumberOrHash
	evmBlockNrOrHash := rpc.BlockNumberOrHash{
		BlockNumber: (*rpc.BlockNumber)(blockNrOrHash.BlockNumber),
		BlockHash: blockNrOrHash.BlockHash,
		RequireCanonical: blockNrOrHash.RequireCanonical,
	}
	return w.EthAPIBackend.BlockByNumberOrHash(ctx, evmBlockNrOrHash)
}

func (w *ethAPIBackendWrapper) FeeHistory(ctx context.Context, blockCount uint64, lastBlock gethrpc.BlockNumber, rewardPercentiles []float64) (*big.Int, [][]*big.Int, []*big.Int, []float64, error) {
	// Convert geth rpc.BlockNumber to evm rpc.BlockNumber
	evmLastBlock := rpc.BlockNumber(lastBlock)
	return w.EthAPIBackend.FeeHistory(ctx, blockCount, evmLastBlock, rewardPercentiles)
}

func (w *ethAPIBackendWrapper) HeaderByNumberOrHash(ctx context.Context, blockNrOrHash gethrpc.BlockNumberOrHash) (*types.Header, error) {
	// Convert geth BlockNumberOrHash to evm BlockNumberOrHash
	evmBlockNrOrHash := rpc.BlockNumberOrHash{
		BlockNumber: (*rpc.BlockNumber)(blockNrOrHash.BlockNumber),
		BlockHash: blockNrOrHash.BlockHash,
		RequireCanonical: blockNrOrHash.RequireCanonical,
	}
	return w.EthAPIBackend.HeaderByNumberOrHash(ctx, evmBlockNrOrHash)
}

func (w *ethAPIBackendWrapper) StateAndHeaderByNumber(ctx context.Context, number gethrpc.BlockNumber) (*state.StateDB, *types.Header, error) {
	// Convert geth rpc.BlockNumber to evm rpc.BlockNumber
	evmNumber := rpc.BlockNumber(number)
	return w.EthAPIBackend.StateAndHeaderByNumber(ctx, evmNumber)
}

func (w *ethAPIBackendWrapper) StateAndHeaderByNumberOrHash(ctx context.Context, blockNrOrHash gethrpc.BlockNumberOrHash) (*state.StateDB, *types.Header, error) {
	// Convert geth BlockNumberOrHash to evm BlockNumberOrHash
	evmBlockNrOrHash := rpc.BlockNumberOrHash{
		BlockNumber: (*rpc.BlockNumber)(blockNrOrHash.BlockNumber),
		BlockHash: blockNrOrHash.BlockHash,
		RequireCanonical: blockNrOrHash.RequireCanonical,
	}
	return w.EthAPIBackend.StateAndHeaderByNumberOrHash(ctx, evmBlockNrOrHash)
}

// gpoBackendWrapper wraps EthAPIBackend for the gas price oracle
type gpoBackendWrapper struct {
	*EthAPIBackend
}

func (w *gpoBackendWrapper) BlockByNumber(ctx context.Context, number gethrpc.BlockNumber) (*types.Block, error) {
	// Convert geth rpc.BlockNumber to evm rpc.BlockNumber
	evmNumber := rpc.BlockNumber(number)
	return w.EthAPIBackend.BlockByNumber(ctx, evmNumber)
}

func (w *gpoBackendWrapper) HeaderByNumber(ctx context.Context, number gethrpc.BlockNumber) (*types.Header, error) {
	// Convert geth rpc.BlockNumber to evm rpc.BlockNumber
	evmNumber := rpc.BlockNumber(number)
	return w.EthAPIBackend.HeaderByNumber(ctx, evmNumber)
}

func (w *gpoBackendWrapper) GetReceipts(ctx context.Context, hash common.Hash) (types.Receipts, error) {
	return w.EthAPIBackend.GetReceipts(ctx, hash)
}