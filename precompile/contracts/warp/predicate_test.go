// (c) 2023, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package warp

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"github.com/luxfi/evm/v2/commontype"
	"github.com/luxfi/evm/v2/iface"
	"github.com/luxfi/evm/v2/localsigner"
	"github.com/luxfi/evm/v2/precompile/precompileconfig"
	"github.com/luxfi/evm/v2/precompile/testutils"
	"github.com/luxfi/evm/v2/predicate"
	"github.com/luxfi/evm/v2/utils"
	"github.com/luxfi/evm/v2/utils/set"
	"github.com/luxfi/geth/common"
	agoUtils "github.com/luxfi/node/utils"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const pChainHeight uint64 = 1337

var (
	_ agoUtils.Sortable[*testValidator] = (*testValidator)(nil)

	errTest        = errors.New("non-nil error")
	networkID      = uint32(54321)
	sourceChainID  = GenerateTestID()
	sourceSubnetID = GenerateTestID()
	
	// PrimaryNetworkID is the ID of the primary network
	PrimaryNetworkID = iface.ID{}

	// valid unsigned warp message used throughout testing
	unsignedMsg *iface.UnsignedMessage
	// valid addressed payload
	addressedPayload      *iface.AddressedCall
	addressedPayloadBytes []byte
	// blsSignatures of [unsignedMsg] from each of [testVdrs]
	blsSignatures []*iface.BLSSignature

	numTestVdrs = 10_000
	testVdrs    []*testValidator
	vdrs        map[iface.NodeID]*iface.GetValidatorOutput

	predicateTests = make(map[string]testutils.PredicateTest)
)

func init() {
	testVdrs = make([]*testValidator, 0, numTestVdrs)
	for i := 0; i < numTestVdrs; i++ {
		testVdrs = append(testVdrs, newTestValidator())
	}
	agoUtils.Sort(testVdrs)

	vdrs = map[iface.NodeID]*iface.GetValidatorOutput{
		testVdrs[0].nodeID: {
			NodeID:    testVdrs[0].nodeID,
			PublicKey: testVdrs[0].vdr.PublicKeyBytes,
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
	}

	var err error
	addr := GenerateTestShortID()
	addressedPayload, err = iface.NewAddressedCall(
		addr[:],
		[]byte{1, 2, 3},
	)
	if err != nil {
		panic(err)
	}
	addressedPayloadBytes = addressedPayload.Bytes()
	unsignedMsg, err = iface.NewUnsignedMessage(networkID, sourceChainID, addressedPayload.Bytes())
	if err != nil {
		panic(err)
	}

	for _, testVdr := range testVdrs {
		blsSignature, err := testVdr.sk.Sign(unsignedMsg.Bytes())
		if err != nil {
			panic(err)
		}
		blsSignatures = append(blsSignatures, blsSignature)
	}

	initWarpPredicateTests()
}

type testValidator struct {
	nodeID iface.NodeID
	sk     iface.Signer
	vdr    *ValidatorImpl
}

func (v *testValidator) Compare(o *testValidator) int {
	return v.vdr.Compare(o.vdr)
}

func newTestValidator() *testValidator {
	sk, err := localsigner.New()
	if err != nil {
		panic(err)
	}

	nodeID := GenerateTestNodeID()
	signer := &signerAdapter{sk: sk}
	pk := signer.PublicKey()
	return &testValidator{
		nodeID: nodeID,
		sk:     signer,
		vdr: &ValidatorImpl{
			PublicKey:      pk,
			PublicKeyBytes: pk.UncompressedBytes(),
			Weight:         3,
			NodeIDs:        []iface.NodeID{nodeID},
		},
	}
}

type signatureTest struct {
	name      string
	stateF    func(*gomock.Controller) iface.State
	quorumNum uint64
	quorumDen uint64
	msgF      func(*require.Assertions) *iface.WarpSignedMessage
	err       error
}

// createWarpMessage constructs a signed warp message using the global variable [unsignedMsg]
// and the first [numKeys] signatures from [blsSignatures]
func createWarpMessage(numKeys int) *iface.WarpSignedMessage {
	aggregateSignature, err := iface.AggregateSignatures(blsSignatures[0:numKeys])
	if err != nil {
		panic(err)
	}
	bitSet := set.NewBits()
	for i := 0; i < numKeys; i++ {
		bitSet.Add(i)
	}
	warpSignature := &iface.BitSetSignature{
		Signers: bitSet.Bytes(),
	}
	copy(warpSignature.Signature[:], iface.SignatureToBytes(aggregateSignature))
	warpMsg, err := iface.NewMessage(unsignedMsg, warpSignature)
	if err != nil {
		panic(err)
	}
	return warpMsg
}

// createPredicate constructs a warp message using createWarpMessage with numKeys signers
// and packs it into predicate encoding.
func createPredicate(numKeys int) []byte {
	warpMsg := createWarpMessage(numKeys)
	predicateBytes := predicate.PackPredicate(warpMsg.Bytes())
	return predicateBytes
}

// validatorRange specifies a range of validators to include from [start, end), a staking weight
// to specify for each validator in that range, and whether or not to include the public key.
type validatorRange struct {
	start     int
	end       int
	weight    uint64
	publicKey bool
}

// createConsensusCtx creates a consensus.Context instance with a validator state specified by the given validatorRanges
func createConsensusCtx(validatorRanges []validatorRange) *precompileconfig.PredicateContext {
	getValidatorsOutput := make(map[iface.NodeID]*iface.GetValidatorOutput)

	for _, validatorRange := range validatorRanges {
		for i := validatorRange.start; i < validatorRange.end; i++ {
			validatorOutput := &iface.GetValidatorOutput{
				NodeID: testVdrs[i].nodeID,
				Weight: validatorRange.weight,
			}
			if validatorRange.publicKey {
				validatorOutput.PublicKey = testVdrs[i].vdr.PublicKeyBytes
			}
			getValidatorsOutput[testVdrs[i].nodeID] = validatorOutput
		}
	}

	chainCtx := utils.TestChainContext()
	chainCtx.NetworkID = networkID
	chainCtx.ValidatorState = &validatorStateAdapter{
		GetSubnetIDF: func(ctx context.Context, chainID common.Hash) (common.Hash, error) {
			return common.Hash(sourceSubnetID), nil
		},
		GetValidatorSetF: func(ctx context.Context, height uint64, subnetID common.Hash) (map[common.Hash]*iface.ValidatorOutput, error) {
			// Convert map[iface.NodeID]*iface.GetValidatorOutput to map[common.Hash]*iface.ValidatorOutput
			result := make(map[common.Hash]*iface.ValidatorOutput)
			for nodeID, v := range getValidatorsOutput {
				result[common.Hash(nodeID)] = ConvertGetValidatorOutputToValidatorOutput(v)
			}
			return result, nil
		},
	}
	return &precompileconfig.PredicateContext{
		ConsensusCtx: chainCtx,
		ProposerVMBlockCtx: &commontype.BlockContext{
			PChainHeight: 1,
		},
	}
}

func createValidPredicateTest(predicateCtx *precompileconfig.PredicateContext, numKeys uint64, predicateBytes []byte) testutils.PredicateTest {
	return testutils.PredicateTest{
		Config:           NewDefaultConfig(utils.NewUint64(0)),
		PredicateContext: predicateCtx,
		PredicateBytes:   predicateBytes,
		Gas:              GasCostPerSignatureVerification + uint64(len(predicateBytes))*GasCostPerWarpMessageBytes + numKeys*GasCostPerWarpSigner,
		GasErr:           nil,
		ExpectedErr:      nil,
	}
}

func TestWarpMessageFromPrimaryNetwork(t *testing.T) {
	for _, requirePrimaryNetworkSigners := range []bool{true, false} {
		testWarpMessageFromPrimaryNetwork(t, requirePrimaryNetworkSigners)
	}
}

func testWarpMessageFromPrimaryNetwork(t *testing.T, requirePrimaryNetworkSigners bool) {
	require := require.New(t)
	numKeys := 10
	cChainID := GenerateTestID()
	addressedCall, err := iface.NewAddressedCall(agoUtils.RandomBytes(20), agoUtils.RandomBytes(100))
	require.NoError(err)
	unsignedMsg, err := iface.NewUnsignedMessage(networkID, cChainID, addressedCall.Bytes())
	require.NoError(err)

	getValidatorsOutput := make(map[iface.NodeID]*iface.GetValidatorOutput)
	blsSignatures := make([]*iface.BLSSignature, 0, numKeys)
	for i := 0; i < numKeys; i++ {
		sig, err := testVdrs[i].sk.Sign(unsignedMsg.Bytes())
		require.NoError(err)

		validatorOutput := &iface.GetValidatorOutput{
			NodeID:    testVdrs[i].nodeID,
			Weight:    20,
			PublicKey: testVdrs[i].vdr.PublicKeyBytes,
		}
		getValidatorsOutput[testVdrs[i].nodeID] = validatorOutput
		blsSignatures = append(blsSignatures, sig)
	}
	aggregateSignature, err := iface.AggregateSignatures(blsSignatures)
	require.NoError(err)
	bitSet := set.NewBits()
	for i := 0; i < numKeys; i++ {
		bitSet.Add(i)
	}
	warpSignature := &iface.BitSetSignature{
		Signers: bitSet.Bytes(),
	}
	copy(warpSignature.Signature[:], iface.SignatureToBytes(aggregateSignature))
	warpMsg, err := iface.NewMessage(unsignedMsg, warpSignature)
	require.NoError(err)

	predicateBytes := predicate.PackPredicate(warpMsg.Bytes())

	chainCtx := utils.TestChainContext()
	chainCtx.SubnetID = iface.SubnetID(GenerateTestID())
	chainCtx.ChainID = iface.ChainID(GenerateTestID())
	chainCtx.NetworkID = networkID
	
	subnetID := chainCtx.SubnetID
	chainCtx.ValidatorState = &validatorStateAdapter{
		GetSubnetIDF: func(ctx context.Context, chainID common.Hash) (common.Hash, error) {
			require.Equal(chainID, common.Hash(cChainID))
			return common.Hash(PrimaryNetworkID), nil // Return Primary Network SubnetID
		},
		GetValidatorSetF: func(ctx context.Context, height uint64, testSubnetID common.Hash) (map[common.Hash]*iface.ValidatorOutput, error) {
			expectedSubnetID := common.Hash(subnetID)
			if requirePrimaryNetworkSigners {
				expectedSubnetID = common.Hash(PrimaryNetworkID)
			}
			require.Equal(expectedSubnetID, testSubnetID)
			// Convert the map to the expected type
			result := make(map[common.Hash]*iface.ValidatorOutput)
			for nodeID, v := range getValidatorsOutput {
				result[common.Hash(nodeID)] = ConvertGetValidatorOutputToValidatorOutput(v)
			}
			return result, nil
		},
	}

	test := testutils.PredicateTest{
		Config: NewConfig(utils.NewUint64(0), 0, requirePrimaryNetworkSigners),
		PredicateContext: &precompileconfig.PredicateContext{
			ConsensusCtx: chainCtx,
			ProposerVMBlockCtx: &commontype.BlockContext{
				PChainHeight: 1,
			},
		},
		PredicateBytes: predicateBytes,
		Gas:            GasCostPerSignatureVerification + uint64(len(predicateBytes))*GasCostPerWarpMessageBytes + uint64(numKeys)*GasCostPerWarpSigner,
		GasErr:         nil,
		ExpectedErr:    nil,
	}

	test.Run(t)
}

func TestInvalidPredicatePacking(t *testing.T) {
	numKeys := 1
	predicateCtx := createConsensusCtx([]validatorRange{
		{
			start:     0,
			end:       numKeys,
			weight:    20,
			publicKey: true,
		},
	})
	predicateBytes := createPredicate(numKeys)
	predicateBytes = append(predicateBytes, byte(0x01)) // Invalidate the predicate byte packing

	test := testutils.PredicateTest{
		Config:           NewDefaultConfig(utils.NewUint64(0)),
		PredicateContext: predicateCtx,
		PredicateBytes:   predicateBytes,
		Gas:              GasCostPerSignatureVerification + uint64(len(predicateBytes))*GasCostPerWarpMessageBytes + uint64(numKeys)*GasCostPerWarpSigner,
		GasErr:           errInvalidPredicateBytes,
	}

	test.Run(t)
}

func TestInvalidWarpMessage(t *testing.T) {
	numKeys := 1
	predicateCtx := createConsensusCtx([]validatorRange{
		{
			start:     0,
			end:       numKeys,
			weight:    20,
			publicKey: true,
		},
	})
	warpMsg := createWarpMessage(1)
	warpMsgBytes := warpMsg.Bytes()
	warpMsgBytes = append(warpMsgBytes, byte(0x01)) // Invalidate warp message packing
	predicateBytes := predicate.PackPredicate(warpMsgBytes)

	test := testutils.PredicateTest{
		Config:           NewDefaultConfig(utils.NewUint64(0)),
		PredicateContext: predicateCtx,
		PredicateBytes:   predicateBytes,
		Gas:              GasCostPerSignatureVerification + uint64(len(predicateBytes))*GasCostPerWarpMessageBytes + uint64(numKeys)*GasCostPerWarpSigner,
		GasErr:           errInvalidWarpMsg,
	}

	test.Run(t)
}

func TestInvalidAddressedPayload(t *testing.T) {
	numKeys := 1
	predicateCtx := createConsensusCtx([]validatorRange{
		{
			start:     0,
			end:       numKeys,
			weight:    20,
			publicKey: true,
		},
	})
	aggregateSignature, err := iface.AggregateSignatures(blsSignatures[0:numKeys])
	require.NoError(t, err)
	bitSet := set.NewBits()
	for i := 0; i < numKeys; i++ {
		bitSet.Add(i)
	}
	warpSignature := &iface.BitSetSignature{
		Signers: bitSet.Bytes(),
	}
	copy(warpSignature.Signature[:], iface.SignatureToBytes(aggregateSignature))
	// Create an unsigned message with an invalid addressed payload
	unsignedMsg, err := iface.NewUnsignedMessage(networkID, sourceChainID, []byte{1, 2, 3})
	require.NoError(t, err)
	warpMsg, err := iface.NewMessage(unsignedMsg, warpSignature)
	require.NoError(t, err)
	warpMsgBytes := warpMsg.Bytes()
	predicateBytes := predicate.PackPredicate(warpMsgBytes)

	test := testutils.PredicateTest{
		Config:           NewDefaultConfig(utils.NewUint64(0)),
		PredicateContext: predicateCtx,
		PredicateBytes:   predicateBytes,
		Gas:              GasCostPerSignatureVerification + uint64(len(predicateBytes))*GasCostPerWarpMessageBytes + uint64(numKeys)*GasCostPerWarpSigner,
		GasErr:           errInvalidWarpMsgPayload,
	}

	test.Run(t)
}

func TestInvalidBitSet(t *testing.T) {
	addressedCall, err := iface.NewAddressedCall(agoUtils.RandomBytes(20), agoUtils.RandomBytes(100))
	require.NoError(t, err)
	unsignedMsg, err := iface.NewUnsignedMessage(
		networkID,
		sourceChainID,
		addressedCall.Bytes(),
	)
	require.NoError(t, err)

	msg, err := iface.NewMessage(
		unsignedMsg,
		&iface.BitSetSignature{
			Signers:   make([]byte, 1),
			Signature: [96]byte{},
		},
	)
	require.NoError(t, err)

	numKeys := 1
	predicateCtx := createConsensusCtx([]validatorRange{
		{
			start:     0,
			end:       numKeys,
			weight:    20,
			publicKey: true,
		},
	})
	predicateBytes := predicate.PackPredicate(msg.Bytes())
	test := testutils.PredicateTest{
		Config:           NewDefaultConfig(utils.NewUint64(0)),
		PredicateContext: predicateCtx,
		PredicateBytes:   predicateBytes,
		Gas:              GasCostPerSignatureVerification + uint64(len(predicateBytes))*GasCostPerWarpMessageBytes + uint64(numKeys)*GasCostPerWarpSigner,
		GasErr:           errCannotGetNumSigners,
	}

	test.Run(t)
}

func TestWarpSignatureWeightsDefaultQuorumNumerator(t *testing.T) {
	predicateCtx := createConsensusCtx([]validatorRange{
		{
			start:     0,
			end:       100,
			weight:    20,
			publicKey: true,
		},
	})

	tests := make(map[string]testutils.PredicateTest)
	for _, numSigners := range []int{
		1,
		int(WarpDefaultQuorumNumerator) - 1,
		int(WarpDefaultQuorumNumerator),
		int(WarpDefaultQuorumNumerator) + 1,
		int(WarpQuorumDenominator) - 1,
		int(WarpQuorumDenominator),
		int(WarpQuorumDenominator) + 1,
	} {
		predicateBytes := createPredicate(numSigners)
		// The predicate is valid iff the number of signers is >= the required numerator and does not exceed the denominator.
		var expectedErr error
		if numSigners >= int(WarpDefaultQuorumNumerator) && numSigners <= int(WarpQuorumDenominator) {
			expectedErr = nil
		} else {
			expectedErr = errFailedVerification
		}

		tests[fmt.Sprintf("default quorum %d signature(s)", numSigners)] = testutils.PredicateTest{
			Config:           NewDefaultConfig(utils.NewUint64(0)),
			PredicateContext: predicateCtx,
			PredicateBytes:   predicateBytes,
			Gas:              GasCostPerSignatureVerification + uint64(len(predicateBytes))*GasCostPerWarpMessageBytes + uint64(numSigners)*GasCostPerWarpSigner,
			GasErr:           nil,
			ExpectedErr:      expectedErr,
		}
	}
	testutils.RunPredicateTests(t, tests)
}

// multiple messages all correct, multiple messages all incorrect, mixed bag
func TestWarpMultiplePredicates(t *testing.T) {
	predicateCtx := createConsensusCtx([]validatorRange{
		{
			start:     0,
			end:       100,
			weight:    20,
			publicKey: true,
		},
	})

	tests := make(map[string]testutils.PredicateTest)
	for _, validMessageIndices := range [][]bool{
		{},
		{true, false},
		{false, true},
		{false, false},
		{true, true},
	} {
		var (
			numSigners            = int(WarpQuorumDenominator)
			invalidPredicateBytes = createPredicate(1)
			validPredicateBytes   = createPredicate(numSigners)
		)

		for _, valid := range validMessageIndices {
			var (
				predicate   []byte
				expectedGas uint64
				expectedErr error
			)
			if valid {
				predicate = validPredicateBytes
				expectedGas = GasCostPerSignatureVerification + uint64(len(validPredicateBytes))*GasCostPerWarpMessageBytes + uint64(numSigners)*GasCostPerWarpSigner
				expectedErr = nil
			} else {
				expectedGas = GasCostPerSignatureVerification + uint64(len(invalidPredicateBytes))*GasCostPerWarpMessageBytes + uint64(1)*GasCostPerWarpSigner
				predicate = invalidPredicateBytes
				expectedErr = errFailedVerification
			}

			tests[fmt.Sprintf("multiple predicates %v", validMessageIndices)] = testutils.PredicateTest{
				Config:           NewDefaultConfig(utils.NewUint64(0)),
				PredicateContext: predicateCtx,
				PredicateBytes:   predicate,
				Gas:              expectedGas,
				GasErr:           nil,
				ExpectedErr:      expectedErr,
			}
		}
	}
	testutils.RunPredicateTests(t, tests)
}

func TestWarpSignatureWeightsNonDefaultQuorumNumerator(t *testing.T) {
	predicateCtx := createConsensusCtx([]validatorRange{
		{
			start:     0,
			end:       100,
			weight:    20,
			publicKey: true,
		},
	})

	tests := make(map[string]testutils.PredicateTest)
	nonDefaultQuorumNumerator := 50
	// Ensure this test fails if the DefaultQuroumNumerator is changed to an unexpected value during development
	require.NotEqual(t, nonDefaultQuorumNumerator, int(WarpDefaultQuorumNumerator))
	// Add cases with default quorum
	for _, numSigners := range []int{nonDefaultQuorumNumerator, nonDefaultQuorumNumerator + 1, 99, 100, 101} {
		predicateBytes := createPredicate(numSigners)
		// The predicate is valid iff the number of signers is >= the required numerator and does not exceed the denominator.
		var expectedErr error
		if numSigners >= nonDefaultQuorumNumerator && numSigners <= int(WarpQuorumDenominator) {
			expectedErr = nil
		} else {
			expectedErr = errFailedVerification
		}

		name := fmt.Sprintf("non-default quorum %d signature(s)", numSigners)
		tests[name] = testutils.PredicateTest{
			Config:           NewConfig(utils.NewUint64(0), uint64(nonDefaultQuorumNumerator), false),
			PredicateContext: predicateCtx,
			PredicateBytes:   predicateBytes,
			Gas:              GasCostPerSignatureVerification + uint64(len(predicateBytes))*GasCostPerWarpMessageBytes + uint64(numSigners)*GasCostPerWarpSigner,
			GasErr:           nil,
			ExpectedErr:      expectedErr,
		}
	}

	testutils.RunPredicateTests(t, tests)
}

func initWarpPredicateTests() {
	for _, totalNodes := range []int{10, 100, 1_000, 10_000} {
		testName := fmt.Sprintf("%d signers/%d validators", totalNodes, totalNodes)

		predicateBytes := createPredicate(totalNodes)
		predicateCtx := createConsensusCtx([]validatorRange{
			{
				start:     0,
				end:       totalNodes,
				weight:    20,
				publicKey: true,
			},
		})
		predicateTests[testName] = createValidPredicateTest(predicateCtx, uint64(totalNodes), predicateBytes)
	}

	numSigners := 10
	for _, totalNodes := range []int{100, 1_000, 10_000} {
		testName := fmt.Sprintf("%d signers (heavily weighted)/%d validators", numSigners, totalNodes)

		predicateBytes := createPredicate(numSigners)
		predicateCtx := createConsensusCtx([]validatorRange{
			{
				start:     0,
				end:       numSigners,
				weight:    10_000_000,
				publicKey: true,
			},
			{
				start:     numSigners,
				end:       totalNodes,
				weight:    20,
				publicKey: true,
			},
		})
		predicateTests[testName] = createValidPredicateTest(predicateCtx, uint64(numSigners), predicateBytes)
	}

	for _, totalNodes := range []int{100, 1_000, 10_000} {
		testName := fmt.Sprintf("%d signers (heavily weighted)/%d validators (non-signers without registered PublicKey)", numSigners, totalNodes)

		predicateBytes := createPredicate(numSigners)
		predicateCtx := createConsensusCtx([]validatorRange{
			{
				start:     0,
				end:       numSigners,
				weight:    10_000_000,
				publicKey: true,
			},
			{
				start:     numSigners,
				end:       totalNodes,
				weight:    20,
				publicKey: false,
			},
		})
		predicateTests[testName] = createValidPredicateTest(predicateCtx, uint64(numSigners), predicateBytes)
	}

	for _, totalNodes := range []int{100, 1_000, 10_000} {
		testName := fmt.Sprintf("%d validators w/ %d signers/repeated PublicKeys", totalNodes, numSigners)

		predicateBytes := createPredicate(numSigners)
		getValidatorsOutput := make(map[iface.NodeID]*iface.GetValidatorOutput, totalNodes)
		for i := 0; i < totalNodes; i++ {
			getValidatorsOutput[testVdrs[i].nodeID] = &iface.GetValidatorOutput{
				NodeID:    testVdrs[i].nodeID,
				Weight:    20,
				PublicKey: testVdrs[i%numSigners].vdr.PublicKeyBytes,
			}
		}

		chainCtx := utils.TestChainContext()
		chainCtx.NetworkID = networkID
		chainCtx.ValidatorState = &validatorStateAdapter{
			GetSubnetIDF: func(ctx context.Context, chainID common.Hash) (common.Hash, error) {
				return common.Hash(sourceSubnetID), nil
			},
			GetValidatorSetF: func(ctx context.Context, height uint64, subnetID common.Hash) (map[common.Hash]*iface.ValidatorOutput, error) {
				// Convert the map to the expected type
				result := make(map[common.Hash]*iface.ValidatorOutput)
				for nodeID, v := range getValidatorsOutput {
					result[common.Hash(nodeID)] = ConvertGetValidatorOutputToValidatorOutput(v)
				}
				return result, nil
			},
		}
		
		predicateCtx := &precompileconfig.PredicateContext{
			ConsensusCtx: chainCtx,
			ProposerVMBlockCtx: &commontype.BlockContext{
				PChainHeight: 1,
			},
		}

		predicateTests[testName] = createValidPredicateTest(predicateCtx, uint64(numSigners), predicateBytes)
	}
}

func TestWarpPredicate(t *testing.T) {
	testutils.RunPredicateTests(t, predicateTests)
}

func BenchmarkWarpPredicate(b *testing.B) {
	testutils.RunPredicateBenchmarks(b, predicateTests)
}
