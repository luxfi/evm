// (c) 2024, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package warp

import (
	"context"
	"fmt"

	"github.com/luxfi/evm/warp/messages"
	"github.com/luxfi/node/ids"
	"github.com/luxfi/node/utils/crypto/bls"
	"github.com/luxfi/node/vms/platformvm/warp"
	"github.com/luxfi/node/vms/platformvm/warp/payload"
	"github.com/luxfi/node/database"
)

const (
	ParseErrCode = iota + 1
	VerifyErrCode
)

// Verify verifies the signature of the message
// It also implements the warp.Verifier interface
func (b *backend) Verify(ctx context.Context, unsignedMessage *warp.UnsignedMessage, _ []byte) error {
	messageID := unsignedMessage.ID()
	// Known on-chain messages should be signed
	if _, err := b.GetMessage(messageID); err == nil {
		return nil
	} else if err != database.ErrNotFound {
		return fmt.Errorf("failed to get message %s: %w", messageID, err)
	}

	parsed, err := payload.Parse(unsignedMessage.Payload)
	if err != nil {
		b.stats.IncMessageParseFail()
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	switch p := parsed.(type) {
	case *payload.AddressedCall:
		return b.verifyOffchainAddressedCall(p)
	case *payload.Hash:
		return b.verifyBlockMessage(ctx, p.Bytes())
	default:
		b.stats.IncMessageParseFail()
		return fmt.Errorf("unknown payload type: %T", p)
	}
}

// verifyBlockMessage returns nil if blockHashPayload contains the ID
// of an accepted block indicating it should be signed by the VM.
func (b *backend) verifyBlockMessage(ctx context.Context, blockHashPayload []byte) error {
	if len(blockHashPayload) != 32 {
		return fmt.Errorf("invalid block hash length: expected 32, got %d", len(blockHashPayload))
	}
	blockID, err := ids.ToID(blockHashPayload)
	if err != nil {
		return fmt.Errorf("failed to parse block ID: %w", err)
	}
	_, err = b.blockClient.GetAcceptedBlock(ctx, blockID)
	if err != nil {
		b.stats.IncBlockValidationFail()
		return fmt.Errorf("failed to get block %s: %w", blockID, err)
	}

	return nil
}

// verifyOffchainAddressedCall verifies the addressed call message
func (b *backend) verifyOffchainAddressedCall(addressedCall *payload.AddressedCall) error {
	// Further, parse the payload to see if it is a known type.
	parsed, err := messages.Parse(addressedCall.Payload)
	if err != nil {
		b.stats.IncMessageParseFail()
		return fmt.Errorf("failed to parse addressed call message: %w", err)
	}

	if len(addressedCall.SourceAddress) != 0 {
		return fmt.Errorf("source address should be empty for offchain addressed messages")
	}

	switch p := parsed.(type) {
	case *messages.ValidatorUptime:
		if err := b.verifyUptimeMessage(p); err != nil {
			b.stats.IncUptimeValidationFail()
			return err
		}
	default:
		b.stats.IncMessageParseFail()
		return fmt.Errorf("unknown message type: %T", p)
	}

	return nil
}

func (b *backend) verifyUptimeMessage(uptimeMsg *messages.ValidatorUptime) error {
	// FIXME: GetValidatorAndUptime method doesn't exist in interfaces.State interface
	// vdr, currentUptime, _, err := b.validatorReader.GetValidatorAndUptime(uptimeMsg.ValidationID)
	b.stats.IncUptimeValidationFail()
	return fmt.Errorf("uptime verification not implemented")
}
