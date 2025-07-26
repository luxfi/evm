# Network Upgrades Guide for Lux EVM v2.0.0

## Overview

Starting with Lux EVM v2.0.0, all existing network upgrades (Shanghai, Cancun, Durango, etc.) are active from genesis. This simplifies the initial deployment and ensures all modern features are available immediately.

However, the infrastructure for future network upgrades is maintained to allow for protocol evolution.

## How Network Upgrades Work

### 1. NetworkUpgrades Structure

The `NetworkUpgrades` struct in `params/extras/network_upgrades.go` currently contains:

```go
type NetworkUpgrades struct {
    // GenesisTimestamp is when the genesis network upgrade activates.
    // For v2.0.0, this is always set to 0 (activated at genesis).
    GenesisTimestamp *uint64 `json:"genesisTimestamp,omitempty"`
}
```

### 2. Adding a New Network Upgrade

To add a new network upgrade (e.g., "NextUpgrade"):

1. **Update NetworkUpgrades struct**:
```go
type NetworkUpgrades struct {
    GenesisTimestamp *uint64 `json:"genesisTimestamp,omitempty"`
    NextUpgradeTimestamp *uint64 `json:"nextUpgradeTimestamp,omitempty"`
}
```

2. **Add activation check method**:
```go
func (n *NetworkUpgrades) IsNextUpgrade(time uint64) bool {
    return utils.IsTimestampForked(n.NextUpgradeTimestamp, time)
}
```

3. **Update ChainConfig methods** in `params/config.go`:
```go
func (c *ChainConfig) IsNextUpgrade(time uint64) bool {
    return utils.IsTimestampForked(c.MandatoryNetworkUpgrades.NextUpgradeTimestamp, time)
}
```

4. **Update Rules struct** to include the upgrade flag:
```go
type Rules struct {
    // ... existing fields ...
    IsNextUpgradeEnabled bool
}
```

5. **Update rules() function** to set the flag:
```go
func (c *ChainConfig) rules(num *big.Int, timestamp uint64) Rules {
    return Rules{
        // ... existing fields ...
        IsNextUpgradeEnabled: c.IsNextUpgrade(timestamp),
    }
}
```

### 3. Using Network Upgrades in Code

Once added, you can use the upgrade flag in your code:

```go
if rules.IsNextUpgradeEnabled {
    // New behavior
} else {
    // Old behavior
}
```

### 4. Configuration

Network upgrades are configured through the chain configuration:

```json
{
  "networkUpgrades": {
    "genesisTimestamp": 0,
    "nextUpgradeTimestamp": 1234567890
  }
}
```

### 5. Testing Network Upgrades

Create tests that verify behavior before and after the upgrade:

```go
func TestNextUpgrade(t *testing.T) {
    // Test config with upgrade at timestamp 1000
    config := &ChainConfig{
        MandatoryNetworkUpgrades: MandatoryNetworkUpgrades{
            GenesisTimestamp: utils.NewUint64(0),
            NextUpgradeTimestamp: utils.NewUint64(1000),
        },
    }
    
    // Before upgrade
    rules := config.GenesisRules(big.NewInt(0), 999)
    require.False(t, rules.IsNextUpgradeEnabled)
    
    // After upgrade
    rules = config.GenesisRules(big.NewInt(0), 1000)
    require.True(t, rules.IsNextUpgradeEnabled)
}
```

## Best Practices

1. **Timestamp-based activation**: Always use timestamps, not block numbers, for network upgrades in Lux.

2. **Backward compatibility**: Ensure new upgrades don't break existing functionality.

3. **Gradual rollout**: Test thoroughly on testnets before mainnet activation.

4. **Clear documentation**: Document what changes each upgrade brings.

5. **Upgrade coordination**: Coordinate with node operators to ensure smooth network-wide activation.

## Example: Adding a Hypothetical "Phoenix" Upgrade

Here's a complete example of adding a new "Phoenix" network upgrade:

1. **Update network_upgrades.go**:
```go
type NetworkUpgrades struct {
    GenesisTimestamp *uint64 `json:"genesisTimestamp,omitempty"`
    PhoenixTimestamp *uint64 `json:"phoenixTimestamp,omitempty"`
}

func (n *NetworkUpgrades) IsPhoenix(time uint64) bool {
    return utils.IsTimestampForked(n.PhoenixTimestamp, time)
}
```

2. **Update config.go**:
```go
func (c *ChainConfig) IsPhoenix(time uint64) bool {
    return utils.IsTimestampForked(c.MandatoryNetworkUpgrades.PhoenixTimestamp, time)
}
```

3. **Use in code**:
```go
if c.IsPhoenix(timestamp) {
    // Apply Phoenix upgrade changes
    newGasLimit = 30_000_000
} else {
    newGasLimit = 15_000_000
}
```

## Testing Framework

The upgrade testing framework should verify:

1. Pre-upgrade behavior
2. Exact upgrade activation
3. Post-upgrade behavior
4. State transition correctness
5. Consensus compatibility

See `plugin/evm/vm_upgrade_bytes_test.go` for examples of upgrade testing patterns.