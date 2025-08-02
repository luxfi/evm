// (c) 2020-2023, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package allowlist_test

import (
	"testing"
	"github.com/luxfi/evm/v2/v2/core/extstate/testhelpers"
	"github.com/luxfi/evm/v2/v2/precompile/allowlist"
	"github.com/luxfi/evm/v2/v2/precompile/contract"
	"github.com/luxfi/evm/v2/v2/precompile/registry"
	"github.com/luxfi/evm/v2/v2/precompile/precompileconfig"
	"github.com/luxfi/geth/common"
)

var (
	_ precompileconfig.Config = &dummyConfig{}
	_ contract.Configurator   = &dummyConfigurator{}

	dummyAddr = common.Address{1}
)

type dummyConfig struct {
	precompileconfig.Upgrade
	allowlist.AllowListConfig
}

func (d *dummyConfig) Key() string      { return "dummy" }
func (d *dummyConfig) IsDisabled() bool { return false }
func (d *dummyConfig) Verify(chainConfig precompileconfig.ChainConfig) error {
	return d.AllowListConfig.Verify(chainConfig, d.Upgrade)
}

func (d *dummyConfig) Equal(cfg precompileconfig.Config) bool {
	other, ok := (cfg).(*dummyConfig)
	if !ok {
		return false
	}
	return d.AllowListConfig.Equal(&other.AllowListConfig)
}

type dummyConfigurator struct{}

func (d *dummyConfigurator) MakeConfig() precompileconfig.Config {
	return &dummyConfig{}
}

func (d *dummyConfigurator) Configure(
	chainConfig precompileconfig.ChainConfig,
	precompileConfig precompileconfig.Config,
	state contract.StateDB,
	blockContext contract.ConfigurationBlockContext,
) error {
	cfg := precompileConfig.(*dummyConfig)
	return cfg.AllowListConfig.Configure(chainConfig, dummyAddr, state, blockContext)
}

func TestAllowListRun(t *testing.T) {
	dummyModule := registry.NewModule(
		"dummy",
		dummyAddr,
		allowlist.CreateAllowListPrecompile(dummyAddr),
		&dummyConfigurator{},
	)
	allowlist.RunPrecompileWithAllowListTests(t, dummyModule, testhelpers.NewTestStateDB, nil)
}

func BenchmarkAllowList(b *testing.B) {
	dummyModule := registry.NewModule(
		"dummy",
		dummyAddr,
		allowlist.CreateAllowListPrecompile(dummyAddr),
		&dummyConfigurator{},
	)
	allowlist.BenchPrecompileWithAllowList(b, dummyModule, testhelpers.NewTestStateDB, nil)
}
