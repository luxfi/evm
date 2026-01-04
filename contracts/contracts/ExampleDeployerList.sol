//SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "@openzeppelin/contracts/access/Ownable.sol";
import "./interfaces/IAllowList.sol";
import "./AllowList.sol";

// LP-aligned address: P=0 (Core), C=2 (C-Chain), II=01 (DeployerAllowList)
address constant DEPLOYER_LIST = 0x0000000000000000000000000000000000010201;

// ExampleDeployerList shows how ContractDeployerAllowList precompile can be used in a smart contract
// All methods of [allowList] can be directly called. There are example calls as tasks in hardhat.config.ts file.
contract ExampleDeployerList is AllowList {
  // Precompiled Allow List Contract Address
  constructor() AllowList(DEPLOYER_LIST) {}

  function deployContract() public {
    new Example();
  }
}

// This is an empty contract that can be used to test contract deployment
contract Example {}
