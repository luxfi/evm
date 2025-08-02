// (c) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package types

import (
	ethtypes "github.com/luxfi/geth/core/types"
)

// ConvertHeaderFromEVM converts an EVM Header to ethtypes.Header (geth types)
func ConvertHeaderFromEVM(h *Header) *ethtypes.Header {
	if h == nil {
		return nil
	}

	// Convert evm types to geth types
	var bloom ethtypes.Bloom
	copy(bloom[:], h.Bloom[:])

	var nonce ethtypes.BlockNonce
	copy(nonce[:], h.Nonce[:])

	result := &ethtypes.Header{
		ParentHash:  h.ParentHash,
		UncleHash:   h.UncleHash,
		Coinbase:    h.Coinbase,
		Root:        h.Root,
		TxHash:      h.TxHash,
		ReceiptHash: h.ReceiptHash,
		Bloom:       bloom,
		Difficulty:  h.Difficulty,
		Number:      h.Number,
		GasLimit:    h.GasLimit,
		GasUsed:     h.GasUsed,
		Time:        h.Time,
		Extra:       h.Extra,
		MixDigest:   h.MixDigest,
		Nonce:       nonce,
		BaseFee:     h.BaseFee,
	}

	// Note: ExtDataHash, ExtDataGasUsed, BlockGasCost, BlobGasUsed, ExcessBlobGas, 
	// and ParentBeaconRoot are EVM-specific fields not present in geth types
	// They will be lost in the conversion

	return result
}

// ConvertHeaderToEVM converts an ethtypes.Header (geth types) to EVM Header
func ConvertHeaderToEVM(h *ethtypes.Header) *Header {
	if h == nil {
		return nil
	}

	// Convert geth types to evm types
	var bloom Bloom
	copy(bloom[:], h.Bloom[:])

	var nonce BlockNonce
	copy(nonce[:], h.Nonce[:])

	result := &Header{
		ParentHash:  h.ParentHash,
		UncleHash:   h.UncleHash,
		Coinbase:    h.Coinbase,
		Root:        h.Root,
		TxHash:      h.TxHash,
		ReceiptHash: h.ReceiptHash,
		Bloom:       bloom,
		Difficulty:  h.Difficulty,
		Number:      h.Number,
		GasLimit:    h.GasLimit,
		GasUsed:     h.GasUsed,
		Time:        h.Time,
		Extra:       h.Extra,
		MixDigest:   h.MixDigest,
		Nonce:       nonce,
		BaseFee:     h.BaseFee,
	}

	// ExtDataHash, ExtDataGasUsed, BlockGasCost, BlobGasUsed, ExcessBlobGas,
	// and ParentBeaconRoot will be set to zero values

	return result
}

// ConvertBlockFromEVM converts an EVM Block to ethtypes.Block (geth types)
func ConvertBlockFromEVM(b *Block) *ethtypes.Block {
	if b == nil {
		return nil
	}

	header := ConvertHeaderFromEVM(b.Header())
	
	// Convert transactions
	var transactions []*ethtypes.Transaction
	for _, tx := range b.Transactions() {
		transactions = append(transactions, ConvertTransactionFromEVM(tx))
	}

	// Convert uncles
	var uncles []*ethtypes.Header
	for _, uncle := range b.Uncles() {
		uncles = append(uncles, ConvertHeaderFromEVM(uncle))
	}

	// Create new block with converted data
	body := ethtypes.Body{
		Transactions: transactions,
		Uncles:       uncles,
	}
	return ethtypes.NewBlockWithHeader(header).WithBody(body)
}

// ConvertBlockToEVM converts an ethtypes.Block (geth types) to EVM Block
func ConvertBlockToEVM(b *ethtypes.Block) *Block {
	if b == nil {
		return nil
	}

	header := ConvertHeaderToEVM(b.Header())
	
	// Convert transactions
	var transactions []*Transaction
	for _, tx := range b.Transactions() {
		transactions = append(transactions, ConvertTransactionToEVM(tx))
	}

	// Convert uncles
	var uncles []*Header
	for _, uncle := range b.Uncles() {
		uncles = append(uncles, ConvertHeaderToEVM(uncle))
	}

	// Create new block with converted data
	return NewBlockWithHeader(header).WithBody(transactions, uncles)
}

// ConvertTransactionFromEVM converts an EVM Transaction to ethtypes.Transaction (geth types)
func ConvertTransactionFromEVM(tx *Transaction) *ethtypes.Transaction {
	if tx == nil {
		return nil
	}
	// TODO: Implement proper conversion between EVM and geth transaction types
	// For now, return nil to avoid build errors
	return nil
}

// ConvertTransactionToEVM converts an ethtypes.Transaction (geth types) to EVM Transaction
func ConvertTransactionToEVM(tx *ethtypes.Transaction) *Transaction {
	if tx == nil {
		return nil
	}
	// TODO: Implement proper conversion between geth and EVM transaction types
	// For now, return nil to avoid build errors
	return nil
}

// ConvertReceiptFromEVM converts an EVM Receipt to ethtypes.Receipt (geth types)
func ConvertReceiptFromEVM(r *Receipt) *ethtypes.Receipt {
	if r == nil {
		return nil
	}
	// TODO: Implement proper conversion between EVM and geth receipt types
	// For now, return nil to avoid build errors
	return nil
}

// ConvertReceiptToEVM converts an ethtypes.Receipt (geth types) to EVM Receipt
func ConvertReceiptToEVM(r *ethtypes.Receipt) *Receipt {
	if r == nil {
		return nil
	}
	// TODO: Implement proper conversion between geth and EVM receipt types
	// For now, return nil to avoid build errors
	return nil
}