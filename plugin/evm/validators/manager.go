// manager is a no-op stub that allows the EVM library to compile without node-level dependencies.
package validators

// manager is a no-op stubbed validator manager.
type manager struct{}

// NewManager returns a stub manager so the EVM library can compile without node-level dependencies.
func NewManager(
	ctx interface{},
	db interface{},
	clock interface{},
) (*manager, error) {
	return nil, nil
}