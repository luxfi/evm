//go:build evm_novalidators
// +build evm_novalidators

package validators

// manager is a no-op stub when validators are disabled via build tag evm_novalidators.
type manager struct{}

// NewManager is disabled under the evm_novalidators build tag.
// It returns a stub manager so the EVM library can compile without node-level dependencies.
func NewManager(
	ctx interface{},
	db interface{},
	clock interface{},
) (*manager, error) {
   return nil, nil
}