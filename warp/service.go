// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package warp

import (
	"context"
	"errors"
	"fmt"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/geth/common/hexutil"
	"github.com/luxfi/ids"
	log "github.com/luxfi/log"
	"github.com/luxfi/runtime"
	"github.com/luxfi/validators"
	"github.com/luxfi/warp"
	"github.com/luxfi/warp/payload"

	warpvalidators "github.com/luxfi/evm/warp/validators"
)

var errNoValidators = errors.New("cannot aggregate signatures from network with no validators")

// API introduces chain specific functionality to the evm
type API struct {
	runtimeCtx                   runtime.VMContext
	backend                      Backend
	signatureAggregator          *warp.SignatureAggregator
	requirePrimaryNetworkSigners func() bool
}

func NewAPI(runtimeCtx runtime.VMContext, backend Backend, signatureAggregator *warp.SignatureAggregator, requirePrimaryNetworkSigners func() bool) *API {
	return &API{
		backend:                      backend,
		runtimeCtx:                   runtimeCtx,
		signatureAggregator:          signatureAggregator,
		requirePrimaryNetworkSigners: requirePrimaryNetworkSigners,
	}
}

// GetMessage returns the Warp message associated with a messageID.
func (a *API) GetMessage(ctx context.Context, messageID ids.ID) (hexutil.Bytes, error) {
	message, err := a.backend.GetMessage(messageID)
	if err != nil {
		return nil, fmt.Errorf("failed to get message %s with error %w", messageID, err)
	}
	return hexutil.Bytes(message.Bytes()), nil
}

// GetMessageSignature returns the BLS signature associated with a messageID.
func (a *API) GetMessageSignature(ctx context.Context, messageID ids.ID) (hexutil.Bytes, error) {
	unsignedMessage, err := a.backend.GetMessage(messageID)
	if err != nil {
		return nil, fmt.Errorf("failed to get message %s with error %w", messageID, err)
	}
	signature, err := a.backend.GetMessageSignature(ctx, unsignedMessage)
	if err != nil {
		return nil, fmt.Errorf("failed to get signature for message %s with error %w", messageID, err)
	}
	return signature[:], nil
}

// GetBlockSignature returns the BLS signature associated with a blockID.
func (a *API) GetBlockSignature(ctx context.Context, blockID ids.ID) (hexutil.Bytes, error) {
	signature, err := a.backend.GetBlockSignature(ctx, blockID)
	if err != nil {
		return nil, fmt.Errorf("failed to get signature for block %s with error %w", blockID, err)
	}
	return signature[:], nil
}

// GetMessageAggregateSignature fetches the aggregate signature for the requested [messageID]
func (a *API) GetMessageAggregateSignature(ctx context.Context, messageID ids.ID, quorumNum uint64, chainIDStr string) (signedMessageBytes hexutil.Bytes, err error) {
	unsignedMessage, err := a.backend.GetMessage(messageID)
	if err != nil {
		return nil, err
	}
	return a.aggregateSignatures(ctx, unsignedMessage, quorumNum, chainIDStr)
}

// GetBlockAggregateSignature fetches the aggregate signature for the requested [blockID]
func (a *API) GetBlockAggregateSignature(ctx context.Context, blockID ids.ID, quorumNum uint64, chainIDStr string) (signedMessageBytes hexutil.Bytes, err error) {
	blockHashPayload, err := payload.NewHash(blockID[:])
	if err != nil {
		return nil, err
	}
	chainID := a.runtimeCtx.GetChainID()
	unsignedMessage, err := warp.NewUnsignedMessage(a.runtimeCtx.GetNetworkID(), chainID, blockHashPayload.Bytes())
	if err != nil {
		return nil, err
	}

	return a.aggregateSignatures(ctx, unsignedMessage, quorumNum, chainIDStr)
}

func (a *API) aggregateSignatures(ctx context.Context, unsignedMessage *warp.UnsignedMessage, quorumNum uint64, chainIDStr string) (hexutil.Bytes, error) {
	chainID := a.runtimeCtx.GetChainID()
	if len(chainIDStr) > 0 {
		cid, err := ids.FromString(chainIDStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse chainID: %q", chainIDStr)
		}
		chainID = cid
	}

	// Get validator state from runtime context
	validatorState := a.runtimeCtx.GetValidatorState()
	if validatorState == nil {
		return nil, errors.New("validator state not available")
	}

	// Get current P-chain height
	pChainHeight, err := validatorState.GetCurrentHeight(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current height: %w", err)
	}

	// Create validator state wrapper for warp messaging
	// This handles the special case for Primary Network messages
	sourceChainID := unsignedMessage.SourceChainID
	wrappedState := warpvalidators.NewState(
		validatorState,
		a.runtimeCtx.GetChainID(),
		sourceChainID,
		a.requirePrimaryNetworkSigners(),
	)

	// Get canonical validator set using GetWarpValidatorSet
	warpSet, err := wrappedState.GetWarpValidatorSet(ctx, pChainHeight, chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to get warp validator set: %w", err)
	}

	if warpSet == nil || len(warpSet.Validators) == 0 {
		return nil, fmt.Errorf("%w (ChainID: %s, Height: %d)", errNoValidators, chainID, pChainHeight)
	}

	// Convert WarpSet to warp.Validator slice for the external SignatureAggregator
	warpValidators, totalWeight, err := convertWarpSetToValidators(warpSet)
	if err != nil {
		return nil, fmt.Errorf("failed to convert validator set: %w", err)
	}

	if len(warpValidators) == 0 {
		return nil, fmt.Errorf("%w (ChainID: %s, Height: %d, no validators with BLS keys)", errNoValidators, chainID, pChainHeight)
	}

	log.Debug("Fetching signatures for aggregation",
		"sourceChainID", sourceChainID,
		"targetChainID", chainID,
		"height", pChainHeight,
		"numValidators", len(warpValidators),
		"totalWeight", totalWeight,
	)

	// Check if signature aggregator is available
	if a.signatureAggregator == nil {
		return nil, errors.New("signature aggregator not configured")
	}

	// Create initial message with empty signature for aggregation
	// The SignatureAggregator will collect signatures and update the message
	emptyBitSetSig := &warp.BitSetSignature{
		Signers:   warp.NewBitSet(),
		Signature: [bls.SignatureLen]byte{},
	}
	initialMessage, err := warp.NewMessage(unsignedMessage, emptyBitSetSig)
	if err != nil {
		return nil, fmt.Errorf("failed to create initial message: %w", err)
	}

	// Aggregate signatures from validators
	// quorumNum is the numerator (e.g., 67 for 67%), denominator is 100
	const quorumDen uint64 = 100
	signedMessage, _, _, err := a.signatureAggregator.AggregateSignatures(
		ctx,
		initialMessage,
		nil, // justification - not needed for standard warp messages
		warpValidators,
		quorumNum,
		quorumDen,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate signatures: %w", err)
	}

	return signedMessage.Bytes(), nil
}

// convertWarpSetToValidators converts a validators.WarpSet to a slice of warp.Validator
// for use with the external SignatureAggregator
func convertWarpSetToValidators(warpSet *validators.WarpSet) ([]*warp.Validator, uint64, error) {
	if warpSet == nil {
		return nil, 0, errors.New("nil warp set")
	}

	warpValidators := make([]*warp.Validator, 0, len(warpSet.Validators))
	var totalWeight uint64

	for nodeID, warpValidator := range warpSet.Validators {
		if warpValidator == nil {
			continue
		}

		// Skip validators without BLS public keys (required for warp signing)
		if len(warpValidator.PublicKey) == 0 {
			log.Debug("Skipping validator without BLS public key",
				"nodeID", nodeID,
			)
			continue
		}

		// Parse BLS public key from bytes
		pubKey, err := bls.PublicKeyFromCompressedBytes(warpValidator.PublicKey)
		if err != nil {
			log.Debug("Skipping validator with invalid BLS public key",
				"nodeID", nodeID,
				"error", err,
			)
			continue
		}

		// Create warp.Validator for the external SignatureAggregator
		validator := warp.NewValidator(
			pubKey,
			warpValidator.PublicKey,
			warpValidator.Weight,
			nodeID,
		)
		warpValidators = append(warpValidators, validator)
		totalWeight += warpValidator.Weight
	}

	return warpValidators, totalWeight, nil
}
