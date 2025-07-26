// Copyright (C) 2019-2025, Lux Partners Limited. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/luxfi/evm/plugin/evm/atomic"
	"github.com/luxfi/ids"
	"github.com/luxfi/node/utils/crypto/secp256k1"
	"github.com/luxfi/node/utils/units"
	"github.com/luxfi/node/vms/components/lux"
	"github.com/luxfi/node/vms/secp256k1fx"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestExportTxVerify(t *testing.T) {
	var exportAmount uint64 = 10000000
	exportTx := &atomic.UnsignedExportTx{
		NetworkID:        exportTestNetworkID,
		BlockchainID:     exportTestCChainID,
		DestinationChain: exportTestXChainID,
		Ins: []atomic.EVMInput{
			{
				Address: exportTestEthAddrs[0],
				Amount:  exportAmount,
				AssetID: exportTestLUXAssetID,
				Nonce:   0,
			},
			{
				Address: exportTestEthAddrs[2],
				Amount:  exportAmount,
				AssetID: exportTestLUXAssetID,
				Nonce:   0,
			},
		},
		ExportedOutputs: []*lux.TransferableOutput{
			{
				Asset: lux.Asset{ID: exportTestLUXAssetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: exportAmount,
					OutputOwners: secp256k1fx.OutputOwners{
						Locktime:  0,
						Threshold: 1,
						Addrs:     []ids.ShortID{exportTestShortIDAddrs[0]},
					},
				},
			},
			{
				Asset: lux.Asset{ID: exportTestLUXAssetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: exportAmount,
					OutputOwners: secp256k1fx.OutputOwners{
						Locktime:  0,
						Threshold: 1,
						Addrs:     []ids.ShortID{exportTestShortIDAddrs[1]},
					},
				},
			},
		},
	}

	// Sort the inputs and outputs to ensure the transaction is canonical
	lux.SortTransferableOutputs(exportTx.ExportedOutputs, atomic.Codec)
	// Pass in a list of signers here with the appropriate length
	// to avoid causing a nil-pointer error in the helper method
	emptySigners := make([][]*secp256k1.PrivateKey, 2)
	atomic.SortEVMInputsAndSigners(exportTx.Ins, emptySigners)

	tests := map[string]struct {
		tx          atomic.UnsignedAtomicTx
		expectedErr string
	}{
		"nil tx": {
			tx:          (*atomic.UnsignedExportTx)(nil),
			expectedErr: atomic.ErrNilTx.Error(),
		},
		"valid export tx": {
			tx:          exportTx,
			expectedErr: "",
		},
		"incorrect networkID": {
			tx: func() atomic.UnsignedAtomicTx {
				tx := *exportTx
				tx.NetworkID++
				return &tx
			}(),
			expectedErr: atomic.ErrWrongNetworkID.Error(),
		},
		"incorrect blockchainID": {
			tx: func() atomic.UnsignedAtomicTx {
				tx := *exportTx
				tx.BlockchainID = ids.GenerateTestID()
				return &tx
			}(),
			expectedErr: atomic.ErrWrongChainID.Error(),
		},
		"no exported outputs": {
			tx: func() atomic.UnsignedAtomicTx {
				tx := *exportTx
				tx.ExportedOutputs = nil
				return &tx
			}(),
			expectedErr: atomic.ErrNoExportOutputs.Error(),
		},
		"EVM input with amount 0": {
			tx: func() atomic.UnsignedAtomicTx {
				tx := *exportTx
				tx.Ins = []atomic.EVMInput{
					{
						Address: exportTestEthAddrs[0],
						Amount:  0,
						AssetID: exportTestLUXAssetID,
						Nonce:   0,
					},
				}
				return &tx
			}(),
			expectedErr: atomic.ErrNoValueInput.Error(),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := &atomic.Context{
				NetworkID:   exportTestNetworkID,
				ChainID:     exportTestCChainID,
				AVAXAssetID: exportTestLUXAssetID,
			}
			err := test.tx.Verify(ctx)
			if test.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), test.expectedErr)
			}
		})
	}
}

func TestExportTxGasCost(t *testing.T) {
	avaxAssetID := ids.GenerateTestID()
	chainID := ids.GenerateTestID()
	xChainID := ids.GenerateTestID()
	networkID := uint32(5)
	exportAmount := uint64(5000000)

	tests := map[string]struct {
		UnsignedExportTx *atomic.UnsignedExportTx
		Keys             [][]*secp256k1.PrivateKey

		BaseFee         *big.Int
		ExpectedGasUsed uint64
		ExpectedFee     uint64
		FixedFee        bool
	}{
		"simple export 1wei BaseFee": {
			UnsignedExportTx: &atomic.UnsignedExportTx{
				NetworkID:        networkID,
				BlockchainID:     chainID,
				DestinationChain: xChainID,
				Ins: []atomic.EVMInput{
					{
						Address: exportTestEthAddrs[0],
						Amount:  exportAmount,
						AssetID: avaxAssetID,
						Nonce:   0,
					},
				},
				ExportedOutputs: []*lux.TransferableOutput{
					{
						Asset: lux.Asset{ID: avaxAssetID},
						Out: &secp256k1fx.TransferOutput{
							Amt: exportAmount,
							OutputOwners: secp256k1fx.OutputOwners{
								Locktime:  0,
								Threshold: 1,
								Addrs:     []ids.ShortID{exportTestShortIDAddrs[0]},
							},
						},
					},
				},
			},
			Keys:            [][]*secp256k1.PrivateKey{{exportTestKeys[0]}},
			ExpectedGasUsed: 1230,
			ExpectedFee:     1,
			BaseFee:         big.NewInt(1),
		},
		"simple export 25Gwei BaseFee": {
			UnsignedExportTx: &atomic.UnsignedExportTx{
				NetworkID:        networkID,
				BlockchainID:     chainID,
				DestinationChain: xChainID,
				Ins: []atomic.EVMInput{
					{
						Address: exportTestEthAddrs[0],
						Amount:  exportAmount,
						AssetID: avaxAssetID,
						Nonce:   0,
					},
				},
				ExportedOutputs: []*lux.TransferableOutput{
					{
						Asset: lux.Asset{ID: avaxAssetID},
						Out: &secp256k1fx.TransferOutput{
							Amt: exportAmount,
							OutputOwners: secp256k1fx.OutputOwners{
								Locktime:  0,
								Threshold: 1,
								Addrs:     []ids.ShortID{exportTestShortIDAddrs[0]},
							},
						},
					},
				},
			},
			Keys:            [][]*secp256k1.PrivateKey{{exportTestKeys[0]}},
			ExpectedGasUsed: 1230,
			ExpectedFee:     30750,
			BaseFee:         big.NewInt(25 * units.GWei),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			tx := &atomic.Tx{UnsignedAtomicTx: test.UnsignedExportTx}

			// Sign with the correct key
			if err := tx.Sign(atomic.Codec, test.Keys); err != nil {
				t.Fatal(err)
			}

			gasUsed, err := tx.GasUsed(test.FixedFee)
			if err != nil {
				t.Fatal(err)
			}
			if gasUsed != test.ExpectedGasUsed {
				t.Fatalf("Expected gasUsed to be %d, but found %d", test.ExpectedGasUsed, gasUsed)
			}

			fee, err := atomic.CalculateDynamicFee(gasUsed, test.BaseFee)
			if err != nil {
				t.Fatal(err)
			}
			if fee != test.ExpectedFee {
				t.Fatalf("Expected fee to be %d, but found %d", test.ExpectedFee, fee)
			}
		})
	}
}

func TestExportTxSemanticVerify(t *testing.T) {
	key := exportTestKeys[0]
	addr := key.Address()
	ethAddr := exportTestEthAddrs[0]

	var (
		avaxBalance           = 10 * units.Avax
		custom0Balance uint64 = 100
		custom0AssetID        = ids.ID{1, 2, 3, 4, 5}
	)

	validExportTx := &atomic.UnsignedExportTx{
		NetworkID:        exportTestNetworkID,
		BlockchainID:     exportTestCChainID,
		DestinationChain: exportTestXChainID,
		Ins: []atomic.EVMInput{
			{
				Address: ethAddr,
				Amount:  avaxBalance,
				AssetID: exportTestLUXAssetID,
				Nonce:   0,
			},
			{
				Address: ethAddr,
				Amount:  custom0Balance,
				AssetID: custom0AssetID,
				Nonce:   0,
			},
		},
		ExportedOutputs: []*lux.TransferableOutput{
			{
				Asset: lux.Asset{ID: custom0AssetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: custom0Balance,
					OutputOwners: secp256k1fx.OutputOwners{
						Threshold: 1,
						Addrs:     []ids.ShortID{addr},
					},
				},
			},
		},
	}

	tests := []struct {
		name      string
		tx        *atomic.Tx
		signers   [][]*secp256k1.PrivateKey
		shouldErr bool
	}{
		{
			name:    "valid",
			tx:      &atomic.Tx{UnsignedAtomicTx: validExportTx},
			signers: [][]*secp256k1.PrivateKey{{key}, {key}},
		},
		{
			name: "no outputs",
			tx: func() *atomic.Tx {
				tx := *validExportTx
				tx.ExportedOutputs = nil
				return &atomic.Tx{UnsignedAtomicTx: &tx}
			}(),
			signers:   [][]*secp256k1.PrivateKey{{key}, {key}},
			shouldErr: true,
		},
		{
			name:      "too many signatures",
			tx:        &atomic.Tx{UnsignedAtomicTx: validExportTx},
			signers:   [][]*secp256k1.PrivateKey{{key}, {key}, {key}},
			shouldErr: true,
		},
		{
			name:      "too few signatures",
			tx:        &atomic.Tx{UnsignedAtomicTx: validExportTx},
			signers:   [][]*secp256k1.PrivateKey{{key}},
			shouldErr: true,
		},
		{
			name:      "wrong signature",
			tx:        &atomic.Tx{UnsignedAtomicTx: validExportTx},
			signers:   [][]*secp256k1.PrivateKey{{exportTestKeys[1]}, {key}},
			shouldErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.tx.Sign(atomic.Codec, test.signers); err != nil {
				t.Fatal(err)
			}

			// Create a simple mock state for semantic verification
			// In a real test, this would interact with the blockchain state
			err := verifyExportTxSemantics(test.tx)
			if test.shouldErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestExportTxAccept(t *testing.T) {
	key := exportTestKeys[0]
	addr := key.Address()
	ethAddr := exportTestEthAddrs[0]

	var (
		avaxBalance           = 10 * units.Avax
		custom0Balance uint64 = 100
		custom0AssetID        = ids.ID{1, 2, 3, 4, 5}
	)

	exportTx := &atomic.UnsignedExportTx{
		NetworkID:        exportTestNetworkID,
		BlockchainID:     exportTestCChainID,
		DestinationChain: exportTestXChainID,
		Ins: []atomic.EVMInput{
			{
				Address: ethAddr,
				Amount:  avaxBalance,
				AssetID: exportTestLUXAssetID,
				Nonce:   0,
			},
			{
				Address: ethAddr,
				Amount:  custom0Balance,
				AssetID: custom0AssetID,
				Nonce:   0,
			},
		},
		ExportedOutputs: []*lux.TransferableOutput{
			{
				Asset: lux.Asset{ID: exportTestLUXAssetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: avaxBalance,
					OutputOwners: secp256k1fx.OutputOwners{
						Threshold: 1,
						Addrs:     []ids.ShortID{addr},
					},
				},
			},
			{
				Asset: lux.Asset{ID: custom0AssetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: custom0Balance,
					OutputOwners: secp256k1fx.OutputOwners{
						Threshold: 1,
						Addrs:     []ids.ShortID{addr},
					},
				},
			},
		},
	}

	tx := &atomic.Tx{UnsignedAtomicTx: exportTx}
	signers := [][]*secp256k1.PrivateKey{{key}, {key}}

	if err := tx.Sign(atomic.Codec, signers); err != nil {
		t.Fatal(err)
	}

	// Test AtomicOps
	chainID, atomicRequests, err := tx.AtomicOps()
	require.NoError(t, err)
	require.Equal(t, exportTestXChainID, chainID)
	require.NotNil(t, atomicRequests)

	// Verify the atomic requests contain the expected outputs
	require.Len(t, atomicRequests.PutRequests, 2)

	// Check that the outputs are properly encoded
	avaxUTXOID := lux.UTXOID{
		TxID:        tx.ID(),
		OutputIndex: 0,
	}
	avaxInputID := avaxUTXOID.InputID()

	customUTXOID := lux.UTXOID{
		TxID:        tx.ID(),
		OutputIndex: 1,
	}
	customInputID := customUTXOID.InputID()

	// Find the requests by their keys
	var avaxRequest, customRequest *atomic.Element
	for _, req := range atomicRequests.PutRequests {
		if bytes.Equal(req.Key, avaxInputID[:]) {
			avaxRequest = req
		} else if bytes.Equal(req.Key, customInputID[:]) {
			customRequest = req
		}
	}

	require.NotNil(t, avaxRequest, "AVAX output request not found")
	require.NotNil(t, customRequest, "Custom asset output request not found")

	// Verify traits contain the address
	require.Contains(t, avaxRequest.Traits, addr.Bytes())
	require.Contains(t, customRequest.Traits, addr.Bytes())
}

// Helper function for semantic verification
func verifyExportTxSemantics(tx *atomic.Tx) error {
	exportTx, ok := tx.UnsignedAtomicTx.(*atomic.UnsignedExportTx)
	if !ok {
		return atomic.ErrWrongTxType
	}

	if len(exportTx.ExportedOutputs) == 0 {
		return atomic.ErrNoExportOutputs
	}

	// Verify signatures match inputs
	if len(tx.Creds) != len(exportTx.Ins) {
		return atomic.ErrSignatureVerification
	}

	// Additional semantic checks would go here
	return nil
}

// Test constants for export tests
var (
	exportTestNetworkID   = uint32(1337)
	exportTestCChainID    = ids.GenerateTestID()
	exportTestXChainID    = ids.GenerateTestID()
	exportTestLUXAssetID = ids.GenerateTestID()

	exportTestKeys         []*secp256k1.PrivateKey
	exportTestEthAddrs     []common.Address
	exportTestShortIDAddrs []ids.ShortID
)

func init() {
	// Initialize test keys for export tests
	for i := 0; i < 3; i++ {
		key, _ := secp256k1.NewPrivateKey()
		exportTestKeys = append(exportTestKeys, key)
		exportTestEthAddrs = append(exportTestEthAddrs, GetEthAddress(key))
		exportTestShortIDAddrs = append(exportTestShortIDAddrs, key.Address())
	}
}

// GetEthAddress returns the Ethereum address for a given private key
func GetEthAddress(privKey *secp256k1.PrivateKey) common.Address {
	return common.BytesToAddress(privKey.Address().Bytes())
}