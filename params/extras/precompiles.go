// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package extras

import (
	"bytes"
	"encoding/json"
	"sort"

	"github.com/luxfi/evm/precompile/modules"
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
	for _, module := range modules.RegisteredModules() {
		key := module.ConfigKey
		if value, ok := raw[key]; ok {
			conf := module.MakeConfig()
			if err := json.Unmarshal(value, conf); err != nil {
				return err
			}
			(*ccp)[key] = conf
		}
	}
	return nil
}

// MarshalJSONDeterministic returns the JSON encoding of the Precompiles map
// with keys sorted alphabetically to ensure deterministic output.
// This is critical for genesis hash consistency across builds.
func (ccp Precompiles) MarshalJSONDeterministic() ([]byte, error) {
	if len(ccp) == 0 {
		return []byte("{}"), nil
	}

	// Sort keys for deterministic iteration
	keys := make([]string, 0, len(ccp))
	for k := range ccp {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build JSON manually with sorted keys
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		// Marshal key
		keyBytes, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(keyBytes)
		buf.WriteByte(':')
		// Marshal value
		valBytes, err := json.Marshal(ccp[k])
		if err != nil {
			return nil, err
		}
		buf.Write(valBytes)
	}
	buf.WriteByte('}')

	return buf.Bytes(), nil
}
