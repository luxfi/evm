// Copyright (C) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package warptokenbridge

import (
	"errors"
	"math/big"

	"github.com/luxfi/evm/precompile/contract"
	"github.com/luxfi/evm/vmerrs"
	"github.com/luxfi/ids"
	"github.com/luxfi/node/vms/platformvm/warp"
	"github.com/luxfi/geth/common"
	"github.com/holiman/uint256"
)

const (
	// Gas costs for operations
	VaultTokenGasCost    uint64 = 50000
	MintTokenGasCost     uint64 = 100000
	BurnTokenGasCost     uint64 = 80000
	RegisterTokenGasCost uint64 = 20000
)

var (
	_ contract.StatefulPrecompiledContract = &WarpTokenBridgeV2{}

	ErrInvalidToken       = errors.New("invalid token address")
	ErrTokenNotRegistered = errors.New("token not registered for bridging")
	ErrInsufficientAmount = errors.New("insufficient token amount")
	ErrInvalidDestination = errors.New("invalid destination chain")
	ErrNotTokenOwner      = errors.New("not token owner")

	// Singleton stateful precompiled contract for token bridging via Warp
	WarpTokenBridgeV2Precompile = createWarpTokenBridgeV2Precompile()

	WarpTokenBridgeV2Address = common.HexToAddress("0x0200000000000000000000000000000000000002")
)

// TokenType represents the type of token being bridged
type TokenType uint8

const (
	TokenTypeERC20 TokenType = iota
	TokenTypeERC721
	TokenTypeERC1155
	TokenTypeNative
	TokenTypeXChainNative // Native X-Chain NFT format
)

// TokenVault represents vaulted tokens on X-Chain
type TokenVault struct {
	OriginalChain   ids.ID         // Chain where token originated
	OriginalToken   common.Address // Original token contract address
	TokenType       TokenType
	VaultedAmount   *big.Int       // For fungible tokens
	VaultedTokenIds []*uint256.Int // For NFTs
	Owner           common.Address
	Metadata        []byte         // Token metadata for cross-chain recreation
}

// BridgeMessage represents a cross-chain token transfer message
type BridgeMessage struct {
	MessageType        uint8 // 0: Vault, 1: Mint, 2: Burn, 3: Release
	SourceChainID      ids.ID
	DestinationChainID ids.ID
	TokenAddress       common.Address
	TokenType          TokenType
	Sender             common.Address
	Recipient          common.Address
	
	// For fungible tokens (ERC20, ERC1155 fungible)
	Amount *big.Int
	
	// For non-fungible tokens (ERC721, ERC1155 NFT)
	TokenID *big.Int
	
	// For ERC1155 batch operations
	TokenIDs []*uint256.Int
	Amounts  []*uint256.Int
	
	// Vault reference for minting wrapped tokens
	VaultID [32]byte
	
	// Additional data (metadata, etc.)
	Data []byte
	
	Nonce uint64
}

// WarpTokenBridgeV2 enables comprehensive cross-chain token transfers
type WarpTokenBridgeV2 struct {
	// No allow list state needed for this implementation
}

// Run implements the StatefulPrecompiledContract interface
func (w *WarpTokenBridgeV2) Run(accessibleState contract.AccessibleState, caller common.Address, addr common.Address, input []byte, suppliedGas uint64, readOnly bool) (ret []byte, remainingGas uint64, err error) {
	// Decode the function selector from input
	if len(input) < 4 {
		return nil, 0, errors.New("invalid input length")
	}
	
	selector := input[:4]
	data := input[4:]
	
	// Route to appropriate function based on selector
	switch string(selector) {
	case "vault":
		return vaultToXChain(accessibleState, caller, addr, data, suppliedGas, readOnly)
	case "mint":
		return mintFromVault(accessibleState, caller, addr, data, suppliedGas, readOnly)
	case "burn":
		return burnAndRelease(accessibleState, caller, addr, data, suppliedGas, readOnly)
	case "register":
		return registerToken(accessibleState, caller, addr, data, suppliedGas, readOnly)
	default:
		return nil, 0, errors.New("unknown function selector")
	}
}

// createWarpTokenBridgeV2Precompile creates the singleton instance
func createWarpTokenBridgeV2Precompile() contract.StatefulPrecompiledContract {
	return &WarpTokenBridgeV2{}
}

// vaultToXChain moves tokens from any chain to X-Chain vault
func vaultToXChain(
	accessibleState contract.AccessibleState,
	caller common.Address,
	addr common.Address,
	input []byte,
	suppliedGas uint64,
	readOnly bool,
) (ret []byte, remainingGas uint64, err error) {
	if readOnly {
		return nil, suppliedGas, vmerrs.ErrWriteProtection
	}
	if suppliedGas < VaultTokenGasCost {
		return nil, 0, vmerrs.ErrOutOfGas
	}

	// Parse input based on token type
	tokenType, tokenAddress, tokenData, err := unpackVaultInput(input)
	if err != nil {
		return nil, suppliedGas - VaultTokenGasCost, err
	}

	var message BridgeMessage
	message.MessageType = 0 // Vault operation
	message.SourceChainID = getCurrentChainID()
	message.DestinationChainID = getXChainID()
	message.TokenAddress = tokenAddress
	message.TokenType = tokenType
	message.Sender = caller

	switch tokenType {
	case TokenTypeERC20:
		amount, recipient := unpackERC20Data(tokenData)
		message.Amount = amount
		message.Recipient = recipient
		// Lock ERC20 tokens in bridge contract
		err = lockERC20(accessibleState, caller, tokenAddress, amount)
		
	case TokenTypeERC721:
		tokenID, recipient := unpackERC721Data(tokenData)
		message.TokenID = tokenID
		message.Recipient = recipient
		// Transfer NFT to bridge contract
		err = lockERC721(accessibleState, caller, tokenAddress, tokenID)
		
	case TokenTypeERC1155:
		tokenIDs, amounts, recipient, data := unpackERC1155Data(tokenData)
		message.TokenIDs = tokenIDs
		message.Amounts = amounts
		message.Recipient = recipient
		message.Data = data
		// Lock ERC1155 tokens in bridge contract
		err = lockERC1155(accessibleState, caller, tokenAddress, tokenIDs, amounts)
		
	default:
		return nil, suppliedGas - VaultTokenGasCost, errors.New("unsupported token type")
	}

	if err != nil {
		return nil, suppliedGas - VaultTokenGasCost, err
	}

	// Generate vault ID for future reference
	vaultID := generateVaultID(message)
	message.VaultID = vaultID

	// Send Warp message to X-Chain
	warpMessage, err := createWarpMessage(message)
	if err != nil {
		return nil, suppliedGas - VaultTokenGasCost, err
	}

	if err := sendWarpMessage(accessibleState, warpMessage); err != nil {
		return nil, suppliedGas - VaultTokenGasCost, err
	}

	// Store vault reference locally
	storeVaultReference(accessibleState.GetStateDB(), vaultID, message)

	return packVaultResult(vaultID), suppliedGas - VaultTokenGasCost, nil
}

// mintFromVault mints wrapped tokens on destination chain from X-Chain vault
func mintFromVault(
	accessibleState contract.AccessibleState,
	caller common.Address,
	addr common.Address,
	input []byte,
	suppliedGas uint64,
	readOnly bool,
) (ret []byte, remainingGas uint64, err error) {
	if readOnly {
		return nil, suppliedGas, vmerrs.ErrWriteProtection
	}
	if suppliedGas < MintTokenGasCost {
		return nil, 0, vmerrs.ErrOutOfGas
	}

	// Parse vault ID and destination
	vaultID, recipient, amount, tokenIDs, err := unpackMintInput(input)
	if err != nil {
		return nil, suppliedGas - MintTokenGasCost, err
	}

	// Create mint request message (not used in this stub)
	_ = BridgeMessage{
		MessageType:        1, // Mint operation
		SourceChainID:      getXChainID(),
		DestinationChainID: getCurrentChainID(),
		VaultID:            vaultID,
		Recipient:          recipient,
		Amount:             amount,
		TokenIDs:           tokenIDs,
	}

	// Get verified Warp message from X-Chain
	warpMessage, err := getVerifiedWarpMessage(accessibleState, input)
	if err != nil {
		return nil, suppliedGas - MintTokenGasCost, err
	}

	// Verify message is from X-Chain and contains valid vault proof
	vaultInfo, err := decodeVaultProof(warpMessage.Payload)
	if err != nil {
		return nil, suppliedGas - MintTokenGasCost, err
	}

	// Mint wrapped tokens based on vault info
	var wrappedTokenAddr common.Address
	switch vaultInfo.TokenType {
	case TokenTypeERC20:
		wrappedTokenAddr, err = mintWrappedERC20(accessibleState, vaultInfo, recipient, amount)
	case TokenTypeERC721:
		wrappedTokenAddr, err = mintWrappedERC721(accessibleState, vaultInfo, recipient, tokenIDs[0])
	case TokenTypeERC1155:
		// For ERC1155, we need to get amounts from the message
		var amounts []*uint256.Int
		if len(tokenIDs) > 0 {
			amounts = make([]*uint256.Int, len(tokenIDs))
			for i := range amounts {
				amounts[i] = uint256.NewInt(1) // Default to 1 for each token
			}
		}
		wrappedTokenAddr, err = mintWrappedERC1155(accessibleState, vaultInfo, recipient, tokenIDs, amounts)
	}

	if err != nil {
		return nil, suppliedGas - MintTokenGasCost, err
	}

	return packMintResult(wrappedTokenAddr), suppliedGas - MintTokenGasCost, nil
}

// burnAndRelease burns wrapped tokens and releases from X-Chain vault
func burnAndRelease(
	accessibleState contract.AccessibleState,
	caller common.Address,
	addr common.Address,
	input []byte,
	suppliedGas uint64,
	readOnly bool,
) (ret []byte, remainingGas uint64, err error) {
	if readOnly {
		return nil, suppliedGas, vmerrs.ErrWriteProtection
	}
	if suppliedGas < BurnTokenGasCost {
		return nil, 0, vmerrs.ErrOutOfGas
	}

	// Parse wrapped token and amounts to burn
	wrappedToken, burnData, _, recipient, err := unpackBurnInput(input)
	if err != nil {
		return nil, suppliedGas - BurnTokenGasCost, err
	}

	// Verify wrapped token is valid and get original vault info
	vaultID, _, err := getWrappedTokenVaultInfo(accessibleState.GetStateDB(), wrappedToken)
	if err != nil {
		return nil, suppliedGas - BurnTokenGasCost, err
	}

	// Burn wrapped tokens
	err = burnWrappedTokens(accessibleState, caller, wrappedToken, burnData)
	if err != nil {
		return nil, suppliedGas - BurnTokenGasCost, err
	}

	// Create release message for X-Chain
	releaseMsg := BridgeMessage{
		MessageType:        2, // Burn operation
		SourceChainID:      getCurrentChainID(),
		DestinationChainID: getXChainID(),
		VaultID:            vaultID,
		Sender:             caller,
		Recipient:          recipient,
		// Include what was burned
		Amount:   burnData.Amount,
		TokenIDs: burnData.TokenIDs,
	}

	// Send to X-Chain to trigger release
	warpMessage, err := createWarpMessage(releaseMsg)
	if err != nil {
		return nil, suppliedGas - BurnTokenGasCost, err
	}

	if err := sendWarpMessage(accessibleState, warpMessage); err != nil {
		return nil, suppliedGas - BurnTokenGasCost, err
	}

	return packBurnResult(true), suppliedGas - BurnTokenGasCost, nil
}

// processXChainVault handles incoming vault requests on X-Chain
func processXChainVault(
	accessibleState contract.AccessibleState,
	message BridgeMessage,
) error {
	// Create vault entry on X-Chain
	vault := TokenVault{
		OriginalChain: message.SourceChainID,
		OriginalToken: message.TokenAddress,
		TokenType:     message.TokenType,
		Owner:         message.Recipient,
		Metadata:      message.Data,
	}

	switch message.TokenType {
	case TokenTypeERC20:
		vault.VaultedAmount = message.Amount
	case TokenTypeERC721:
		vault.VaultedTokenIds = []*uint256.Int{uint256.NewInt(0).SetBytes(message.TokenID.Bytes())}
	case TokenTypeERC1155:
		vault.VaultedTokenIds = message.TokenIDs
		vault.VaultedAmount = sumAmounts(message.Amounts)
	}

	// Store vault on X-Chain
	storeXChainVault(accessibleState.GetStateDB(), message.VaultID, vault)
	
	// If converting to X-Chain native format, do conversion
	if shouldConvertToNative(message.TokenType) {
		convertToXChainNative(accessibleState, vault)
	}

	return nil
}

// Helper functions for wrapped token management
func getOrDeployWrappedToken(
	accessibleState contract.AccessibleState,
	originalChain ids.ID,
	originalToken common.Address,
	tokenType TokenType,
	metadata []byte,
) (common.Address, error) {
	// Check if wrapped token already exists
	wrappedAddr := getWrappedTokenAddress(accessibleState.GetStateDB(), originalChain, originalToken)
	if wrappedAddr != (common.Address{}) {
		return wrappedAddr, nil
	}

	// Deploy new wrapped token contract
	switch tokenType {
	case TokenTypeERC20:
		return deployWrappedERC20(accessibleState, originalChain, originalToken, metadata)
	case TokenTypeERC721:
		return deployWrappedERC721(accessibleState, originalChain, originalToken, metadata)
	case TokenTypeERC1155:
		return deployWrappedERC1155(accessibleState, originalChain, originalToken, metadata)
	default:
		return common.Address{}, errors.New("unsupported token type for wrapping")
	}
}

// Integration with main bridge contract at ~/work/lux/bridge
func integrateWithMainBridge(bridgeAddr common.Address) {
	// Register this precompile with the main bridge orchestrator
	// This allows the bridge to route token operations through this precompile
}

// Stub implementations for missing functions - TO BE IMPLEMENTED

func registerToken(accessibleState contract.AccessibleState, caller common.Address, addr common.Address, input []byte, suppliedGas uint64, readOnly bool) (ret []byte, remainingGas uint64, err error) {
	if readOnly {
		return nil, suppliedGas, vmerrs.ErrWriteProtection
	}
	if suppliedGas < RegisterTokenGasCost {
		return nil, 0, vmerrs.ErrOutOfGas
	}
	// TODO: Implement token registration
	return nil, suppliedGas - RegisterTokenGasCost, errors.New("not implemented")
}

func unpackVaultInput(input []byte) (TokenType, common.Address, []byte, error) {
	// TODO: Implement input unpacking
	return TokenTypeERC20, common.Address{}, nil, nil
}

func getCurrentChainID() ids.ID {
	// TODO: Get current chain ID from context
	return ids.Empty
}

func getXChainID() ids.ID {
	// TODO: Get X-Chain ID
	return ids.Empty
}

func unpackERC20Data(data []byte) (*big.Int, common.Address) {
	// TODO: Unpack ERC20 transfer data
	return big.NewInt(0), common.Address{}
}

func unpackERC721Data(data []byte) (*big.Int, common.Address) {
	// TODO: Unpack ERC721 transfer data  
	return big.NewInt(0), common.Address{}
}

func unpackERC1155Data(data []byte) ([]*uint256.Int, []*uint256.Int, common.Address, []byte) {
	// TODO: Unpack ERC1155 transfer data
	return nil, nil, common.Address{}, nil
}

func lockERC20(state contract.AccessibleState, from common.Address, token common.Address, amount *big.Int) error {
	// TODO: Lock ERC20 tokens
	return nil
}

func lockERC721(state contract.AccessibleState, from common.Address, token common.Address, tokenId *big.Int) error {
	// TODO: Lock ERC721 token
	return nil
}

func lockERC1155(state contract.AccessibleState, from common.Address, token common.Address, tokenIds []*uint256.Int, amounts []*uint256.Int) error {
	// TODO: Lock ERC1155 tokens
	return nil
}

func generateVaultID(msg BridgeMessage) [32]byte {
	// TODO: Generate unique vault ID
	return [32]byte{}
}

func createWarpMessage(msg BridgeMessage) (*warp.Message, error) {
	// TODO: Create warp message
	return nil, nil
}

func sendWarpMessage(state contract.AccessibleState, msg *warp.Message) error {
	// TODO: Send warp message
	return nil
}

func storeVaultReference(db contract.StateDB, vaultID [32]byte, msg BridgeMessage) {
	// TODO: Store vault reference
}

func packVaultResult(vaultID [32]byte) []byte {
	// TODO: Pack vault result
	return nil
}

func unpackMintInput(input []byte) ([32]byte, common.Address, *big.Int, []*uint256.Int, error) {
	// TODO: Unpack mint input
	return [32]byte{}, common.Address{}, nil, nil, nil
}

func getVerifiedWarpMessage(state contract.AccessibleState, input []byte) (*warp.Message, error) {
	// TODO: Get verified warp message
	return nil, nil
}

func decodeVaultProof(payload []byte) (*TokenVault, error) {
	// TODO: Decode vault proof
	return nil, nil
}

func mintWrappedERC20(state contract.AccessibleState, vault *TokenVault, to common.Address, amount *big.Int) (common.Address, error) {
	// TODO: Mint wrapped ERC20
	return common.Address{}, nil
}

func mintWrappedERC721(state contract.AccessibleState, vault *TokenVault, to common.Address, tokenId *uint256.Int) (common.Address, error) {
	// TODO: Mint wrapped ERC721
	return common.Address{}, nil
}

func mintWrappedERC1155(state contract.AccessibleState, vault *TokenVault, to common.Address, tokenIds []*uint256.Int, amounts []*uint256.Int) (common.Address, error) {
	// TODO: Mint wrapped ERC1155
	return common.Address{}, nil
}

func packMintResult(addr common.Address) []byte {
	// TODO: Pack mint result
	return nil
}

type BurnData struct {
	Amount   *big.Int
	TokenIDs []*uint256.Int
}

func unpackBurnInput(input []byte) (common.Address, *BurnData, ids.ID, common.Address, error) {
	// TODO: Unpack burn input
	return common.Address{}, nil, ids.Empty, common.Address{}, nil
}

func getWrappedTokenVaultInfo(db contract.StateDB, token common.Address) ([32]byte, *TokenVault, error) {
	// TODO: Get wrapped token vault info
	return [32]byte{}, nil, nil
}

func burnWrappedTokens(state contract.AccessibleState, from common.Address, token common.Address, data *BurnData) error {
	// TODO: Burn wrapped tokens
	return nil
}

func packBurnResult(success bool) []byte {
	// TODO: Pack burn result
	return nil
}

func sumAmounts(amounts []*uint256.Int) *big.Int {
	// TODO: Sum amounts
	return big.NewInt(0)
}

func storeXChainVault(db contract.StateDB, vaultID [32]byte, vault TokenVault) {
	// TODO: Store X-Chain vault
}

func shouldConvertToNative(tokenType TokenType) bool {
	// TODO: Check if should convert to native
	return false
}

func convertToXChainNative(state contract.AccessibleState, vault TokenVault) {
	// TODO: Convert to X-Chain native format
}

func getWrappedTokenAddress(db contract.StateDB, chain ids.ID, token common.Address) common.Address {
	// TODO: Get wrapped token address
	return common.Address{}
}

func deployWrappedERC20(state contract.AccessibleState, chain ids.ID, token common.Address, metadata []byte) (common.Address, error) {
	// TODO: Deploy wrapped ERC20
	return common.Address{}, nil
}

func deployWrappedERC721(state contract.AccessibleState, chain ids.ID, token common.Address, metadata []byte) (common.Address, error) {
	// TODO: Deploy wrapped ERC721
	return common.Address{}, nil
}

func deployWrappedERC1155(state contract.AccessibleState, chain ids.ID, token common.Address, metadata []byte) (common.Address, error) {
	// TODO: Deploy wrapped ERC1155
	return common.Address{}, nil
}