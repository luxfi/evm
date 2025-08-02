// (c) 2023, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package warp

import (
	"context"
	"testing"
	"github.com/luxfi/evm/v2/iface"
	"github.com/luxfi/evm/v2/utils/set"
	"github.com/luxfi/geth/common"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// This test copies the test coverage from https://github.com/luxfi/node/blob/v1.10.0/vms/platformvm/warp/signature_test.go#L137.
// These tests are only expected to fail if there is a breaking change in Lux that unexpectedly changes behavior.
type signatureVerificationTest struct {
	name         string
	stateF       func(*gomock.Controller) iface.State
	quorumNum    uint64
	quorumDen    uint64
	msgF         func(*require.Assertions) *iface.WarpSignedMessage
	verifyErr    error
	canonicalErr error
}

// createTestState creates a test state adapter
func createTestState(
	getSubnetID func(context.Context, iface.ID) (iface.ID, error),
	getValidatorSet func(context.Context, uint64, iface.ID) (map[iface.NodeID]*iface.GetValidatorOutput, error),
) iface.State {
	return &stateAdapter{
		GetSubnetIDF:     getSubnetID,
		GetValidatorSetF: getValidatorSet,
	}
}

// This test copies the test coverage from https://github.com/luxfi/node/blob/0117ab96/vms/platformvm/warp/signature_test.go#L137.
// These tests are only expected to fail if there is a breaking change in Lux that unexpectedly changes behavior.
func TestSignatureVerification(t *testing.T) {
	tests := []signatureVerificationTest{
		{
			name: "can't get subnetID",
			stateF: func(ctrl *gomock.Controller) iface.State {
				return createTestState(
					func(ctx context.Context, chainID iface.ID) (iface.ID, error) {
						return sourceSubnetID, errTest
					},
					nil,
				)
			},
			quorumNum: 1,
			quorumDen: 2,
			msgF: func(require *require.Assertions) *iface.WarpSignedMessage {
				unsignedMsg, err := iface.NewUnsignedMessage(
					networkID,
					sourceChainID,
					addressedPayloadBytes,
				)
				require.NoError(err)

				msg, err := iface.NewMessage(
					unsignedMsg,
					&iface.BitSetSignature{},
				)
				require.NoError(err)
				return msg
			},
			canonicalErr: errTest,
		},
		{
			name: "can't get validator set",
			stateF: func(ctrl *gomock.Controller) iface.State {
				return createTestState(
					func(ctx context.Context, chainID iface.ID) (iface.ID, error) {
						return sourceSubnetID, nil
					},
					func(ctx context.Context, height uint64, subnetID iface.ID) (map[iface.NodeID]*iface.GetValidatorOutput, error) {
						return nil, errTest
					},
				)
			},
			quorumNum: 1,
			quorumDen: 2,
			msgF: func(require *require.Assertions) *iface.WarpSignedMessage {
				unsignedMsg, err := iface.NewUnsignedMessage(
					networkID,
					sourceChainID,
					addressedPayloadBytes,
				)
				require.NoError(err)

				msg, err := iface.NewMessage(
					unsignedMsg,
					&iface.BitSetSignature{},
				)
				require.NoError(err)
				return msg
			},
			canonicalErr: errTest,
		},
		{
			name: "weight overflow",
			stateF: func(ctrl *gomock.Controller) iface.State {
				return createTestState(
					func(ctx context.Context, chainID iface.ID) (iface.ID, error) {
						return sourceSubnetID, nil
					},
					func(ctx context.Context, height uint64, subnetID iface.ID) (map[iface.NodeID]*iface.GetValidatorOutput, error) {
						return map[iface.NodeID]*iface.GetValidatorOutput{
							testVdrs[0].nodeID: {
								NodeID:    testVdrs[0].nodeID,
								PublicKey: testVdrs[0].vdr.PublicKeyBytes,
								Weight:    iface.MaxUint64,
							},
							testVdrs[1].nodeID: {
								NodeID:    testVdrs[1].nodeID,
								PublicKey: testVdrs[1].vdr.PublicKeyBytes,
								Weight:    iface.MaxUint64,
							},
						}, nil
					},
				)
			},
			quorumNum: 1,
			quorumDen: 2,
			msgF: func(require *require.Assertions) *iface.WarpSignedMessage {
				unsignedMsg, err := iface.NewUnsignedMessage(
					networkID,
					sourceChainID,
					addressedPayloadBytes,
				)
				require.NoError(err)

				msg, err := iface.NewMessage(
					unsignedMsg,
					&iface.BitSetSignature{
						Signers: make([]byte, 8),
					},
				)
				require.NoError(err)
				return msg
			},
			verifyErr:    iface.ErrWeightOverflow,
			canonicalErr: iface.ErrWeightOverflow,
		},
		{
			name: "invalid bit set index",
			stateF: func(ctrl *gomock.Controller) iface.State {
				return createTestState(
					func(ctx context.Context, chainID iface.ID) (iface.ID, error) {
						return sourceSubnetID, nil
					},
					func(ctx context.Context, height uint64, subnetID iface.ID) (map[iface.NodeID]*iface.GetValidatorOutput, error) {
						return vdrs, nil
					},
				)
			},
			quorumNum: 1,
			quorumDen: 2,
			msgF: func(require *require.Assertions) *iface.WarpSignedMessage {
				unsignedMsg, err := iface.NewUnsignedMessage(
					networkID,
					sourceChainID,
					addressedPayloadBytes,
				)
				require.NoError(err)

				msg, err := iface.NewMessage(
					unsignedMsg,
					&iface.BitSetSignature{
						Signers:   make([]byte, 1),
						Signature: [iface.SignatureLen]byte{},
					},
				)
				require.NoError(err)
				return msg
			},
			verifyErr:    iface.ErrInvalidBitSet,
			canonicalErr: iface.ErrInvalidBitSet,
		},
		{
			name: "unknown index",
			stateF: func(ctrl *gomock.Controller) iface.State {
				return createTestState(
					func(ctx context.Context, chainID iface.ID) (iface.ID, error) {
						return sourceSubnetID, nil
					},
					func(ctx context.Context, height uint64, subnetID iface.ID) (map[iface.NodeID]*iface.GetValidatorOutput, error) {
						return vdrs, nil
					},
				)
			},
			quorumNum: 1,
			quorumDen: 2,
			msgF: func(require *require.Assertions) *iface.WarpSignedMessage {
				unsignedMsg, err := iface.NewUnsignedMessage(
					networkID,
					sourceChainID,
					addressedPayloadBytes,
				)
				require.NoError(err)

				signers := set.NewBits()
				signers.Add(3) // vdr oob

				msg, err := iface.NewMessage(
					unsignedMsg,
					&iface.BitSetSignature{
						Signers:   signers.Bytes(),
						Signature: [iface.SignatureLen]byte{},
					},
				)
				require.NoError(err)
				return msg
			},
			verifyErr:    iface.ErrUnknownValidator,
			canonicalErr: iface.ErrUnknownValidator,
		},
		{
			name: "insufficient weight",
			stateF: func(ctrl *gomock.Controller) iface.State {
				return createTestState(
					func(ctx context.Context, chainID iface.ID) (iface.ID, error) {
						return sourceSubnetID, nil
					},
					func(ctx context.Context, height uint64, subnetID iface.ID) (map[iface.NodeID]*iface.GetValidatorOutput, error) {
						return vdrs, nil
					},
				)
			},
			quorumNum: 1,
			quorumDen: 1,
			msgF: func(require *require.Assertions) *iface.WarpSignedMessage {
				unsignedMsg, err := iface.NewUnsignedMessage(
					networkID,
					sourceChainID,
					addressedPayloadBytes,
				)
				require.NoError(err)

				// [signers] has weight from [vdr[0], vdr[1]],
				// which is 6, which is less than 9
				signers := set.NewBits()
				signers.Add(0)
				signers.Add(1)

				unsignedBytes := unsignedMsg.Bytes()
				vdr0Sig, err := testVdrs[0].sk.Sign(unsignedBytes)
				require.NoError(err)
				vdr1Sig, err := testVdrs[1].sk.Sign(unsignedBytes)
				require.NoError(err)
				aggSig, err := iface.AggregateSignatures([]*iface.BLSSignature{vdr0Sig, vdr1Sig})
				require.NoError(err)
				aggSigBytes := [iface.SignatureLen]byte{}
				copy(aggSigBytes[:], iface.SignatureToBytes(aggSig))

				msg, err := iface.NewMessage(
					unsignedMsg,
					&iface.BitSetSignature{
						Signers:   signers.Bytes(),
						Signature: aggSigBytes,
					},
				)
				require.NoError(err)
				return msg
			},
			verifyErr:    iface.ErrInsufficientWeight,
			canonicalErr: iface.ErrInsufficientWeight,
		},
		{
			name: "can't parse sig",
			stateF: func(ctrl *gomock.Controller) iface.State {
				return createTestState(
					func(ctx context.Context, chainID iface.ID) (iface.ID, error) {
						return sourceSubnetID, nil
					},
					func(ctx context.Context, height uint64, subnetID iface.ID) (map[iface.NodeID]*iface.GetValidatorOutput, error) {
						return vdrs, nil
					},
				)
			},
			quorumNum: 1,
			quorumDen: 2,
			msgF: func(require *require.Assertions) *iface.WarpSignedMessage {
				unsignedMsg, err := iface.NewUnsignedMessage(
					networkID,
					sourceChainID,
					addressedPayloadBytes,
				)
				require.NoError(err)

				signers := set.NewBits()
				signers.Add(0)
				signers.Add(1)

				msg, err := iface.NewMessage(
					unsignedMsg,
					&iface.BitSetSignature{
						Signers:   signers.Bytes(),
						Signature: [iface.SignatureLen]byte{},
					},
				)
				require.NoError(err)
				return msg
			},
			verifyErr:    iface.ErrParseSignature,
			canonicalErr: iface.ErrParseSignature,
		},
		{
			name: "no validators",
			stateF: func(ctrl *gomock.Controller) iface.State {
				return createTestState(
					func(ctx context.Context, chainID iface.ID) (iface.ID, error) {
						return sourceSubnetID, nil
					},
					func(ctx context.Context, height uint64, subnetID iface.ID) (map[iface.NodeID]*iface.GetValidatorOutput, error) {
						return nil, nil
					},
				)
			},
			quorumNum: 1,
			quorumDen: 2,
			msgF: func(require *require.Assertions) *iface.WarpSignedMessage {
				unsignedMsg, err := iface.NewUnsignedMessage(
					networkID,
					sourceChainID,
					addressedPayloadBytes,
				)
				require.NoError(err)

				unsignedBytes := unsignedMsg.Bytes()
				vdr0Sig, err := testVdrs[0].sk.Sign(unsignedBytes)
				require.NoError(err)
				aggSigBytes := [iface.SignatureLen]byte{}
				copy(aggSigBytes[:], iface.SignatureToBytes(vdr0Sig))

				msg, err := iface.NewMessage(
					unsignedMsg,
					&iface.BitSetSignature{
						Signers:   nil,
						Signature: aggSigBytes,
					},
				)
				require.NoError(err)
				return msg
			},
			verifyErr: iface.ErrNoPublicKeys,
		},
		{
			name: "invalid signature (substitute)",
			stateF: func(ctrl *gomock.Controller) iface.State {
				return createTestState(
					func(ctx context.Context, chainID iface.ID) (iface.ID, error) {
						return sourceSubnetID, nil
					},
					func(ctx context.Context, height uint64, subnetID iface.ID) (map[iface.NodeID]*iface.GetValidatorOutput, error) {
						return vdrs, nil
					},
				)
			},
			quorumNum: 3,
			quorumDen: 5,
			msgF: func(require *require.Assertions) *iface.WarpSignedMessage {
				unsignedMsg, err := iface.NewUnsignedMessage(
					networkID,
					sourceChainID,
					addressedPayloadBytes,
				)
				require.NoError(err)

				signers := set.NewBits()
				signers.Add(0)
				signers.Add(1)

				unsignedBytes := unsignedMsg.Bytes()
				vdr0Sig, err := testVdrs[0].sk.Sign(unsignedBytes)
				require.NoError(err)
				// Give sig from vdr[2] even though the bit vector says it
				// should be from vdr[1]
				vdr2Sig, err := testVdrs[2].sk.Sign(unsignedBytes)
				require.NoError(err)
				aggSig, err := iface.AggregateSignatures([]*iface.BLSSignature{vdr0Sig, vdr2Sig})
				require.NoError(err)
				aggSigBytes := [iface.SignatureLen]byte{}
				copy(aggSigBytes[:], iface.SignatureToBytes(aggSig))

				msg, err := iface.NewMessage(
					unsignedMsg,
					&iface.BitSetSignature{
						Signers:   signers.Bytes(),
						Signature: aggSigBytes,
					},
				)
				require.NoError(err)
				return msg
			},
			verifyErr:    iface.ErrInvalidSignature,
			canonicalErr: iface.ErrInvalidSignature,
		},
		{
			name: "invalid signature (missing one)",
			stateF: func(ctrl *gomock.Controller) iface.State {
				return createTestState(
					func(ctx context.Context, chainID iface.ID) (iface.ID, error) {
						return sourceSubnetID, nil
					},
					func(ctx context.Context, height uint64, subnetID iface.ID) (map[iface.NodeID]*iface.GetValidatorOutput, error) {
						return vdrs, nil
					},
				)
			},
			quorumNum: 3,
			quorumDen: 5,
			msgF: func(require *require.Assertions) *iface.WarpSignedMessage {
				unsignedMsg, err := iface.NewUnsignedMessage(
					networkID,
					sourceChainID,
					addressedPayloadBytes,
				)
				require.NoError(err)

				signers := set.NewBits()
				signers.Add(0)
				signers.Add(1)

				unsignedBytes := unsignedMsg.Bytes()
				vdr0Sig, err := testVdrs[0].sk.Sign(unsignedBytes)
				require.NoError(err)
				// Don't give the sig from vdr[1]
				aggSigBytes := [iface.SignatureLen]byte{}
				copy(aggSigBytes[:], iface.SignatureToBytes(vdr0Sig))

				msg, err := iface.NewMessage(
					unsignedMsg,
					&iface.BitSetSignature{
						Signers:   signers.Bytes(),
						Signature: aggSigBytes,
					},
				)
				require.NoError(err)
				return msg
			},
			verifyErr:    iface.ErrInvalidSignature,
			canonicalErr: iface.ErrInvalidSignature,
		},
		{
			name: "invalid signature (extra one)",
			stateF: func(ctrl *gomock.Controller) iface.State {
				return createTestState(
					func(ctx context.Context, chainID iface.ID) (iface.ID, error) {
						return sourceSubnetID, nil
					},
					func(ctx context.Context, height uint64, subnetID iface.ID) (map[iface.NodeID]*iface.GetValidatorOutput, error) {
						return vdrs, nil
					},
				)
			},
			quorumNum: 3,
			quorumDen: 5,
			msgF: func(require *require.Assertions) *iface.WarpSignedMessage {
				unsignedMsg, err := iface.NewUnsignedMessage(
					networkID,
					sourceChainID,
					addressedPayloadBytes,
				)
				require.NoError(err)

				signers := set.NewBits()
				signers.Add(0)
				signers.Add(1)

				unsignedBytes := unsignedMsg.Bytes()
				vdr0Sig, err := testVdrs[0].sk.Sign(unsignedBytes)
				require.NoError(err)
				vdr1Sig, err := testVdrs[1].sk.Sign(unsignedBytes)
				require.NoError(err)
				// Give sig from vdr[2] even though the bit vector doesn't have
				// it
				vdr2Sig, err := testVdrs[2].sk.Sign(unsignedBytes)
				require.NoError(err)
				aggSig, err := iface.AggregateSignatures([]*iface.BLSSignature{vdr0Sig, vdr1Sig, vdr2Sig})
				require.NoError(err)
				aggSigBytes := [iface.SignatureLen]byte{}
				copy(aggSigBytes[:], iface.SignatureToBytes(aggSig))

				msg, err := iface.NewMessage(
					unsignedMsg,
					&iface.BitSetSignature{
						Signers:   signers.Bytes(),
						Signature: aggSigBytes,
					},
				)
				require.NoError(err)
				return msg
			},
			verifyErr:    iface.ErrInvalidSignature,
			canonicalErr: iface.ErrInvalidSignature,
		},
		{
			name: "valid signature",
			stateF: func(ctrl *gomock.Controller) iface.State {
				return createTestState(
					func(ctx context.Context, chainID iface.ID) (iface.ID, error) {
						return sourceSubnetID, nil
					},
					func(ctx context.Context, height uint64, subnetID iface.ID) (map[iface.NodeID]*iface.GetValidatorOutput, error) {
						return vdrs, nil
					},
				)
			},
			quorumNum: 1,
			quorumDen: 2,
			msgF: func(require *require.Assertions) *iface.WarpSignedMessage {
				unsignedMsg, err := iface.NewUnsignedMessage(
					networkID,
					sourceChainID,
					addressedPayloadBytes,
				)
				require.NoError(err)

				// [signers] has weight from [vdr[1], vdr[2]],
				// which is 6, which is greater than 4.5
				signers := set.NewBits()
				signers.Add(1)
				signers.Add(2)

				unsignedBytes := unsignedMsg.Bytes()
				vdr1Sig, err := testVdrs[1].sk.Sign(unsignedBytes)
				require.NoError(err)
				vdr2Sig, err := testVdrs[2].sk.Sign(unsignedBytes)
				require.NoError(err)
				aggSig, err := iface.AggregateSignatures([]*iface.BLSSignature{vdr1Sig, vdr2Sig})
				require.NoError(err)
				aggSigBytes := [iface.SignatureLen]byte{}
				copy(aggSigBytes[:], iface.SignatureToBytes(aggSig))

				msg, err := iface.NewMessage(
					unsignedMsg,
					&iface.BitSetSignature{
						Signers:   signers.Bytes(),
						Signature: aggSigBytes,
					},
				)
				require.NoError(err)
				return msg
			},
			verifyErr: nil,
		},
		{
			name: "valid signature (boundary)",
			stateF: func(ctrl *gomock.Controller) iface.State {
				return createTestState(
					func(ctx context.Context, chainID iface.ID) (iface.ID, error) {
						return sourceSubnetID, nil
					},
					func(ctx context.Context, height uint64, subnetID iface.ID) (map[iface.NodeID]*iface.GetValidatorOutput, error) {
						return vdrs, nil
					},
				)
			},
			quorumNum: 2,
			quorumDen: 3,
			msgF: func(require *require.Assertions) *iface.WarpSignedMessage {
				unsignedMsg, err := iface.NewUnsignedMessage(
					networkID,
					sourceChainID,
					addressedPayloadBytes,
				)
				require.NoError(err)

				// [signers] has weight from [vdr[1], vdr[2]],
				// which is 6, which meets the minimum 6
				signers := set.NewBits()
				signers.Add(1)
				signers.Add(2)

				unsignedBytes := unsignedMsg.Bytes()
				vdr1Sig, err := testVdrs[1].sk.Sign(unsignedBytes)
				require.NoError(err)
				vdr2Sig, err := testVdrs[2].sk.Sign(unsignedBytes)
				require.NoError(err)
				aggSig, err := iface.AggregateSignatures([]*iface.BLSSignature{vdr1Sig, vdr2Sig})
				require.NoError(err)
				aggSigBytes := [iface.SignatureLen]byte{}
				copy(aggSigBytes[:], iface.SignatureToBytes(aggSig))

				msg, err := iface.NewMessage(
					unsignedMsg,
					&iface.BitSetSignature{
						Signers:   signers.Bytes(),
						Signature: aggSigBytes,
					},
				)
				require.NoError(err)
				return msg
			},
			verifyErr: nil,
		},
		{
			name: "valid signature (missing key)",
			stateF: func(ctrl *gomock.Controller) iface.State {
				return createTestState(
					func(ctx context.Context, chainID iface.ID) (iface.ID, error) {
						return sourceSubnetID, nil
					},
					func(ctx context.Context, height uint64, subnetID iface.ID) (map[iface.NodeID]*iface.GetValidatorOutput, error) {
						return map[iface.NodeID]*iface.GetValidatorOutput{
							testVdrs[0].nodeID: {
								NodeID:    testVdrs[0].nodeID,
								PublicKey: nil,
								Weight:    testVdrs[0].vdr.Weight,
							},
							testVdrs[1].nodeID: {
								NodeID:    testVdrs[1].nodeID,
								PublicKey: testVdrs[1].vdr.PublicKeyBytes,
								Weight:    testVdrs[1].vdr.Weight,
							},
							testVdrs[2].nodeID: {
								NodeID:    testVdrs[2].nodeID,
								PublicKey: testVdrs[2].vdr.PublicKeyBytes,
								Weight:    testVdrs[2].vdr.Weight,
							},
						}, nil
					},
				)
			},
			quorumNum: 1,
			quorumDen: 3,
			msgF: func(require *require.Assertions) *iface.WarpSignedMessage {
				unsignedMsg, err := iface.NewUnsignedMessage(
					networkID,
					sourceChainID,
					addressedPayloadBytes,
				)
				require.NoError(err)

				// [signers] has weight from [vdr2, vdr3],
				// which is 6, which is greater than 3
				signers := set.NewBits()
				// Note: the bits are shifted because vdr[0]'s key was zeroed
				signers.Add(0) // vdr[1]
				signers.Add(1) // vdr[2]

				unsignedBytes := unsignedMsg.Bytes()
				vdr1Sig, err := testVdrs[1].sk.Sign(unsignedBytes)
				require.NoError(err)
				vdr2Sig, err := testVdrs[2].sk.Sign(unsignedBytes)
				require.NoError(err)
				aggSig, err := iface.AggregateSignatures([]*iface.BLSSignature{vdr1Sig, vdr2Sig})
				require.NoError(err)
				aggSigBytes := [iface.SignatureLen]byte{}
				copy(aggSigBytes[:], iface.SignatureToBytes(aggSig))

				msg, err := iface.NewMessage(
					unsignedMsg,
					&iface.BitSetSignature{
						Signers:   signers.Bytes(),
						Signature: aggSigBytes,
					},
				)
				require.NoError(err)
				return msg
			},
			verifyErr: nil,
		},
		{
			name: "valid signature (duplicate key)",
			stateF: func(ctrl *gomock.Controller) iface.State {
				return createTestState(
					func(ctx context.Context, chainID iface.ID) (iface.ID, error) {
						return sourceSubnetID, nil
					},
					func(ctx context.Context, height uint64, subnetID iface.ID) (map[iface.NodeID]*iface.GetValidatorOutput, error) {
						return map[iface.NodeID]*iface.GetValidatorOutput{
							testVdrs[0].nodeID: {
								NodeID:    testVdrs[0].nodeID,
								PublicKey: nil,
								Weight:    testVdrs[0].vdr.Weight,
							},
							testVdrs[1].nodeID: {
								NodeID:    testVdrs[1].nodeID,
								PublicKey: testVdrs[2].vdr.PublicKeyBytes,
								Weight:    testVdrs[1].vdr.Weight,
							},
							testVdrs[2].nodeID: {
								NodeID:    testVdrs[2].nodeID,
								PublicKey: testVdrs[2].vdr.PublicKeyBytes,
								Weight:    testVdrs[2].vdr.Weight,
							},
						}, nil
					},
				)
			},
			quorumNum: 2,
			quorumDen: 3,
			msgF: func(require *require.Assertions) *iface.WarpSignedMessage {
				unsignedMsg, err := iface.NewUnsignedMessage(
					networkID,
					sourceChainID,
					addressedPayloadBytes,
				)
				require.NoError(err)

				// [signers] has weight from [vdr2, vdr3],
				// which is 6, which meets the minimum 6
				signers := set.NewBits()
				// Note: the bits are shifted because vdr[0]'s key was zeroed
				// Note: vdr[1] and vdr[2] were combined because of a shared pk
				signers.Add(0) // vdr[1] + vdr[2]

				unsignedBytes := unsignedMsg.Bytes()
				// Because vdr[1] and vdr[2] share a key, only one of them sign.
				vdr2Sig, err := testVdrs[2].sk.Sign(unsignedBytes)
				require.NoError(err)
				aggSigBytes := [iface.SignatureLen]byte{}
				copy(aggSigBytes[:], iface.SignatureToBytes(vdr2Sig))

				msg, err := iface.NewMessage(
					unsignedMsg,
					&iface.BitSetSignature{
						Signers:   signers.Bytes(),
						Signature: aggSigBytes,
					},
				)
				require.NoError(err)
				return msg
			},
			verifyErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			msg := tt.msgF(require)
			pChainState := tt.stateF(ctrl)

			// Convert State to ValidatorState
			validatorState := &validatorStateAdapter{
				GetSubnetIDF: func(ctx context.Context, chainID common.Hash) (common.Hash, error) {
					subnetID, err := pChainState.GetSubnetID(ctx, iface.ID(chainID))
					return common.Hash(subnetID), err
				},
				GetValidatorSetF: func(ctx context.Context, height uint64, subnetID common.Hash) (map[common.Hash]*iface.ValidatorOutput, error) {
					validators, err := pChainState.GetValidatorSet(ctx, height, iface.ID(subnetID))
					if err != nil {
						return nil, err
					}
					result := make(map[common.Hash]*iface.ValidatorOutput)
					for nodeID, v := range validators {
						result[common.Hash(nodeID)] = ConvertGetValidatorOutputToValidatorOutput(v)
					}
					return result, nil
				},
			}
			
			validatorSet, err := iface.GetCanonicalValidatorSetFromChainID(
				context.Background(),
				validatorState,
				pChainHeight,
				msg.UnsignedMessage.SourceChainID,
			)
			require.ErrorIs(err, tt.canonicalErr)
			if err != nil {
				return
			}
			err = msg.Signature.Verify(
				&msg.UnsignedMessage,
				networkID,
				&validatorSet,
				tt.quorumNum,
				tt.quorumDen,
			)
			require.ErrorIs(err, tt.verifyErr)
		})
	}
}
