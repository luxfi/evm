//go:build !node_validators

package interfaces

// Validator is a stub interface for when validators are disabled
type Validator struct {
	NodeID      []byte
	PublicKey   []byte
	TxID        []byte
	Weight      uint64
	StartTime   uint64
	IsActive    bool
	IsCurrently bool
	IsConnected bool
	Uptime      uint64
}
