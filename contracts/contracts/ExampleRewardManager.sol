//SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "./interfaces/IRewardManager.sol";
import "@openzeppelin/contracts/access/Ownable.sol";

// LP-aligned address: P=0 (Core), C=2 (C-Chain), II=05 (RewardManager)
address constant REWARD_MANAGER_ADDRESS = 0x0000000000000000000000000000000000010205;

// ExampleRewardManager is a sample wrapper contract for RewardManager precompile.
contract ExampleRewardManager is Ownable {
  IRewardManager rewardManager = IRewardManager(REWARD_MANAGER_ADDRESS);

  constructor() Ownable() {}

  function currentRewardAddress() public view returns (address) {
    return rewardManager.currentRewardAddress();
  }

  function setRewardAddress(address addr) public onlyOwner {
    rewardManager.setRewardAddress(addr);
  }

  function allowFeeRecipients() public onlyOwner {
    rewardManager.allowFeeRecipients();
  }

  function disableRewards() public onlyOwner {
    rewardManager.disableRewards();
  }

  function areFeeRecipientsAllowed() public view returns (bool) {
    return rewardManager.areFeeRecipientsAllowed();
  }
}
