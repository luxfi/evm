//SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "./FeeGovernor.sol";
import "./interfaces/IWarpMessenger.sol";

// Warp precompile address (LP-aligned)
address constant WARP_MESSENGER_ADDRESS = 0x0200000000000000000000000000000000000005;

/// @title FeeGovernorWarp - Cross-Chain Fee Governance Extension
/// @notice Extends FeeGovernor with Warp messaging for cross-chain governance
/// @dev Uses IWarpMessenger to receive governance commands from P-Chain or other subnets
///
/// Future capabilities:
/// 1. Receive P-Chain reward rate updates via Warp
/// 2. Synchronize fee configs across subnets
/// 3. Receive validator-weight-based voting from P-Chain
contract FeeGovernorWarp is FeeGovernor {
    // -------------------------------------------------------------------------
    // Warp Message Types
    // -------------------------------------------------------------------------

    /// @notice Message type identifiers for Warp payloads
    uint8 public constant MSG_TYPE_FEE_UPDATE = 1;
    uint8 public constant MSG_TYPE_REWARD_UPDATE = 2;
    uint8 public constant MSG_TYPE_EMERGENCY_PAUSE = 3;

    /// @notice Warp messenger interface
    IWarpMessenger public immutable warpMessenger;

    /// @notice Authorized source chain IDs for governance messages
    mapping(bytes32 => bool) public authorizedSourceChains;

    /// @notice Authorized sender addresses per source chain
    mapping(bytes32 => mapping(address => bool)) public authorizedSenders;

    // -------------------------------------------------------------------------
    // Events
    // -------------------------------------------------------------------------

    event WarpMessageReceived(
        bytes32 indexed sourceChainID,
        address indexed sender,
        uint8 messageType,
        bytes payload
    );

    event SourceChainAuthorized(bytes32 indexed chainID, bool authorized);
    event SenderAuthorized(bytes32 indexed chainID, address indexed sender, bool authorized);

    // -------------------------------------------------------------------------
    // Constructor
    // -------------------------------------------------------------------------

    constructor(
        address _votingToken,
        bool _isNFTVoting,
        address _guardian,
        uint256 votingDelayBlocks,
        uint256 votingPeriodBlocks,
        uint256 timelockDelaySeconds,
        uint256 proposalThresholdTokens,
        uint256 quorumBps
    ) FeeGovernor(
        _votingToken,
        _isNFTVoting,
        _guardian,
        votingDelayBlocks,
        votingPeriodBlocks,
        timelockDelaySeconds,
        proposalThresholdTokens,
        quorumBps
    ) {
        warpMessenger = IWarpMessenger(WARP_MESSENGER_ADDRESS);
    }

    // -------------------------------------------------------------------------
    // Warp Authorization Management
    // -------------------------------------------------------------------------

    /// @notice Authorize a source chain for governance messages
    /// @param chainID The blockchain ID to authorize
    /// @param authorized Whether to authorize or revoke
    function setSourceChainAuthorization(
        bytes32 chainID,
        bool authorized
    ) external onlyGuardian {
        authorizedSourceChains[chainID] = authorized;
        emit SourceChainAuthorized(chainID, authorized);
    }

    /// @notice Authorize a sender on a source chain
    /// @param chainID The source blockchain ID
    /// @param sender The sender address to authorize
    /// @param authorized Whether to authorize or revoke
    function setSenderAuthorization(
        bytes32 chainID,
        address sender,
        bool authorized
    ) external onlyGuardian {
        authorizedSenders[chainID][sender] = authorized;
        emit SenderAuthorized(chainID, sender, authorized);
    }

    // -------------------------------------------------------------------------
    // Warp Message Handling
    // -------------------------------------------------------------------------

    /// @notice Process a verified Warp message for governance
    /// @param index The index of the verified Warp message in predicate storage
    /// @dev Caller must ensure the message has been verified by including it in tx predicates
    function processWarpMessage(uint32 index) external whenNotPaused {
        (WarpMessage memory message, bool valid) = warpMessenger.getVerifiedWarpMessage(index);
        require(valid, "FeeGovernorWarp: invalid warp message");

        // Verify source chain is authorized
        require(
            authorizedSourceChains[message.sourceChainID],
            "FeeGovernorWarp: unauthorized source chain"
        );

        // Verify sender is authorized
        require(
            authorizedSenders[message.sourceChainID][message.originSenderAddress],
            "FeeGovernorWarp: unauthorized sender"
        );

        // Decode and process the message
        _processPayload(message.sourceChainID, message.originSenderAddress, message.payload);
    }

    /// @notice Internal payload processing
    function _processPayload(
        bytes32 sourceChainID,
        address sender,
        bytes memory payload
    ) internal {
        require(payload.length >= 1, "FeeGovernorWarp: empty payload");

        uint8 messageType = uint8(payload[0]);

        emit WarpMessageReceived(sourceChainID, sender, messageType, payload);

        if (messageType == MSG_TYPE_FEE_UPDATE) {
            _processFeeUpdate(payload);
        } else if (messageType == MSG_TYPE_REWARD_UPDATE) {
            _processRewardUpdate(payload);
        } else if (messageType == MSG_TYPE_EMERGENCY_PAUSE) {
            _processEmergencyPause();
        } else {
            revert("FeeGovernorWarp: unknown message type");
        }
    }

    /// @notice Process a fee configuration update from Warp
    /// @dev Payload format: [type(1)] [gasLimit(32)] [targetBlockRate(32)] ...
    function _processFeeUpdate(bytes memory payload) internal {
        require(payload.length == 1 + 8 * 32, "FeeGovernorWarp: invalid fee update payload");

        // Decode fee config (skip first byte which is message type)
        FeeConfig memory config;
        uint256 offset = 1;

        config.gasLimit = _readUint256(payload, offset);
        offset += 32;
        config.targetBlockRate = _readUint256(payload, offset);
        offset += 32;
        config.minBaseFee = _readUint256(payload, offset);
        offset += 32;
        config.targetGas = _readUint256(payload, offset);
        offset += 32;
        config.baseFeeChangeDenominator = _readUint256(payload, offset);
        offset += 32;
        config.minBlockGasCost = _readUint256(payload, offset);
        offset += 32;
        config.maxBlockGasCost = _readUint256(payload, offset);
        offset += 32;
        config.blockGasCostStep = _readUint256(payload, offset);

        // Apply the fee config directly (bypasses normal proposal flow for cross-chain sync)
        feeManager.setFeeConfig(
            config.gasLimit,
            config.targetBlockRate,
            config.minBaseFee,
            config.targetGas,
            config.baseFeeChangeDenominator,
            config.minBlockGasCost,
            config.maxBlockGasCost,
            config.blockGasCostStep
        );
    }

    /// @notice Process a reward address update from Warp
    /// @dev Payload format: [type(1)] [address(20)]
    function _processRewardUpdate(bytes memory payload) internal {
        require(payload.length == 1 + 20, "FeeGovernorWarp: invalid reward update payload");

        address newRewardAddress;
        assembly {
            newRewardAddress := shr(96, mload(add(payload, 33)))
        }

        rewardManager.setRewardAddress(newRewardAddress);
    }

    /// @notice Process an emergency pause command from Warp
    function _processEmergencyPause() internal {
        paused = true;
        emit Paused(address(this));
    }

    // -------------------------------------------------------------------------
    // Warp Message Sending (for cross-chain sync)
    // -------------------------------------------------------------------------

    /// @notice Send current fee config to other chains via Warp
    /// @return messageID The Warp message ID
    function broadcastFeeConfig() external whenNotPaused returns (bytes32) {
        (
            uint256 gasLimit,
            uint256 targetBlockRate,
            uint256 minBaseFee,
            uint256 targetGas,
            uint256 baseFeeChangeDenom,
            uint256 minBlockGasCost,
            uint256 maxBlockGasCost,
            uint256 blockGasCostStep
        ) = feeManager.getFeeConfig();

        bytes memory payload = abi.encodePacked(
            MSG_TYPE_FEE_UPDATE,
            gasLimit,
            targetBlockRate,
            minBaseFee,
            targetGas,
            baseFeeChangeDenom,
            minBlockGasCost,
            maxBlockGasCost,
            blockGasCostStep
        );

        return warpMessenger.sendWarpMessage(payload);
    }

    /// @notice Get this chain's blockchain ID
    /// @return The blockchain ID from Warp messenger
    function getBlockchainID() external view returns (bytes32) {
        return warpMessenger.getBlockchainID();
    }

    // -------------------------------------------------------------------------
    // Internal Helpers
    // -------------------------------------------------------------------------

    function _readUint256(bytes memory data, uint256 offset) internal pure returns (uint256 result) {
        require(data.length >= offset + 32, "FeeGovernorWarp: out of bounds");
        assembly {
            result := mload(add(add(data, 32), offset))
        }
    }
}
