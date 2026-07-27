// (c) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
// SPDX-License-Identifier: LicenseRef-Ecosystem
pragma solidity ^0.8.0;

/**
 * A verified claim about P-chain stake, produced by the stakeWeight precompile.
 *
 * `weight` is the validator's own stake PLUS every LUX delegated to it: the
 * P-chain keeps one weight per nodeID and folds each delegator into it, so a
 * delegation can never be counted twice and delegators do not vote separately.
 * Both weights are P-chain uint64 values widened to uint256 here so a tally can
 * multiply without overflowing.
 */
struct StakeWeight {
    bytes20 nodeID;
    address voter;
    uint64 authEpoch;
    uint64 pChainHeight;
    uint256 weight;
    uint256 totalWeight;
}

/**
 * IStakeWeight reads a stake-weight ballot the transaction carried in its
 * access list, at 0x0200000000000000000000000000000000000006.
 *
 * The precompile answers ONE question and holds no state, no owner and no
 * roles: at the P-chain height the ballot names, did this nodeID hold at least
 * this weight out of exactly this total, does it STILL hold at least that much
 * at the block's P-chain height, and has its P-chain-registered BLS key
 * authorised `voter` to speak for it? A ballot that overstates any of those
 * returns valid=false with a zeroed struct.
 *
 * A consuming contract owes four checks, and the precompile deliberately does
 * not make them for you — they are policy, not fact:
 *
 *   1. `valid` is true.
 *   2. `msg.sender == sw.voter`. The attestation is a public fact once the
 *      ballot bytes are on chain; only the authorised address may spend it.
 *   3. `sw.pChainHeight == proposal.snapshotHeight`, pinned when the proposal
 *      was created. This is what stops a voter picking a favourable snapshot,
 *      and it is why `sw.totalWeight` is a safe denominator: every ballot in a
 *      proposal reports the same one.
 *   4. `!hasVoted[proposalId][sw.nodeID]`, then set it. A ballot is replayable
 *      by construction — the same bytes verify every time — so the ONLY thing
 *      stopping a validator counting its weight twice is this flag, keyed by
 *      nodeID and not by address.
 *
 * Plus a fifth if authorisations should be revocable: keep
 * `lastEpoch[nodeID]`, reject `sw.authEpoch < lastEpoch[nodeID]`, and raise it
 * on a greater one. A validator rotates its voting address by signing a higher
 * epoch. No registry, no admin, nobody who can revoke on its behalf.
 */
interface IStakeWeight {
    /**
     * @param index which of this transaction's stakeWeight access-list entries to read
     * @return stakeWeight the verified claim, zeroed when valid is false
     * @return valid whether the block's committed predicate results attest to it
     */
    function getVerifiedStakeWeight(uint32 index)
        external
        view
        returns (StakeWeight memory stakeWeight, bool valid);
}
