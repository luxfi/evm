//go:build node_validators

package validators

import (
	"github.com/luxfi/node/ids"
	"github.com/luxfi/node/consensus"
	"github.com/luxfi/node/consensus/validators"
	"github.com/luxfi/node/database"
	"github.com/luxfi/node/utils/timer/mockable"
)

// manager wraps the consensus validator manager for EVM usage
type manager struct {
	validators.Manager
	ctx      *consensus.Context
	subnetID ids.ID
	chainID  ids.ID
	db       database.Database
	clock    *mockable.Clock
}

// NewManager returns the actual validator manager implementation.
func NewManager(
	ctx *consensus.Context,
	db database.Database,
	clock *mockable.Clock,
) (*manager, error) {
	m := validators.NewManager()
	return &manager{
		Manager:   m,
		ctx:       ctx,
		subnetID:  ctx.SubnetID,
		chainID:   ctx.ChainID,
		db:        db,
		clock:     clock,
	}, nil
}