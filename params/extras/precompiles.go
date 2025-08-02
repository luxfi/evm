// (c) 2023 Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package extras

import (
	"encoding/json"
	"fmt"

	"github.com/luxfi/evm/precompile/registry"
	"github.com/luxfi/evm/precompile/precompileconfig"
)

type Precompiles map[string]precompileconfig.Config

// UnmarshalJSON parses the JSON-encoded data into the ChainConfigPrecompiles.
// ChainConfigPrecompiles is a map of precompile module keys to their
// configuration.
func (ccp *Precompiles) UnmarshalJSON(data []byte) error {
	raw := make(map[string]json.RawMessage)
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*ccp = make(Precompiles)
	for _, module := range registry.RegisteredModules() {
		key := module.ConfigKey()
		if value, ok := raw[key]; ok {
			conf := module.MakeConfig()
			if err := json.Unmarshal(value, conf); err != nil {
				return err
			}
			// Type assert to precompileconfig.Config
			if cfg, ok := conf.(precompileconfig.Config); ok {
				(*ccp)[key] = cfg
			} else {
				return fmt.Errorf("config for %s does not implement precompileconfig.Config", key)
			}
		}
	}
	return nil
}
