// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"math/big"
	"sync"
	"testing"

	"github.com/luxfi/evm/params"
	"github.com/luxfi/evm/params/extras"
	"github.com/luxfi/evm/precompile/contracts/txallowlist"
	"github.com/luxfi/evm/utils"
	"github.com/luxfi/geth/common"
	ethparams "github.com/luxfi/geth/params"
	"github.com/stretchr/testify/require"
)

// upgradedAt is a chain on which Durango and the tx allow list both start at
// time t. Rules evaluated before t and at t are two rule sets that differ in
// what IntrinsicGas charges a contract creation (Durango's init code words) and
// in whether the tx allow list is on.
func upgradedAt(t uint64) *params.ChainConfig {
	return params.WithExtra(&params.ChainConfig{ChainID: big.NewInt(1)}, &extras.ChainConfig{
		NetworkUpgrades: extras.NetworkUpgrades{DurangoTimestamp: utils.NewUint64(t)},
		UpgradeConfig: extras.UpgradeConfig{PrecompileUpgrades: []extras.PrecompileUpgrade{
			{Config: txallowlist.NewConfig(utils.NewUint64(t), nil, nil, nil)},
		}},
	})
}

// initCode is 64 non-zero bytes of creation data: two init code words.
var initCode = func() []byte {
	b := make([]byte, 64)
	for i := range b {
		b[i] = 0xAA
	}
	return b
}()

// ruleSet is one chain config at one time, and what its rules must say there.
type ruleSet struct {
	name    string
	config  *params.ChainConfig
	time    uint64
	upgrade bool // Durango and the tx allow list are on
}

// intrinsicGas is what a creation carrying initCode must be charged on these
// chains, which have no Ethereum forks scheduled: the frontier data price, and
// Durango's init code word gas when it is on.
func (r ruleSet) intrinsicGas() uint64 {
	gas := ethparams.TxGas + uint64(len(initCode))*ethparams.TxDataNonZeroGasFrontier
	if r.upgrade {
		gas += 2 * ethparams.InitCodeWordGas
	}
	return gas
}

// check evaluates r's Rules and asserts that what they say is r's own.
func (r ruleSet) check() (string, bool) {
	rules := r.config.Rules(common.Big1, params.IsMergeTODO, r.time)
	extra := params.GetRulesExtra(rules)
	if extra.IsDurango != r.upgrade {
		return "IsDurango", false
	}
	if extra.IsPrecompileEnabled(txallowlist.ContractAddress) != r.upgrade {
		return "the tx allow list", false
	}
	gas, err := IntrinsicGas(initCode, nil, true, rules)
	if err != nil || gas != r.intrinsicGas() {
		return "IntrinsicGas", false
	}
	return "", true
}

// Rules evaluated later do not change what Rules evaluated earlier say: each
// Rules carries the chain config and time it was evaluated at, and its Lux
// rules follow from those alone.
func TestRulesExtraIsTheRulesOwn(t *testing.T) {
	config := upgradedAt(100)
	before := config.Rules(common.Big1, params.IsMergeTODO, 99)
	at := config.Rules(common.Big1, params.IsMergeTODO, 100)

	require.False(t, params.GetRulesExtra(before).IsDurango, "Rules before Durango read as Durango")
	require.False(t, params.GetRulesExtra(before).IsPrecompileEnabled(txallowlist.ContractAddress))
	require.True(t, params.GetRulesExtra(at).IsDurango)
	require.True(t, params.GetRulesExtra(at).IsPrecompileEnabled(txallowlist.ContractAddress))

	gasBefore, err := IntrinsicGas(initCode, nil, true, before)
	require.NoError(t, err)
	gasAt, err := IntrinsicGas(initCode, nil, true, at)
	require.NoError(t, err)
	require.Equal(t, ruleSet{upgrade: false}.intrinsicGas(), gasBefore)
	require.Equal(t, ruleSet{upgrade: true}.intrinsicGas(), gasAt)
}

// Goroutines evaluating different rule sets at once each see their own: two
// times on one chain, and one time on two chains. Under -race this also holds
// the evaluation free of shared writes.
func TestRulesExtraUnderConcurrentEvaluation(t *testing.T) {
	a, b := upgradedAt(100), upgradedAt(200)
	sets := []ruleSet{
		{name: "a before", config: a, time: 99, upgrade: false},
		{name: "a at", config: a, time: 100, upgrade: true},
		{name: "b between", config: b, time: 150, upgrade: false},
		{name: "a between", config: a, time: 150, upgrade: true},
	}
	const iterations = 2000

	var wg sync.WaitGroup
	for _, r := range sets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if what, ok := r.check(); !ok {
					t.Errorf("%s: %s is another goroutine's at iteration %d", r.name, what, i)
					return
				}
			}
		}()
	}
	wg.Wait()
}
