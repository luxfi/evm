// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package warp

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/luxfi/cache"
	"github.com/luxfi/cache/lru"
	"github.com/luxfi/database"
	"github.com/luxfi/ids"

	"github.com/luxfi/vm/chain"
	"github.com/luxfi/warp"
	"github.com/luxfi/warp/payload"

	"github.com/luxfi/crypto"
	"github.com/luxfi/evm/plugin/evm/validators/interfaces"
	log "github.com/luxfi/log"
)

var (
	_                         Backend = (*backend)(nil)
	errParsingOffChainMessage         = errors.New("failed to parse off-chain message")

	messageCacheSize = 500
)

type BlockClient interface {
	GetAcceptedBlock(ctx context.Context, blockID ids.ID) (chain.Block, error)
}

// Backend tracks signature-eligible warp messages and provides an interface to fetch them.
// The backend is also used to query for warp message signatures by the signature request handler.
//
// Post-ZAP the signed subject is the warp.SignedCore (its D = ID() is what the
// BLS Beam signs via warp.BeamSigningBytes(D)); the deleted RLP
// unsigned-message type is gone.
type Backend interface {
	// AddMessage signs [core] and adds it to the warp backend database.
	AddMessage(core *warp.SignedCore) error

	// GetMessageSignature validates the message and returns the signature of the requested message.
	GetMessageSignature(ctx context.Context, core *warp.SignedCore) ([]byte, error)

	// GetBlockSignature returns the signature of a hash payload containing blockID if it's the ID of an accepted block.
	GetBlockSignature(ctx context.Context, blockID ids.ID) ([]byte, error)

	// GetMessage retrieves the [core] from the warp backend database if available.
	GetMessage(messageHash ids.ID) (*warp.SignedCore, error)

	// Verify verifies the signature of the message.
	Verify(ctx context.Context, core *warp.SignedCore, _ []byte) error
}

// backend implements Backend, keeps track of warp messages, and generates message signatures.
type backend struct {
	networkID                 uint32
	sourceChainID             ids.ID
	db                        database.Database
	warpSigner                warp.Signer
	blockClient               BlockClient
	validatorReader           interfaces.ValidatorReader
	signatureCache            cache.Cacher[ids.ID, []byte]
	messageCache              *lru.Cache[ids.ID, *warp.SignedCore]
	offchainAddressedCallMsgs map[string]*warp.SignedCore
	stats                     *verifierStats
}

// NewBackend creates a new Backend, and initializes the signature cache and message tracking database.
func NewBackend(
	networkID uint32,
	sourceChainID ids.ID,
	warpSigner warp.Signer,
	blockClient BlockClient,
	validatorReader interfaces.ValidatorReader,
	db database.Database,
	signatureCache cache.Cacher[ids.ID, []byte],
	offchainMessages [][]byte,
) (Backend, error) {
	b := &backend{
		networkID:                 networkID,
		sourceChainID:             sourceChainID,
		db:                        db,
		warpSigner:                warpSigner,
		blockClient:               blockClient,
		signatureCache:            signatureCache,
		validatorReader:           validatorReader,
		messageCache:              lru.NewCache[ids.ID, *warp.SignedCore](messageCacheSize),
		stats:                     newVerifierStats(),
		offchainAddressedCallMsgs: make(map[string]*warp.SignedCore),
	}
	return b, b.initOffChainMessages(offchainMessages)
}

func (b *backend) initOffChainMessages(offchainMessages [][]byte) error {
	for i, offchainMsg := range offchainMessages {
		core, err := warp.ParseSignedCore(offchainMsg)
		if err != nil {
			return fmt.Errorf("%w at index %d: %w", errParsingOffChainMessage, i, err)
		}

		if core.NetworkID != b.networkID {
			return fmt.Errorf("wrong network ID at index %d", i)
		}

		// Compare source chain IDs
		if !bytes.Equal(core.SourceChainID[:], b.sourceChainID[:]) {
			return fmt.Errorf("wrong source chain ID at index %d", i)
		}

		_, err = payload.ParsePayload(core.Payload)
		if err != nil {
			return fmt.Errorf("%w at index %d as AddressedCall: %w", errParsingOffChainMessage, i, err)
		}
		messageID := core.ID()
		msgIDHash := ids.ID(crypto.Keccak256Hash(messageID[:]))
		b.offchainAddressedCallMsgs[msgIDHash.String()] = core
	}

	return nil
}

func (b *backend) AddMessage(core *warp.SignedCore) error {
	coreID := core.ID()
	messageID := ids.ID(crypto.Keccak256Hash(coreID[:]))
	log.Debug("Adding warp message to backend", "messageID", messageID)

	// In the case when a node restarts, and possibly changes its bls key, the cache gets emptied but the database does not.
	// So to avoid having incorrect signatures saved in the database after a bls key change, we save the full message in the database.
	// Whereas for the cache, after the node restart, the cache would be emptied so we can directly save the signatures.
	if err := b.db.Put(messageID[:], core.Bytes()); err != nil {
		return fmt.Errorf("failed to put warp signature in db: %w", err)
	}

	if _, err := b.signMessage(core); err != nil {
		return fmt.Errorf("failed to sign warp message: %w", err)
	}
	return nil
}

func (b *backend) GetMessageSignature(ctx context.Context, core *warp.SignedCore) ([]byte, error) {
	coreID := core.ID()
	messageID := ids.ID(crypto.Keccak256Hash(coreID[:]))

	log.Debug("Getting warp message from backend", "messageID", messageID)
	if sig, ok := b.signatureCache.Get(messageID); ok {
		return sig, nil
	}

	if err := b.Verify(ctx, core, nil); err != nil {
		return nil, fmt.Errorf("failed to validate warp message: %w", err)
	}
	return b.signMessage(core)
}

func (b *backend) GetBlockSignature(ctx context.Context, blockID ids.ID) ([]byte, error) {
	log.Debug("Getting block from backend", "blockID", blockID)

	blockHashPayload, err := payload.NewHash(blockID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create new block hash payload: %w", err)
	}

	core, err := warp.NewSignedCore(b.networkID, b.sourceChainID, blockHashPayload.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to create new signed core: %w", err)
	}

	coreID := core.ID()
	messageID := ids.ID(crypto.Keccak256Hash(coreID[:]))
	if sig, ok := b.signatureCache.Get(messageID); ok {
		return sig, nil
	}

	if err := b.verifyBlockMessage(ctx, blockHashPayload); err != nil {
		return nil, fmt.Errorf("failed to validate block message: %w", err)
	}

	sig, err := b.signMessage(core)
	if err != nil {
		return nil, fmt.Errorf("failed to sign block message: %w", err)
	}
	return sig, nil
}

func (b *backend) GetMessage(messageID ids.ID) (*warp.SignedCore, error) {
	if message, ok := b.messageCache.Get(messageID); ok {
		return message, nil
	}
	messageIDStr := messageID.String()
	if message, ok := b.offchainAddressedCallMsgs[messageIDStr]; ok {
		return message, nil
	}

	coreBytes, err := b.db.Get(messageID[:])
	if err != nil {
		return nil, err
	}

	core, err := warp.ParseSignedCore(coreBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse signed core %s: %w", messageID.String(), err)
	}
	b.messageCache.Put(messageID, core)

	return core, nil
}

func (b *backend) signMessage(core *warp.SignedCore) ([]byte, error) {
	if b.warpSigner == nil {
		return nil, errors.New("warp signer not configured")
	}

	sig, err := b.warpSigner.Sign(core)
	if err != nil {
		return nil, fmt.Errorf("failed to sign message: %w", err)
	}

	coreID := core.ID()
	messageID := ids.ID(crypto.Keccak256Hash(coreID[:]))
	b.signatureCache.Put(messageID, sig)
	return sig, nil
}
