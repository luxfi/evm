// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package atomic

import (
	"fmt"

	"github.com/luxfi/ids"
	"github.com/luxfi/evm/v2/v2/plugin/evm/message"
	"github.com/luxfi/geth/crypto"
)

type summaryParser struct{}

func NewSummaryParser() *summaryParser {
	return &summaryParser{}
}

func (a *summaryParser) Parse(summaryBytes []byte, acceptImpl func(*Summary) error) (*Summary, error) {
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