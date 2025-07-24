// Copyright (C) 2019-2024, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package params

// Lux-specific gas costs for precompiled contracts
const (
	// AssetBalanceApricot is the gas cost for querying native asset balance
	AssetBalanceApricot uint64 = 2474

	// AssetCallApricot is the gas cost for calling native asset transfer
	AssetCallApricot uint64 = 9000
	
	// TxGas is the base gas cost for a transaction
	TxGas uint64 = 21000
	
	// EVM operation gas costs
	CallCreateDepth    uint64 = 1024  // Maximum depth of call/create stack
	CallNewAccountGas  uint64 = 25000 // Paid for CALL when the destination address didn't exist prior
	CreateDataGas      uint64 = 200   // Per byte of data attached to a create
	MaxCodeSize               = 24576 // Maximum bytecode to permit for a contract
)
