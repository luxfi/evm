// SPDX-License-Identifier: MIT
// FeeGovernor test suite

import { expect } from "chai";
import { ethers } from "hardhat";
import { FeeGovernor } from "../typechain-types";
import { MockERC20 } from "../typechain-types/contracts/test/FeeGovernorTest.sol";

describe("FeeGovernor", function () {
  let governor: FeeGovernor;
  let token: MockERC20;
  let guardian: string;
  let alice: string;
  let bob: string;

  const VOTING_DELAY = 1n;
  const VOTING_PERIOD = 10n;
  const TIMELOCK_DELAY = 48n * 60n * 60n; // 48 hours in seconds
  const PROPOSAL_THRESHOLD = 100n;
  const QUORUM_BPS = 400n; // 4%

  beforeEach(async function () {
    const signers = await ethers.getSigners();
    guardian = signers[0].address;
    alice = signers[1].address;
    bob = signers[2].address;

    // Deploy mock ERC20 token
    const MockERC20Factory = await ethers.getContractFactory("MockERC20");
    token = await MockERC20Factory.deploy() as MockERC20;
    await token.waitForDeployment();

    // Mint tokens
    await token.mint(alice, 1000n);
    await token.mint(bob, 500n);

    // Deploy FeeGovernor
    const FeeGovernorFactory = await ethers.getContractFactory("FeeGovernor");
    governor = await FeeGovernorFactory.deploy(
      await token.getAddress(),
      false, // ERC20 voting
      guardian,
      VOTING_DELAY,
      VOTING_PERIOD,
      TIMELOCK_DELAY,
      PROPOSAL_THRESHOLD,
      QUORUM_BPS
    ) as FeeGovernor;
    await governor.waitForDeployment();
  });

  describe("Constructor", function () {
    it("should set correct voting parameters", async function () {
      expect(await governor.votingDelay()).to.equal(VOTING_DELAY);
      expect(await governor.votingPeriod()).to.equal(VOTING_PERIOD);
      expect(await governor.timelockDelay()).to.equal(TIMELOCK_DELAY);
      expect(await governor.proposalThreshold()).to.equal(PROPOSAL_THRESHOLD);
    });

    it("should set correct guardian", async function () {
      expect(await governor.guardian()).to.equal(guardian);
    });

    it("should reject zero voting token", async function () {
      const FeeGovernorFactory = await ethers.getContractFactory("FeeGovernor");
      await expect(
        FeeGovernorFactory.deploy(
          ethers.ZeroAddress,
          false,
          guardian,
          VOTING_DELAY,
          VOTING_PERIOD,
          TIMELOCK_DELAY,
          PROPOSAL_THRESHOLD,
          QUORUM_BPS
        )
      ).to.be.revertedWith("FeeGovernor: zero voting token");
    });

    it("should reject invalid timelock delay", async function () {
      const FeeGovernorFactory = await ethers.getContractFactory("FeeGovernor");
      await expect(
        FeeGovernorFactory.deploy(
          await token.getAddress(),
          false,
          guardian,
          VOTING_DELAY,
          VOTING_PERIOD,
          1n, // Too short
          PROPOSAL_THRESHOLD,
          QUORUM_BPS
        )
      ).to.be.revertedWith("FeeGovernor: invalid timelock delay");
    });
  });

  describe("Voting Power", function () {
    it("should return correct voting power for token holders", async function () {
      expect(await governor.getVotes(alice)).to.equal(1000n);
      expect(await governor.getVotes(bob)).to.equal(500n);
    });

    it("should return zero for non-holders", async function () {
      const nonHolder = (await ethers.getSigners())[5].address;
      expect(await governor.getVotes(nonHolder)).to.equal(0n);
    });
  });

  describe("Quorum", function () {
    it("should calculate quorum correctly", async function () {
      // Total supply: 1500, quorum: 4% = 60
      const totalSupply = await token.totalSupply();
      const expectedQuorum = (totalSupply * QUORUM_BPS) / 10000n;
      expect(await governor.quorum()).to.equal(expectedQuorum);
    });
  });

  describe("Emergency Functions", function () {
    it("should allow guardian to pause", async function () {
      expect(await governor.paused()).to.equal(false);
      await governor.pause();
      expect(await governor.paused()).to.equal(true);
    });

    it("should allow guardian to unpause", async function () {
      await governor.pause();
      expect(await governor.paused()).to.equal(true);
      await governor.unpause();
      expect(await governor.paused()).to.equal(false);
    });

    it("should reject non-guardian pause", async function () {
      const signers = await ethers.getSigners();
      await expect(
        governor.connect(signers[1]).pause()
      ).to.be.revertedWith("FeeGovernor: not guardian");
    });

    it("should allow guardian transfer", async function () {
      const signers = await ethers.getSigners();
      const newGuardian = signers[3].address;
      await governor.transferGuardian(newGuardian);
      expect(await governor.guardian()).to.equal(newGuardian);
    });
  });

  describe("Constants", function () {
    it("should have correct max change BPS", async function () {
      expect(await governor.MAX_CHANGE_BPS()).to.equal(1000n);
    });

    it("should have correct BPS denominator", async function () {
      expect(await governor.BPS_DENOMINATOR()).to.equal(10000n);
    });

    it("should have correct min timelock delay", async function () {
      expect(await governor.MIN_TIMELOCK_DELAY()).to.equal(48n * 60n * 60n);
    });

    it("should have correct max timelock delay", async function () {
      expect(await governor.MAX_TIMELOCK_DELAY()).to.equal(72n * 60n * 60n);
    });
  });
});
