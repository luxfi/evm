// (c) 2020-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package utils

import (
	"context"
	"errors"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
	luxlog "github.com/luxfi/log"
	"github.com/luxfi/metrics"
	"github.com/luxfi/node/v2/utils/constants"
	"github.com/luxfi/node/v2/vms/platformvm/warp"
	"github.com/luxfi/node/v2/quasar"
	"github.com/luxfi/node/v2/quasar/validators"
	"github.com/luxfi/node/v2/upgrade"
	"github.com/luxfi/evm/localsigner"
	"github.com/luxfi/evm/commontype"
)

var (
	testChainID  = ids.ID{5, 4, 3, 2, 1}
	testCChainID = ids.ID{1, 2, 3, 4, 5}
	testXChainID = ids.ID{2, 3, 4, 5, 6}
)

// warpSignerAdapter adapts warp.Signer to quasar.WarpSigner
type warpSignerAdapter struct {
	signer warp.Signer
}

// Sign implements the WarpSigner interface
func (w *warpSignerAdapter) Sign(msg *quasar.WarpMessage) (*quasar.WarpSignature, error) {
	// For testing, we'll create a simple implementation
	// In production, this would need proper conversion between message types
	unsignedMsg := &warp.UnsignedMessage{
		// Convert fields from msg
	}
	_, err := w.signer.Sign(unsignedMsg)
	if err != nil {
		return nil, err
	}
	// Convert the signature to quasar.WarpSignature
	return &quasar.WarpSignature{
		// Signature fields would be populated from sig
	}, nil
}

// noopRegistry is a no-op implementation of metrics.Registry for testing
type noopRegistry struct{}

func (n *noopRegistry) Register(c metrics.Collector) error {
	return nil
}

func (n *noopRegistry) MustRegister(c metrics.Collector) {}

func (n *noopRegistry) Unregister(c metrics.Collector) bool {
	return true
}

func (n *noopRegistry) Gather() ([]*metrics.MetricFamily, error) {
	return nil, nil
}

func TestConsensusContext() *quasar.Context {
	signer, err := localsigner.New()
	if err != nil {
		panic(err)
	}
	pk := signer.PublicKey()
	networkID := constants.UnitTestID
	chainID := testChainID

	ctx := &quasar.Context{
		NetworkID:       networkID,
		SubnetID:        ids.Empty,
		ChainID:         chainID,
		NodeID:          ids.GenerateTestNodeID(),
		XChainID:        testXChainID,
		CChainID:        testCChainID,
		NetworkUpgrades: &upgrade.Default,
		PublicKey:       bls.PublicKeyToUncompressedBytes(pk),
		WarpSigner:      &warpSignerAdapter{signer: warp.NewSigner(signer, networkID, chainID)},
		Log:             luxlog.NewNoOpLogger(),
		BCLookup:        ids.NewAliaser(),
		Metrics:         &noopRegistry{},
		ChainDataDir:    "",
		ValidatorState:  NewTestValidatorState(),
	}

	aliaser := ctx.BCLookup.(ids.Aliaser)
	_ = aliaser.Alias(testCChainID, "C")
	_ = aliaser.Alias(testCChainID, testCChainID.String())
	_ = aliaser.Alias(testXChainID, "X")
	_ = aliaser.Alias(testXChainID, testXChainID.String())

	return ctx
}

// ConvertToChainContext converts a quasar.Context to commontype.ChainContext
func ConvertToChainContext(ctx *quasar.Context) *commontype.ChainContext {
	// Convert 20-byte NodeID to 32-byte NodeID by padding with zeros
	var nodeID commontype.NodeID
	copy(nodeID[:], ctx.NodeID[:])
	
	return &commontype.ChainContext{
		NetworkID:    ctx.NetworkID,
		SubnetID:     commontype.SubnetID(ctx.SubnetID),
		ChainID:      commontype.ChainID(ctx.ChainID),
		NodeID:       nodeID,
		AppVersion:   uint32(0), // Default for testing
		ChainDataDir: ctx.ChainDataDir,
	}
}

// TestChainContext returns a test ChainContext
func TestChainContext() *commontype.ChainContext {
	return ConvertToChainContext(TestConsensusContext())
}

// TestValidatorState is a test implementation of validator state
type TestValidatorState struct {
	GetMinimumHeightF       func(context.Context) (uint64, error)
	GetCurrentHeightF       func(context.Context) (uint64, error)
	GetSubnetIDF            func(context.Context, ids.ID) (ids.ID, error)
	GetValidatorSetF        func(context.Context, uint64, ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error)
	GetCurrentValidatorSetF func(context.Context, ids.ID) (map[ids.ID]*validators.GetCurrentValidatorOutput, uint64, error)
}

func NewTestValidatorState() *TestValidatorState {
	return &TestValidatorState{
		GetMinimumHeightF: func(context.Context) (uint64, error) {
			return 0, nil
		},
		GetCurrentHeightF: func(context.Context) (uint64, error) {
			return 0, nil
		},
		GetSubnetIDF: func(_ context.Context, chainID ids.ID) (ids.ID, error) {
			subnetID, ok := map[ids.ID]ids.ID{
				constants.QuantumChainID: constants.PrimaryNetworkID,
				testXChainID:              constants.PrimaryNetworkID,
				testCChainID:              constants.PrimaryNetworkID,
			}[chainID]
			if !ok {
				return ids.Empty, errors.New("unknown chain")
			}
			return subnetID, nil
		},
		GetValidatorSetF: func(context.Context, uint64, ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
			return map[ids.NodeID]*validators.GetValidatorOutput{}, nil
		},
		GetCurrentValidatorSetF: func(context.Context, ids.ID) (map[ids.ID]*validators.GetCurrentValidatorOutput, uint64, error) {
			return map[ids.ID]*validators.GetCurrentValidatorOutput{}, 0, nil
		},
	}
}

func (tvs *TestValidatorState) GetMinimumHeight(ctx context.Context) (uint64, error) {
	if tvs.GetMinimumHeightF != nil {
		return tvs.GetMinimumHeightF(ctx)
	}
	return 0, nil
}

func (tvs *TestValidatorState) GetCurrentHeight(ctx context.Context) (uint64, error) {
	if tvs.GetCurrentHeightF != nil {
		return tvs.GetCurrentHeightF(ctx)
	}
	return 0, nil
}

func (tvs *TestValidatorState) GetSubnetID(ctx context.Context, chainID ids.ID) (ids.ID, error) {
	if tvs.GetSubnetIDF != nil {
		return tvs.GetSubnetIDF(ctx, chainID)
	}
	return ids.Empty, nil
}

func (tvs *TestValidatorState) GetValidatorSet(ctx context.Context, height uint64, subnetID ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
	if tvs.GetValidatorSetF != nil {
		return tvs.GetValidatorSetF(ctx, height, subnetID)
	}
	return nil, nil
}

func (tvs *TestValidatorState) GetCurrentValidatorSet(ctx context.Context, subnetID ids.ID) (map[ids.ID]*validators.GetCurrentValidatorOutput, uint64, error) {
	if tvs.GetCurrentValidatorSetF != nil {
		return tvs.GetCurrentValidatorSetF(ctx, subnetID)
	}
	return nil, 0, nil
}

func (tvs *TestValidatorState) ApplyValidatorWeightDiffs(
	ctx context.Context,
	validators map[ids.NodeID]*validators.GetValidatorOutput,
	startHeight uint64,
	endHeight uint64,
	subnetID ids.ID,
) error {
	return nil
}

func (tvs *TestValidatorState) ApplyValidatorPublicKeyDiffs(
	ctx context.Context,
	validators map[ids.NodeID]*validators.GetValidatorOutput,
	startHeight uint64,
	endHeight uint64,
	subnetID ids.ID,
) error {
	return nil
}
