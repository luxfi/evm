//SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "./interfaces/IGovernor.sol";
import "./interfaces/IFeeManager.sol";
import "./interfaces/IRewardManager.sol";
import "./interfaces/IAllowList.sol";

// LP-aligned address scheme: P=3 (EVM/Crypto), C=2 (C-Chain)
address constant GOV_FEE_MANAGER_ADDRESS = 0x000000000000000000000000000000000001320f;
address constant GOV_REWARD_MANAGER_ADDRESS = 0x0000000000000000000000000000000000010205;

/// @title FeeGovernor - Lux Network Fee/Reward DAO Governance
/// @notice DAO Governor for proposing and executing fee parameter changes
/// @dev Integrates with FeeManager and RewardManager precompiles via AllowList
///
/// Design constraints:
/// 1. Max 10% change per parameter per proposal
/// 2. 48-72 hour timelock on execution
/// 3. Token-weighted or NFT-based voting
/// 4. Emergency pause capability
/// 5. Bounds checking on all parameters
contract FeeGovernor is IGovernor {
    // -------------------------------------------------------------------------
    // Constants and Configuration
    // -------------------------------------------------------------------------

    /// @notice Maximum parameter change percentage (10% = 1000 basis points)
    uint256 public constant MAX_CHANGE_BPS = 1000;
    uint256 public constant BPS_DENOMINATOR = 10000;

    /// @notice Voting support values
    uint8 public constant VOTE_AGAINST = 0;
    uint8 public constant VOTE_FOR = 1;
    uint8 public constant VOTE_ABSTAIN = 2;

    /// @notice Timelock bounds (in seconds)
    uint256 public constant MIN_TIMELOCK_DELAY = 48 hours;
    uint256 public constant MAX_TIMELOCK_DELAY = 72 hours;
    uint256 public constant TIMELOCK_GRACE_PERIOD = 14 days;

    /// @notice Parameter bounds (safety limits)
    uint256 public constant MIN_GAS_LIMIT = 1_000_000;
    uint256 public constant MAX_GAS_LIMIT = 100_000_000;
    uint256 public constant MIN_TARGET_BLOCK_RATE = 1;
    uint256 public constant MAX_TARGET_BLOCK_RATE = 60;
    uint256 public constant MIN_BASE_FEE = 1_000_000_000; // 1 gwei
    uint256 public constant MAX_BASE_FEE = 1_000_000_000_000; // 1000 gwei
    uint256 public constant MIN_BASE_FEE_CHANGE_DENOM = 8;
    uint256 public constant MAX_BASE_FEE_CHANGE_DENOM = 1000;

    // -------------------------------------------------------------------------
    // State Variables
    // -------------------------------------------------------------------------

    /// @notice Precompile interfaces
    IFeeManager public immutable feeManager;
    IRewardManager public immutable rewardManager;

    /// @notice Governance token for voting weight
    address public immutable votingToken;

    /// @notice Whether voting is NFT-based (ERC721) or token-based (ERC20)
    bool public immutable isNFTVoting;

    /// @notice Governance parameters
    uint256 private _votingDelay;    // Blocks before voting starts
    uint256 private _votingPeriod;   // Blocks for voting
    uint256 private _timelockDelay;  // Seconds before execution
    uint256 private _proposalThreshold; // Tokens required to propose
    uint256 private _quorumNumerator;   // Quorum percentage (basis points)

    /// @notice Emergency pause state
    bool public paused;

    /// @notice Guardian address for emergency actions
    address public guardian;

    /// @notice Proposal storage
    uint256 public proposalCount;
    mapping(uint256 => Proposal) private _proposals;
    mapping(uint256 => mapping(address => bool)) private _hasVoted;
    mapping(uint256 => mapping(address => uint256)) private _voteWeight;

    // -------------------------------------------------------------------------
    // Modifiers
    // -------------------------------------------------------------------------

    modifier whenNotPaused() {
        require(!paused, "FeeGovernor: paused");
        _;
    }

    modifier onlyGuardian() {
        require(msg.sender == guardian, "FeeGovernor: not guardian");
        _;
    }

    // -------------------------------------------------------------------------
    // Constructor
    // -------------------------------------------------------------------------

    /// @notice Initialize the Governor contract
    /// @param _votingToken Address of ERC20 or ERC721 token for voting
    /// @param _isNFTVoting True if using NFT (ERC721) voting
    /// @param _guardian Emergency guardian address
    /// @param votingDelayBlocks Blocks before voting starts
    /// @param votingPeriodBlocks Blocks for voting period
    /// @param timelockDelaySeconds Seconds for timelock (48-72 hours)
    /// @param proposalThresholdTokens Tokens required to create proposal
    /// @param quorumBps Quorum in basis points (e.g., 400 = 4%)
    constructor(
        address _votingToken,
        bool _isNFTVoting,
        address _guardian,
        uint256 votingDelayBlocks,
        uint256 votingPeriodBlocks,
        uint256 timelockDelaySeconds,
        uint256 proposalThresholdTokens,
        uint256 quorumBps
    ) {
        require(_votingToken != address(0), "FeeGovernor: zero voting token");
        require(_guardian != address(0), "FeeGovernor: zero guardian");
        require(
            timelockDelaySeconds >= MIN_TIMELOCK_DELAY &&
            timelockDelaySeconds <= MAX_TIMELOCK_DELAY,
            "FeeGovernor: invalid timelock delay"
        );
        require(quorumBps <= BPS_DENOMINATOR, "FeeGovernor: quorum exceeds 100%");

        feeManager = IFeeManager(GOV_FEE_MANAGER_ADDRESS);
        rewardManager = IRewardManager(GOV_REWARD_MANAGER_ADDRESS);
        votingToken = _votingToken;
        isNFTVoting = _isNFTVoting;
        guardian = _guardian;

        _votingDelay = votingDelayBlocks;
        _votingPeriod = votingPeriodBlocks;
        _timelockDelay = timelockDelaySeconds;
        _proposalThreshold = proposalThresholdTokens;
        _quorumNumerator = quorumBps;
    }

    // -------------------------------------------------------------------------
    // Proposal Creation
    // -------------------------------------------------------------------------

    /// @notice Create a new fee configuration proposal
    /// @param feeConfig Proposed fee configuration
    /// @param treasuryRecipient Proposed treasury recipient (address(0) to skip)
    /// @param description Human-readable proposal description
    /// @return proposalId Unique proposal identifier
    function propose(
        FeeConfig calldata feeConfig,
        address treasuryRecipient,
        string calldata description
    ) external override whenNotPaused returns (uint256 proposalId) {
        require(
            getVotes(msg.sender) >= _proposalThreshold,
            "FeeGovernor: below proposal threshold"
        );

        // Validate parameter bounds
        _validateBounds(feeConfig);

        // Validate max 10% change per parameter
        _validateMaxChange(feeConfig);

        proposalId = ++proposalCount;
        uint256 startBlock = block.number + _votingDelay;
        uint256 endBlock = startBlock + _votingPeriod;

        _proposals[proposalId] = Proposal({
            id: proposalId,
            proposer: msg.sender,
            feeConfig: feeConfig,
            treasuryRecipient: treasuryRecipient,
            forVotes: 0,
            againstVotes: 0,
            abstainVotes: 0,
            startBlock: startBlock,
            endBlock: endBlock,
            executionTime: 0,
            executed: false,
            canceled: false
        });

        emit ProposalCreated(
            proposalId,
            msg.sender,
            feeConfig,
            treasuryRecipient,
            startBlock,
            endBlock,
            description
        );
    }

    // -------------------------------------------------------------------------
    // Voting
    // -------------------------------------------------------------------------

    /// @notice Cast a vote on a proposal
    /// @param proposalId Proposal to vote on
    /// @param support 0=against, 1=for, 2=abstain
    /// @return weight Voting weight applied
    function castVote(
        uint256 proposalId,
        uint8 support
    ) external override whenNotPaused returns (uint256 weight) {
        return _castVote(proposalId, msg.sender, support, "");
    }

    /// @notice Cast a vote with reason
    /// @param proposalId Proposal to vote on
    /// @param support 0=against, 1=for, 2=abstain
    /// @param reason Voting rationale
    /// @return weight Voting weight applied
    function castVoteWithReason(
        uint256 proposalId,
        uint8 support,
        string calldata reason
    ) external override whenNotPaused returns (uint256 weight) {
        return _castVote(proposalId, msg.sender, support, reason);
    }

    function _castVote(
        uint256 proposalId,
        address voter,
        uint8 support,
        string memory reason
    ) internal returns (uint256 weight) {
        require(state(proposalId) == ProposalState.Active, "FeeGovernor: not active");
        require(!_hasVoted[proposalId][voter], "FeeGovernor: already voted");
        require(support <= VOTE_ABSTAIN, "FeeGovernor: invalid vote type");

        weight = getVotes(voter);
        require(weight > 0, "FeeGovernor: no voting power");

        _hasVoted[proposalId][voter] = true;
        _voteWeight[proposalId][voter] = weight;

        Proposal storage proposal = _proposals[proposalId];
        if (support == VOTE_FOR) {
            proposal.forVotes += weight;
        } else if (support == VOTE_AGAINST) {
            proposal.againstVotes += weight;
        } else {
            proposal.abstainVotes += weight;
        }

        emit VoteCast(voter, proposalId, support, weight, reason);
    }

    // -------------------------------------------------------------------------
    // Queue and Execute
    // -------------------------------------------------------------------------

    /// @notice Queue a succeeded proposal for execution
    /// @param proposalId Proposal to queue
    function queue(uint256 proposalId) external override whenNotPaused {
        require(state(proposalId) == ProposalState.Succeeded, "FeeGovernor: not succeeded");

        Proposal storage proposal = _proposals[proposalId];
        proposal.executionTime = block.timestamp + _timelockDelay;

        emit ProposalQueued(proposalId, proposal.executionTime);
    }

    /// @notice Execute a queued proposal
    /// @param proposalId Proposal to execute
    function execute(uint256 proposalId) external override whenNotPaused {
        require(state(proposalId) == ProposalState.Queued, "FeeGovernor: not queued");

        Proposal storage proposal = _proposals[proposalId];
        require(
            block.timestamp >= proposal.executionTime,
            "FeeGovernor: timelock not elapsed"
        );
        require(
            block.timestamp <= proposal.executionTime + TIMELOCK_GRACE_PERIOD,
            "FeeGovernor: proposal expired"
        );

        proposal.executed = true;

        // Execute fee configuration change
        feeManager.setFeeConfig(
            proposal.feeConfig.gasLimit,
            proposal.feeConfig.targetBlockRate,
            proposal.feeConfig.minBaseFee,
            proposal.feeConfig.targetGas,
            proposal.feeConfig.baseFeeChangeDenominator,
            proposal.feeConfig.minBlockGasCost,
            proposal.feeConfig.maxBlockGasCost,
            proposal.feeConfig.blockGasCostStep
        );

        // Execute treasury recipient change if specified
        if (proposal.treasuryRecipient != address(0)) {
            rewardManager.setRewardAddress(proposal.treasuryRecipient);
        }

        emit ProposalExecuted(proposalId);
    }

    /// @notice Cancel a proposal
    /// @param proposalId Proposal to cancel
    function cancel(uint256 proposalId) external override {
        Proposal storage proposal = _proposals[proposalId];
        require(!proposal.executed, "FeeGovernor: already executed");
        require(!proposal.canceled, "FeeGovernor: already canceled");

        // Only proposer or guardian can cancel
        require(
            msg.sender == proposal.proposer || msg.sender == guardian,
            "FeeGovernor: not authorized"
        );

        proposal.canceled = true;
        emit ProposalCanceled(proposalId);
    }

    // -------------------------------------------------------------------------
    // Emergency Functions
    // -------------------------------------------------------------------------

    /// @notice Pause all governance actions
    function pause() external onlyGuardian {
        paused = true;
        emit Paused(msg.sender);
    }

    /// @notice Unpause governance actions
    function unpause() external onlyGuardian {
        paused = false;
        emit Unpaused(msg.sender);
    }

    /// @notice Transfer guardian role
    /// @param newGuardian New guardian address
    function transferGuardian(address newGuardian) external onlyGuardian {
        require(newGuardian != address(0), "FeeGovernor: zero address");
        guardian = newGuardian;
    }

    // -------------------------------------------------------------------------
    // View Functions
    // -------------------------------------------------------------------------

    /// @notice Get proposal state
    /// @param proposalId Proposal to query
    /// @return Current proposal state
    function state(uint256 proposalId) public view override returns (ProposalState) {
        Proposal storage proposal = _proposals[proposalId];
        require(proposal.id != 0, "FeeGovernor: unknown proposal");

        if (proposal.canceled) {
            return ProposalState.Canceled;
        }

        if (proposal.executed) {
            return ProposalState.Executed;
        }

        if (block.number < proposal.startBlock) {
            return ProposalState.Pending;
        }

        if (block.number <= proposal.endBlock) {
            return ProposalState.Active;
        }

        // Check if succeeded
        if (_quorumReached(proposalId) && _voteSucceeded(proposalId)) {
            if (proposal.executionTime == 0) {
                return ProposalState.Succeeded;
            }
            if (block.timestamp < proposal.executionTime + TIMELOCK_GRACE_PERIOD) {
                return ProposalState.Queued;
            }
            return ProposalState.Expired;
        }

        return ProposalState.Defeated;
    }

    /// @notice Get proposal details
    /// @param proposalId Proposal to query
    /// @return Proposal struct
    function getProposal(uint256 proposalId) external view override returns (Proposal memory) {
        return _proposals[proposalId];
    }

    /// @notice Check if account has voted
    /// @param proposalId Proposal to query
    /// @param account Account to check
    /// @return True if voted
    function hasVoted(uint256 proposalId, address account) external view override returns (bool) {
        return _hasVoted[proposalId][account];
    }

    /// @notice Get voting power of an account
    /// @param account Account to query
    /// @return Voting weight
    function getVotes(address account) public view override returns (uint256) {
        if (isNFTVoting) {
            // ERC721 balance (1 NFT = 1 vote)
            return _getERC721Balance(account);
        }
        // ERC20 balance
        return _getERC20Balance(account);
    }

    /// @notice Get quorum requirement
    /// @return Quorum in voting tokens
    function quorum() public view override returns (uint256) {
        uint256 totalSupply = _getTotalSupply();
        return (totalSupply * _quorumNumerator) / BPS_DENOMINATOR;
    }

    function proposalThreshold() external view override returns (uint256) {
        return _proposalThreshold;
    }

    function votingDelay() external view override returns (uint256) {
        return _votingDelay;
    }

    function votingPeriod() external view override returns (uint256) {
        return _votingPeriod;
    }

    function timelockDelay() external view override returns (uint256) {
        return _timelockDelay;
    }

    // -------------------------------------------------------------------------
    // Internal Validation
    // -------------------------------------------------------------------------

    /// @notice Validate fee config within absolute bounds
    function _validateBounds(FeeConfig calldata config) internal pure {
        require(
            config.gasLimit >= MIN_GAS_LIMIT && config.gasLimit <= MAX_GAS_LIMIT,
            "FeeGovernor: gasLimit out of bounds"
        );
        require(
            config.targetBlockRate >= MIN_TARGET_BLOCK_RATE &&
            config.targetBlockRate <= MAX_TARGET_BLOCK_RATE,
            "FeeGovernor: targetBlockRate out of bounds"
        );
        require(
            config.minBaseFee >= MIN_BASE_FEE && config.minBaseFee <= MAX_BASE_FEE,
            "FeeGovernor: minBaseFee out of bounds"
        );
        require(
            config.baseFeeChangeDenominator >= MIN_BASE_FEE_CHANGE_DENOM &&
            config.baseFeeChangeDenominator <= MAX_BASE_FEE_CHANGE_DENOM,
            "FeeGovernor: baseFeeChangeDenominator out of bounds"
        );
        require(
            config.minBlockGasCost <= config.maxBlockGasCost,
            "FeeGovernor: minBlockGasCost > maxBlockGasCost"
        );
    }

    /// @notice Validate max 10% change per parameter
    function _validateMaxChange(FeeConfig calldata proposed) internal view {
        (
            uint256 curGasLimit,
            uint256 curTargetBlockRate,
            uint256 curMinBaseFee,
            uint256 curTargetGas,
            uint256 curBaseFeeChangeDenom,
            uint256 curMinBlockGasCost,
            uint256 curMaxBlockGasCost,
            uint256 curBlockGasCostStep
        ) = feeManager.getFeeConfig();

        _checkMaxChange(curGasLimit, proposed.gasLimit, "gasLimit");
        _checkMaxChange(curTargetBlockRate, proposed.targetBlockRate, "targetBlockRate");
        _checkMaxChange(curMinBaseFee, proposed.minBaseFee, "minBaseFee");
        _checkMaxChange(curTargetGas, proposed.targetGas, "targetGas");
        _checkMaxChange(curBaseFeeChangeDenom, proposed.baseFeeChangeDenominator, "baseFeeChangeDenom");
        _checkMaxChange(curMinBlockGasCost, proposed.minBlockGasCost, "minBlockGasCost");
        _checkMaxChange(curMaxBlockGasCost, proposed.maxBlockGasCost, "maxBlockGasCost");
        _checkMaxChange(curBlockGasCostStep, proposed.blockGasCostStep, "blockGasCostStep");
    }

    /// @notice Check single parameter does not exceed 10% change
    function _checkMaxChange(
        uint256 current,
        uint256 proposed,
        string memory param
    ) internal pure {
        if (current == 0 && proposed == 0) return;
        if (current == 0) {
            // Allow setting from zero within bounds
            return;
        }

        uint256 maxDelta = (current * MAX_CHANGE_BPS) / BPS_DENOMINATOR;
        uint256 delta = proposed > current ? proposed - current : current - proposed;

        require(delta <= maxDelta, string.concat("FeeGovernor: ", param, " exceeds 10%"));
    }

    /// @notice Check if quorum is reached
    function _quorumReached(uint256 proposalId) internal view returns (bool) {
        Proposal storage proposal = _proposals[proposalId];
        return (proposal.forVotes + proposal.abstainVotes) >= quorum();
    }

    /// @notice Check if vote succeeded (more for than against)
    function _voteSucceeded(uint256 proposalId) internal view returns (bool) {
        Proposal storage proposal = _proposals[proposalId];
        return proposal.forVotes > proposal.againstVotes;
    }

    // -------------------------------------------------------------------------
    // Token Interface Calls
    // -------------------------------------------------------------------------

    function _getERC20Balance(address account) internal view returns (uint256) {
        (bool success, bytes memory data) = votingToken.staticcall(
            abi.encodeWithSignature("balanceOf(address)", account)
        );
        require(success && data.length >= 32, "FeeGovernor: balance call failed");
        return abi.decode(data, (uint256));
    }

    function _getERC721Balance(address account) internal view returns (uint256) {
        (bool success, bytes memory data) = votingToken.staticcall(
            abi.encodeWithSignature("balanceOf(address)", account)
        );
        require(success && data.length >= 32, "FeeGovernor: balance call failed");
        return abi.decode(data, (uint256));
    }

    function _getTotalSupply() internal view returns (uint256) {
        (bool success, bytes memory data) = votingToken.staticcall(
            abi.encodeWithSignature("totalSupply()")
        );
        require(success && data.length >= 32, "FeeGovernor: totalSupply call failed");
        return abi.decode(data, (uint256));
    }
}
