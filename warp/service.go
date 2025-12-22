// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package warp

import (
	"context"
	"errors"
	"fmt"

	consensuscontext "github.com/luxfi/consensus/context"
	"github.com/luxfi/geth/common/hexutil"
	"github.com/luxfi/geth/log"
	"github.com/luxfi/ids"
	"github.com/luxfi/warp"
	"github.com/luxfi/warp/payload"
)

var errNoValidators = errors.New("cannot aggregate signatures from network with no validators")

// API introduces chain specific functionality to the evm
type API struct {
	chainContext                 context.Context
	backend                      Backend
	signatureAggregator          interface{} // TODO: implement signature aggregator
	requirePrimaryNetworkSigners func() bool
}

func NewAPI(chainCtx context.Context, backend Backend, signatureAggregator interface{}, requirePrimaryNetworkSigners func() bool) *API {
	return &API{
		backend:                      backend,
		chainContext:                 chainCtx,
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
	chainID := consensuscontext.GetChainID(a.chainContext)
	unsignedMessage, err := warp.NewUnsignedMessage(consensuscontext.GetNetworkID(a.chainContext), chainID, blockHashPayload.Bytes())
	if err != nil {
		return nil, err
	}

	return a.aggregateSignatures(ctx, unsignedMessage, quorumNum, chainIDStr)
}

func (a *API) aggregateSignatures(ctx context.Context, unsignedMessage *warp.UnsignedMessage, quorumNum uint64, chainIDStr string) (hexutil.Bytes, error) {
	chainID := consensuscontext.GetChainID(a.chainContext)
	if len(chainIDStr) > 0 {
		cid, err := ids.FromString(chainIDStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse chainID: %q", chainIDStr)
		}
		chainID = cid
	}
	validatorState := consensuscontext.GetValidatorState(a.chainContext)
	pChainHeight, err := validatorState.GetCurrentHeight(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: implement validator state wrapper with luxfi/warp
	_ = validatorState
	// TODO: implement GetCanonicalValidatorSet with luxfi/warp
	validators := make(map[ids.NodeID]uint64)
	totalWeight := uint64(0)
	err = nil // errors.New("GetCanonicalValidatorSet not yet implemented")
	if err != nil {
		return nil, fmt.Errorf("failed to get validator set: %w", err)
	}
	if len(validators) == 0 {
		return nil, fmt.Errorf("%w (ChainID: %s, Height: %d)", errNoValidators, chainID, pChainHeight)
	}

	log.Debug("Fetching signature",
		"sourceChainID", chainID,
		"height", pChainHeight,
		"numValidators", len(validators),
		"totalWeight", totalWeight,
	)
	// TODO: implement signature aggregation with luxfi/warp
	// For now, return an error
	return nil, errors.New("signature aggregation not yet implemented with luxfi/warp")
}
