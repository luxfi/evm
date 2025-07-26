// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package atomic

import (
	"fmt"

	"github.com/luxfi/node/ids"
	"github.com/luxfi/evm/plugin/evm/message"
	"github.com/luxfi/geth/crypto"
)

var _ message.SyncableParser = (*summaryParser)(nil)

type summaryParser struct{}

func NewSummaryParser() *summaryParser {
	return &summaryParser{}
}

func (a *summaryParser) Parse(summaryBytes []byte, acceptImpl message.AcceptImplFn) (message.Syncable, error) {
	summary := Summary{}
	if _, err := message.Codec.Unmarshal(summaryBytes, &summary); err != nil {
		return nil, fmt.Errorf("failed to parse syncable summary: %w", err)
	}

	summary.bytes = summaryBytes
	summaryID, err := ids.ToID(crypto.Keccak256(summaryBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to compute summary ID: %w", err)
	}
	summary.summaryID = summaryID
	summary.acceptImpl = acceptImpl
	return &summary, nil
}