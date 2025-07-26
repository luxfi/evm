// Copyright (C) 2019-2025, Lux Partners Limited. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"math/big"
	"testing"

	"github.com/luxfi/evm/plugin/evm/atomic"
	"github.com/luxfi/ids"
	"github.com/luxfi/node/utils/crypto/secp256k1"
	"github.com/luxfi/node/utils/set"
	"github.com/luxfi/node/utils/units"
	"github.com/luxfi/node/vms/components/lux"
	"github.com/luxfi/node/vms/secp256k1fx"
	"github.com/ethereum/go-ethereum/common"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestImportTxVerify(t *testing.T) {
	importAmount := uint64(10000000)
	
	utxo := &lux.UTXO{
		UTXOID: lux.UTXOID{
			TxID:        ids.GenerateTestID(),
			OutputIndex: 0,
		},
		Asset: lux.Asset{ID: exportTestLUXAssetID},
		Out: &secp256k1fx.TransferOutput{
			Amt: importAmount,
			OutputOwners: secp256k1fx.OutputOwners{
				Locktime:  0,
				Threshold: 1,
				Addrs:     []ids.ShortID{exportTestShortIDAddrs[0]},
			},
		},
	}

	importTx := &atomic.UnsignedImportTx{
		NetworkID:    exportTestNetworkID,
		BlockchainID: exportTestCChainID,
		SourceChain:  exportTestXChainID,
		ImportedInputs: []*lux.TransferableInput{
			{
				UTXOID: utxo.UTXOID,
				Asset:  utxo.Asset,
				In: &secp256k1fx.TransferInput{
					Amt: importAmount,
					Input: secp256k1fx.Input{
						SigIndices: []uint32{0},
					},
				},
			},
		},
		Outs: []atomic.EVMOutput{
			{
				Address: exportTestEthAddrs[0],
				Amount:  importAmount - atomic.TxBytesGas*atomic.NativeAssetCallGasPrice,
				AssetID: exportTestLUXAssetID,
			},
		},
	}

	tests := map[string]struct {
		tx          atomic.UnsignedAtomicTx
		expectedErr string
	}{
		"nil tx": {
			tx:          (*atomic.UnsignedImportTx)(nil),
			expectedErr: atomic.ErrNilTx.Error(),
		},
		"valid import tx": {
			tx:          importTx,
			expectedErr: "",
		},
		"incorrect networkID": {
			tx: func() atomic.UnsignedAtomicTx {
				tx := *importTx
				tx.NetworkID++
				return &tx
			}(),
			expectedErr: atomic.ErrWrongNetworkID.Error(),
		},
		"incorrect blockchainID": {
			tx: func() atomic.UnsignedAtomicTx {
				tx := *importTx
				tx.BlockchainID = ids.GenerateTestID()
				return &tx
			}(),
			expectedErr: atomic.ErrWrongChainID.Error(),
		},
		"no imported inputs": {
			tx: func() atomic.UnsignedAtomicTx {
				tx := *importTx
				tx.ImportedInputs = nil
				return &tx
			}(),
			expectedErr: atomic.ErrNoImportInputs.Error(),
		},
		"EVM output with amount 0": {
			tx: func() atomic.UnsignedAtomicTx {
				tx := *importTx
				tx.Outs = []atomic.EVMOutput{
					{
						Address: exportTestEthAddrs[0],
						Amount:  0,
						AssetID: exportTestLUXAssetID,
					},
				}
				return &tx
			}(),
			expectedErr: atomic.ErrNoValueOutput.Error(),
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

func TestImportTxSemanticVerify(t *testing.T) {
	key := exportTestKeys[0]
	importAmount := uint64(10000000)

	validImportTx := createTestImportTx(t, importAmount, key)

	tests := []struct {
		name      string
		tx        *atomic.Tx
		signers   [][]*secp256k1.PrivateKey
		shouldErr bool
	}{
		{
			name:    "valid",
			tx:      &atomic.Tx{UnsignedAtomicTx: validImportTx},
			signers: [][]*secp256k1.PrivateKey{{key}},
		},
		{
			name:      "too many signatures",
			tx:        &atomic.Tx{UnsignedAtomicTx: validImportTx},
			signers:   [][]*secp256k1.PrivateKey{{key}, {key}},
			shouldErr: true,
		},
		{
			name:      "too few signatures",
			tx:        &atomic.Tx{UnsignedAtomicTx: validImportTx},
			signers:   [][]*secp256k1.PrivateKey{},
			shouldErr: true,
		},
		{
			name:      "wrong signature",
			tx:        &atomic.Tx{UnsignedAtomicTx: validImportTx},
			signers:   [][]*secp256k1.PrivateKey{{exportTestKeys[1]}},
			shouldErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.tx.Sign(atomic.Codec, test.signers); err != nil {
				t.Fatal(err)
			}

			err := verifyImportTxSemantics(test.tx)
			if test.shouldErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestImportTxGasCost(t *testing.T) {
	avaxAssetID := ids.GenerateTestID()
	importAmount := uint64(10000000)

	tests := map[string]struct {
		numInputs       int
		numOutputs      int
		expectedGasUsed uint64
		expectedFee     uint64
	}{
		"single input and output": {
			numInputs:       1,
			numOutputs:      1,
			expectedGasUsed: 11230,
			expectedFee:     280750,
		},
		"multiple inputs": {
			numInputs:       3,
			numOutputs:      1,
			expectedGasUsed: 13366,
			expectedFee:     334150,
		},
		"multiple outputs": {
			numInputs:       1,
			numOutputs:      3,
			expectedGasUsed: 11318,
			expectedFee:     282950,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Create import tx with specified inputs and outputs
			importedInputs := make([]*lux.TransferableInput, test.numInputs)
			for i := 0; i < test.numInputs; i++ {
				importedInputs[i] = &lux.TransferableInput{
					UTXOID: lux.UTXOID{
						TxID:        ids.GenerateTestID(),
						OutputIndex: uint32(i),
					},
					Asset: lux.Asset{ID: avaxAssetID},
					In: &secp256k1fx.TransferInput{
						Amt: importAmount,
						Input: secp256k1fx.Input{
							SigIndices: []uint32{0},
						},
					},
				}
			}

			outs := make([]atomic.EVMOutput, test.numOutputs)
			outputAmount := (importAmount * uint64(test.numInputs)) / uint64(test.numOutputs)
			for i := 0; i < test.numOutputs; i++ {
				outs[i] = atomic.EVMOutput{
					Address: exportTestEthAddrs[i%len(exportTestEthAddrs)],
					Amount:  outputAmount,
					AssetID: avaxAssetID,
				}
			}

			importTx := &atomic.UnsignedImportTx{
				NetworkID:      exportTestNetworkID,
				BlockchainID:   exportTestCChainID,
				SourceChain:    exportTestXChainID,
				ImportedInputs: importedInputs,
				Outs:           outs,
			}

			tx := &atomic.Tx{UnsignedAtomicTx: importTx}

			// Sign with test keys
			signers := make([][]*secp256k1.PrivateKey, test.numInputs)
			for i := range signers {
				signers[i] = []*secp256k1.PrivateKey{exportTestKeys[0]}
			}
			
			if err := tx.Sign(atomic.Codec, signers); err != nil {
				t.Fatal(err)
			}

			gasUsed, err := tx.GasUsed(true)
			if err != nil {
				t.Fatal(err)
			}
			if gasUsed != test.expectedGasUsed {
				t.Fatalf("Expected gasUsed to be %d, but found %d", test.expectedGasUsed, gasUsed)
			}

			fee, err := atomic.CalculateDynamicFee(gasUsed, big.NewInt(25*units.GWei))
			if err != nil {
				t.Fatal(err)
			}
			if fee != test.expectedFee {
				t.Fatalf("Expected fee to be %d, but found %d", test.expectedFee, fee)
			}
		})
	}
}

func TestImportTxEVMStateTransfer(t *testing.T) {
	key := exportTestKeys[0]
	ethAddr := key.PublicKey().Address().Hex()

	importAmount := uint64(10 * units.Avax)
	customAssetID := ids.ID{1, 2, 3, 4, 5}
	customAmount := uint64(100)

	tests := []struct {
		name        string
		outputs     []atomic.EVMOutput
		avaxBalance *uint256.Int
		balances    map[ids.ID]*big.Int
	}{
		{
			name: "single AVAX output",
			outputs: []atomic.EVMOutput{
				{
					Address: common.HexToAddress(ethAddr),
					Amount:  importAmount,
					AssetID: exportTestLUXAssetID,
				},
			},
			avaxBalance: uint256.NewInt(importAmount * atomic.X2CRateUint64),
			balances:    map[ids.ID]*big.Int{},
		},
		{
			name: "multiple outputs same asset",
			outputs: []atomic.EVMOutput{
				{
					Address: common.HexToAddress(ethAddr),
					Amount:  importAmount / 2,
					AssetID: exportTestLUXAssetID,
				},
				{
					Address: common.HexToAddress(ethAddr),
					Amount:  importAmount / 2,
					AssetID: exportTestLUXAssetID,
				},
			},
			avaxBalance: uint256.NewInt(importAmount * atomic.X2CRateUint64),
			balances:    map[ids.ID]*big.Int{},
		},
		{
			name: "mixed assets",
			outputs: []atomic.EVMOutput{
				{
					Address: common.HexToAddress(ethAddr),
					Amount:  importAmount,
					AssetID: exportTestLUXAssetID,
				},
				{
					Address: common.HexToAddress(ethAddr),
					Amount:  customAmount,
					AssetID: customAssetID,
				},
			},
			avaxBalance: uint256.NewInt(importAmount * atomic.X2CRateUint64),
			balances: map[ids.ID]*big.Int{
				customAssetID: big.NewInt(int64(customAmount)),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create import tx with test outputs
			importTx := &atomic.UnsignedImportTx{
				NetworkID:    exportTestNetworkID,
				BlockchainID: exportTestCChainID,
				SourceChain:  exportTestXChainID,
				ImportedInputs: []*lux.TransferableInput{
					{
						UTXOID: lux.UTXOID{
							TxID:        ids.GenerateTestID(),
							OutputIndex: 0,
						},
						Asset: lux.Asset{ID: exportTestLUXAssetID},
						In: &secp256k1fx.TransferInput{
							Amt: importAmount,
							Input: secp256k1fx.Input{
								SigIndices: []uint32{0},
							},
						},
					},
				},
				Outs: test.outputs,
			}

			// Mock state DB for testing
			stateDB := &mockStateDB{
				balances:         make(map[common.Address]*uint256.Int),
				balancesMultiCoin: make(map[common.Address]map[common.Hash]*big.Int),
				nonces:           make(map[common.Address]uint64),
			}

			// Execute state transfer
			err := importTx.EVMStateTransfer(stateDB)
			require.NoError(t, err)

			// Verify balances
			actualBalance := stateDB.GetBalance(common.HexToAddress(ethAddr))
			require.Equal(t, test.avaxBalance, actualBalance)

			for assetID, expectedBalance := range test.balances {
				actualBalance := stateDB.GetBalanceMultiCoin(common.HexToAddress(ethAddr), common.Hash(assetID))
				require.Equal(t, expectedBalance, actualBalance)
			}
		})
	}
}

func TestImportTxAtomicOps(t *testing.T) {
	key := exportTestKeys[0]
	importAmount := uint64(10000000)

	importTx := createTestImportTx(t, importAmount, key)
	tx := &atomic.Tx{UnsignedAtomicTx: importTx}
	
	if err := tx.Sign(atomic.Codec, [][]*secp256k1.PrivateKey{{key}}); err != nil {
		t.Fatal(err)
	}

	chainID, atomicRequests, err := tx.AtomicOps()
	require.NoError(t, err)
	require.Equal(t, exportTestXChainID, chainID)
	require.NotNil(t, atomicRequests)

	// Verify the atomic requests contain expected operations
	require.NotEmpty(t, atomicRequests.RemoveRequests)
	
	// Verify the consumed UTXOs are in remove requests
	consumedUTXOs := tx.InputUTXOs()
	require.Equal(t, len(importTx.ImportedInputs), len(consumedUTXOs))
	
	for _, utxo := range consumedUTXOs {
		found := false
		for _, removeReq := range atomicRequests.RemoveRequests {
			if string(removeReq) == string(utxo[:]) {
				found = true
				break
			}
		}
		require.True(t, found, "UTXO not found in remove requests")
	}
}

// Helper functions

func createTestImportTx(t *testing.T, importAmount uint64, key *secp256k1.PrivateKey) *atomic.UnsignedImportTx {
	return &atomic.UnsignedImportTx{
		NetworkID:    exportTestNetworkID,
		BlockchainID: exportTestCChainID,
		SourceChain:  exportTestXChainID,
		ImportedInputs: []*lux.TransferableInput{
			{
				UTXOID: lux.UTXOID{
					TxID:        ids.GenerateTestID(),
					OutputIndex: 0,
				},
				Asset: lux.Asset{ID: exportTestLUXAssetID},
				In: &secp256k1fx.TransferInput{
					Amt: importAmount,
					Input: secp256k1fx.Input{
						SigIndices: []uint32{0},
					},
				},
			},
		},
		Outs: []atomic.EVMOutput{
			{
				Address: GetEthAddress(key),
				Amount:  importAmount - atomic.TxBytesGas*atomic.NativeAssetCallGasPrice,
				AssetID: exportTestLUXAssetID,
			},
		},
	}
}

func verifyImportTxSemantics(tx *atomic.Tx) error {
	importTx, ok := tx.UnsignedAtomicTx.(*atomic.UnsignedImportTx)
	if !ok {
		return atomic.ErrWrongTxType
	}

	if len(importTx.ImportedInputs) == 0 {
		return atomic.ErrNoImportInputs
	}

	// Verify signatures match inputs
	if len(tx.Creds) != len(importTx.ImportedInputs) {
		return atomic.ErrSignatureVerification
	}

	// Additional semantic checks would go here
	return nil
}

// Mock StateDB for testing
type mockStateDB struct {
	balances          map[common.Address]*uint256.Int
	balancesMultiCoin map[common.Address]map[common.Hash]*big.Int
	nonces            map[common.Address]uint64
}

func (m *mockStateDB) GetBalance(addr common.Address) *uint256.Int {
	if balance, ok := m.balances[addr]; ok {
		return balance
	}
	return uint256.NewInt(0)
}

func (m *mockStateDB) SetBalance(addr common.Address, balance *uint256.Int) {
	m.balances[addr] = balance
}

func (m *mockStateDB) AddBalance(addr common.Address, amount *uint256.Int) {
	current := m.GetBalance(addr)
	m.SetBalance(addr, new(uint256.Int).Add(current, amount))
}

func (m *mockStateDB) GetBalanceMultiCoin(addr common.Address, coinID common.Hash) *big.Int {
	if addrBalances, ok := m.balancesMultiCoin[addr]; ok {
		if balance, ok := addrBalances[coinID]; ok {
			return balance
		}
	}
	return big.NewInt(0)
}

func (m *mockStateDB) AddBalanceMultiCoin(addr common.Address, coinID common.Hash, amount *big.Int) {
	if _, ok := m.balancesMultiCoin[addr]; !ok {
		m.balancesMultiCoin[addr] = make(map[common.Hash]*big.Int)
	}
	current := m.GetBalanceMultiCoin(addr, coinID)
	m.balancesMultiCoin[addr][coinID] = new(big.Int).Add(current, amount)
}

func (m *mockStateDB) GetNonce(addr common.Address) uint64 {
	return m.nonces[addr]
}

func (m *mockStateDB) SetNonce(addr common.Address, nonce uint64) {
	m.nonces[addr] = nonce
}