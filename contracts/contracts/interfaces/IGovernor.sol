//SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

/// @title IGovernor - Lux Fee/Reward Governance Interface
/// @notice Interface for DAO governance of FeeManager and RewardManager precompiles
interface IGovernor {
    /// @notice Proposal state enumeration
    enum ProposalState {
        Pending,    // Proposal created, not yet active
        Active,     // Voting period active
        Defeated,   // Quorum not met or majority against
        Succeeded,  // Voting passed, awaiting execution
        Queued,     // In timelock queue
        Executed,   // Proposal executed
        Canceled,   // Proposal canceled
        Expired     // Timelock expired
    }

    /// @notice Fee configuration parameters (mirrors IFeeManager.FeeConfig)
    struct FeeConfig {
        uint256 gasLimit;
        uint256 targetBlockRate;
        uint256 minBaseFee;
        uint256 targetGas;
        uint256 baseFeeChangeDenominator;
        uint256 minBlockGasCost;
        uint256 maxBlockGasCost;
        uint256 blockGasCostStep;
    }

    /// @notice Proposal data structure
    struct Proposal {
        uint256 id;
        address proposer;
        FeeConfig feeConfig;
        address treasuryRecipient;
        uint256 forVotes;
        uint256 againstVotes;
        uint256 abstainVotes;
        uint256 startBlock;
        uint256 endBlock;
        uint256 executionTime;
        bool executed;
        bool canceled;
    }

    // Events
    event ProposalCreated(
        uint256 indexed proposalId,
        address indexed proposer,
        FeeConfig feeConfig,
        address treasuryRecipient,
        uint256 startBlock,
        uint256 endBlock,
        string description
    );

    event VoteCast(
        address indexed voter,
        uint256 indexed proposalId,
        uint8 support,
        uint256 weight,
        string reason
    );

    event ProposalQueued(uint256 indexed proposalId, uint256 executionTime);
    event ProposalExecuted(uint256 indexed proposalId);
    event ProposalCanceled(uint256 indexed proposalId);
    event Paused(address account);
    event Unpaused(address account);

    // Core governance functions
    function propose(
        FeeConfig calldata feeConfig,
        address treasuryRecipient,
        string calldata description
    ) external returns (uint256 proposalId);

    function castVote(uint256 proposalId, uint8 support) external returns (uint256 weight);
    function castVoteWithReason(uint256 proposalId, uint8 support, string calldata reason) external returns (uint256 weight);
    function queue(uint256 proposalId) external;
    function execute(uint256 proposalId) external;
    function cancel(uint256 proposalId) external;

    // View functions
    function state(uint256 proposalId) external view returns (ProposalState);
    function getProposal(uint256 proposalId) external view returns (Proposal memory);
    function hasVoted(uint256 proposalId, address account) external view returns (bool);
    function getVotes(address account) external view returns (uint256);
    function quorum() external view returns (uint256);
    function proposalThreshold() external view returns (uint256);
    function votingDelay() external view returns (uint256);
    function votingPeriod() external view returns (uint256);
    function timelockDelay() external view returns (uint256);
}
