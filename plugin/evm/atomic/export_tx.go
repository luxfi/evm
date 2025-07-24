// (c) 2019-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package atomic

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/luxfi/geth/params"
	"github.com/luxfi/evm/plugin/evm/upgrade/ap0"
	"github.com/luxfi/evm/plugin/evm/upgrade/ap5"
	"github.com/holiman/uint256"

	"github.com/luxfi/node/chains/atomic"
	"github.com/luxfi/node/ids"
	"github.com/luxfi/evm/consensus"
	avalancheutils "github.com/luxfi/node/utils"
	"github.com/luxfi/node/utils/constants"
	"github.com/luxfi/node/utils/crypto/secp256k1"
	"github.com/luxfi/node/utils/math"
	"github.com/luxfi/node/utils/set"
	"github.com/luxfi/node/utils/wrappers"
	"github.com/luxfi/node/vms/components/lux"
	"github.com/luxfi/node/vms/components/verify"
	"github.com/luxfi/node/vms/secp256k1fx"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/log"
)

var (
	_                             UnsignedAtomicTx       = &UnsignedExportTx{}
	_                             secp256k1fx.UnsignedTx = &UnsignedExportTx{}
	ErrExportNonLUXInputBanff                           = errors.New("export input cannot contain non-LUX in Banff")
	ErrExportNonLUXOutputBanff                          = errors.New("export output cannot contain non-LUX in Banff")
	ErrNoExportOutputs                                   = errors.New("tx has no export outputs")
	errPublicKeySignatureMismatch                        = errors.New("signature doesn't match public key")
	errOverflowExport                                    = errors.New("overflow when computing export amount + txFee")
	errInsufficientFunds                                 = errors.New("insufficient funds")
	errInvalidNonce                                      = errors.New("invalid nonce")
)

// UnsignedExportTx is an unsigned ExportTx
type UnsignedExportTx struct {
	Metadata
	// ID of the network on which this tx was issued
	NetworkID uint32 `serialize:"true" json:"networkID"`
	// ID of this blockchain.
	BlockchainID ids.ID `serialize:"true" json:"blockchainID"`
	// Which chain to send the funds to
	DestinationChain ids.ID `serialize:"true" json:"destinationChain"`
	// Inputs
	Ins []EVMInput `serialize:"true" json:"inputs"`
	// Outputs that are exported to the chain
	ExportedOutputs []*lux.TransferableOutput `serialize:"true" json:"exportedOutputs"`
}

// InputUTXOs returns a set of all the hash(address:nonce) exporting funds.
func (utx *UnsignedExportTx) InputUTXOs() set.Set[ids.ID] {
	set := set.NewSet[ids.ID](len(utx.Ins))
	for _, in := range utx.Ins {
		// Total populated bytes is exactly 32 bytes.
		// 8 (Nonce) + 4 (Address Length) + 20 (Address)
		var rawID [32]byte
		packer := wrappers.Packer{Bytes: rawID[:]}
		packer.PackLong(in.Nonce)
		packer.PackBytes(in.Address.Bytes())
		set.Add(ids.ID(rawID))
	}
	return set
}

// Verify this transaction is well-formed
func (utx *UnsignedExportTx) Verify(
	ctx *consensus.Context,
	rules params.Rules,
) error {
	switch {
	case utx == nil:
		return ErrNilTx
	case len(utx.ExportedOutputs) == 0:
		return ErrNoExportOutputs
	case utx.NetworkID != ctx.NetworkID:
		return ErrWrongNetworkID
	case ctx.ChainID != utx.BlockchainID:
		return ErrWrongChainID
	}

	// Make sure that the tx has a valid peer chain ID
	if rules.IsApricotPhase5 {
		// Note that SameSubnet verifies that [tx.DestinationChain] isn't this
		// chain's ID
		if err := verify.SameSubnet(context.TODO(), ctx, utx.DestinationChain); err != nil {
			return ErrWrongChainID
		}
	} else {
		if utx.DestinationChain != ctx.XChainID {
			return ErrWrongChainID
		}
	}

	for _, in := range utx.Ins {
		if err := in.Verify(); err != nil {
			return err
		}
		if rules.IsBanff && in.AssetID != ctx.LUXAssetID {
			return ErrExportNonLUXInputBanff
		}
	}

	for _, out := range utx.ExportedOutputs {
		if err := out.Verify(); err != nil {
			return err
		}
		assetID := out.AssetID()
		if assetID != ctx.LUXAssetID && utx.DestinationChain == constants.PlatformChainID {
			return ErrWrongChainID
		}
		if rules.IsBanff && assetID != ctx.LUXAssetID {
			return ErrExportNonLUXOutputBanff
		}
	}
	if !lux.IsSortedTransferableOutputs(utx.ExportedOutputs, Codec) {
		return ErrOutputsNotSorted
	}
	if rules.IsApricotPhase1 && !avalancheutils.IsSortedAndUnique(utx.Ins) {
		return ErrInputsNotSortedUnique
	}

	return nil
}

func (utx *UnsignedExportTx) GasUsed(fixedFee bool) (uint64, error) {
	byteCost := calcBytesCost(len(utx.Bytes()))
	numSigs := uint64(len(utx.Ins))
	sigCost, err := math.Mul64(numSigs, secp256k1fx.CostPerSignature)
	if err != nil {
		return 0, err
	}
	cost, err := math.Add64(byteCost, sigCost)
	if err != nil {
		return 0, err
	}
	if fixedFee {
		cost, err = math.Add64(cost, ap5.AtomicTxIntrinsicGas)
		if err != nil {
			return 0, err
		}
	}

	return cost, nil
}

// Amount of [assetID] burned by this transaction
func (utx *UnsignedExportTx) Burned(assetID ids.ID) (uint64, error) {
	var (
		spent uint64
		input uint64
		err   error
	)
	for _, out := range utx.ExportedOutputs {
		if out.AssetID() == assetID {
			spent, err = math.Add64(spent, out.Output().Amount())
			if err != nil {
				return 0, err
			}
		}
	}
	for _, in := range utx.Ins {
		if in.AssetID == assetID {
			input, err = math.Add64(input, in.Amount)
			if err != nil {
				return 0, err
			}
		}
	}

	return math.Sub(input, spent)
}

// SemanticVerify this transaction is valid.
func (utx *UnsignedExportTx) SemanticVerify(
	backend *Backend,
	stx *Tx,
	parent AtomicBlockContext,
	baseFee *big.Int,
) error {
	ctx := backend.Ctx
	rules := backend.Rules
	if err := utx.Verify(ctx, rules); err != nil {
		return err
	}

	// Check the transaction consumes and produces the right amounts
	fc := lux.NewFlowChecker()
	switch {
	// Apply dynamic fees to export transactions as of Apricot Phase 3
	case rules.IsApricotPhase3:
		gasUsed, err := stx.GasUsed(rules.IsApricotPhase5)
		if err != nil {
			return err
		}
		txFee, err := CalculateDynamicFee(gasUsed, baseFee)
		if err != nil {
			return err
		}
		fc.Produce(ctx.LUXAssetID, txFee)
	// Apply fees to export transactions before Apricot Phase 3
	default:
		fc.Produce(ctx.LUXAssetID, ap0.AtomicTxFee)
	}
	for _, out := range utx.ExportedOutputs {
		fc.Produce(out.AssetID(), out.Output().Amount())
	}
	for _, in := range utx.Ins {
		fc.Consume(in.AssetID, in.Amount)
	}

	if err := fc.Verify(); err != nil {
		return fmt.Errorf("export tx flow check failed due to: %w", err)
	}

	if len(utx.Ins) != len(stx.Creds) {
		return fmt.Errorf("export tx contained mismatched number of inputs/credentials (%d vs. %d)", len(utx.Ins), len(stx.Creds))
	}

	for i, input := range utx.Ins {
		cred, ok := stx.Creds[i].(*secp256k1fx.Credential)
		if !ok {
			return fmt.Errorf("expected *secp256k1fx.Credential but got %T", cred)
		}
		if err := cred.Verify(); err != nil {
			return err
		}

		if len(cred.Sigs) != 1 {
			return fmt.Errorf("expected one signature for EVM Input Credential, but found: %d", len(cred.Sigs))
		}
		pubKey, err := backend.SecpCache.RecoverPublicKey(utx.Bytes(), cred.Sigs[0][:])
		if err != nil {
			return err
		}
		if input.Address != pubKey.EthAddress() {
			return errPublicKeySignatureMismatch
		}
	}

	return nil
}

// AtomicOps returns the atomic operations for this transaction.
func (utx *UnsignedExportTx) AtomicOps() (ids.ID, *atomic.Requests, error) {
	txID := utx.ID()

	elems := make([]*atomic.Element, len(utx.ExportedOutputs))
	for i, out := range utx.ExportedOutputs {
		utxo := &lux.UTXO{
			UTXOID: lux.UTXOID{
				TxID:        txID,
				OutputIndex: uint32(i),
			},
			Asset: lux.Asset{ID: out.AssetID()},
			Out:   out.Out,
		}

		utxoBytes, err := Codec.Marshal(CodecVersion, utxo)
		if err != nil {
			return ids.ID{}, nil, err
		}
		utxoID := utxo.InputID()
		elem := &atomic.Element{
			Key:   utxoID[:],
			Value: utxoBytes,
		}
		if out, ok := utxo.Out.(lux.Addressable); ok {
			elem.Traits = out.Addresses()
		}

		elems[i] = elem
	}

	return utx.DestinationChain, &atomic.Requests{PutRequests: elems}, nil
}

// NewExportTx returns a new ExportTx
func NewExportTx(
	ctx *consensus.Context,
	rules params.Rules,
	state StateDB,
	assetID ids.ID, // AssetID of the tokens to export
	amount uint64, // Amount of tokens to export
	chainID ids.ID, // Chain to send the UTXOs to
	to ids.ShortID, // Address of chain recipient
	baseFee *big.Int, // fee to use post-AP3
	keys []*secp256k1.PrivateKey, // Pay the fee and provide the tokens
) (*Tx, error) {
	outs := []*lux.TransferableOutput{{
		Asset: lux.Asset{ID: assetID},
		Out: &secp256k1fx.TransferOutput{
			Amt: amount,
			OutputOwners: secp256k1fx.OutputOwners{
				Locktime:  0,
				Threshold: 1,
				Addrs:     []ids.ShortID{to},
			},
		},
	}}

	var (
		luxNeeded           uint64 = 0
		ins, luxIns         []EVMInput
		signers, luxSigners [][]*secp256k1.PrivateKey
		err                  error
	)

	// consume non-LUX
	if assetID != ctx.LUXAssetID {
		ins, signers, err = GetSpendableFunds(ctx, state, keys, assetID, amount)
		if err != nil {
			return nil, fmt.Errorf("couldn't generate tx inputs/signers: %w", err)
		}
	} else {
		luxNeeded = amount
	}

	switch {
	case rules.IsApricotPhase3:
		utx := &UnsignedExportTx{
			NetworkID:        ctx.NetworkID,
			BlockchainID:     ctx.ChainID,
			DestinationChain: chainID,
			Ins:              ins,
			ExportedOutputs:  outs,
		}
		tx := &Tx{UnsignedAtomicTx: utx}
		if err := tx.Sign(Codec, nil); err != nil {
			return nil, err
		}

		var cost uint64
		cost, err = tx.GasUsed(rules.IsApricotPhase5)
		if err != nil {
			return nil, err
		}

		luxIns, luxSigners, err = GetSpendableLUXWithFee(ctx, state, keys, luxNeeded, cost, baseFee)
	default:
		var newLuxNeeded uint64
		newLuxNeeded, err = math.Add64(luxNeeded, ap0.AtomicTxFee)
		if err != nil {
			return nil, errOverflowExport
		}
		luxIns, luxSigners, err = GetSpendableFunds(ctx, state, keys, ctx.LUXAssetID, newLuxNeeded)
	}
	if err != nil {
		return nil, fmt.Errorf("couldn't generate tx inputs/signers: %w", err)
	}
	ins = append(ins, luxIns...)
	signers = append(signers, luxSigners...)

	lux.SortTransferableOutputs(outs, Codec)
	SortEVMInputsAndSigners(ins, signers)

	// Create the transaction
	utx := &UnsignedExportTx{
		NetworkID:        ctx.NetworkID,
		BlockchainID:     ctx.ChainID,
		DestinationChain: chainID,
		Ins:              ins,
		ExportedOutputs:  outs,
	}
	tx := &Tx{UnsignedAtomicTx: utx}
	if err := tx.Sign(Codec, signers); err != nil {
		return nil, err
	}
	return tx, utx.Verify(ctx, rules)
}

// EVMStateTransfer executes the state update from the atomic export transaction
func (utx *UnsignedExportTx) EVMStateTransfer(ctx *consensus.Context, state StateDB) error {
	addrs := map[[20]byte]uint64{}
	for _, from := range utx.Ins {
		if from.AssetID == ctx.LUXAssetID {
			log.Debug("export_tx", "dest", utx.DestinationChain, "addr", from.Address, "amount", from.Amount, "assetID", "LUX")
			// We multiply the input amount by x2cRate to convert LUX back to the appropriate
			// denomination before export.
			amount := new(uint256.Int).Mul(
				uint256.NewInt(from.Amount),
				uint256.NewInt(X2CRate.Uint64()),
			)
			if state.GetBalance(from.Address).Cmp(amount) < 0 {
				return errInsufficientFunds
			}
			state.SubBalance(from.Address, amount)
		} else {
			log.Debug("export_tx", "dest", utx.DestinationChain, "addr", from.Address, "amount", from.Amount, "assetID", from.AssetID)
			amount := new(big.Int).SetUint64(from.Amount)
			if state.GetBalanceMultiCoin(from.Address, common.Hash(from.AssetID)).Cmp(amount) < 0 {
				return errInsufficientFunds
			}
			state.SubBalanceMultiCoin(from.Address, common.Hash(from.AssetID), amount)
		}
		if state.GetNonce(from.Address) != from.Nonce {
			return errInvalidNonce
		}
		addrs[from.Address] = from.Nonce
	}
	for addr, nonce := range addrs {
		state.SetNonce(addr, nonce+1)
	}
	return nil
}

// GetSpendableFunds returns a list of EVMInputs and keys (in corresponding
// order) to total [amount] of [assetID] owned by [keys].
// Note: we return [][]*secp256k1.PrivateKey even though each input
// corresponds to a single key, so that the signers can be passed in to
// [tx.Sign] which supports multiple keys on a single input.
func GetSpendableFunds(
	ctx *consensus.Context,
	state StateDB,
	keys []*secp256k1.PrivateKey,
	assetID ids.ID,
	amount uint64,
) ([]EVMInput, [][]*secp256k1.PrivateKey, error) {
	inputs := []EVMInput{}
	signers := [][]*secp256k1.PrivateKey{}
	// Note: we assume that each key in [keys] is unique, so that iterating over
	// the keys will not produce duplicated nonces in the returned EVMInput slice.
	for _, key := range keys {
		if amount == 0 {
			break
		}
		addr := key.EthAddress()
		var balance uint64
		if assetID == ctx.LUXAssetID {
			// If the asset is LUX, we divide by the x2cRate to convert back to the correct
			// denomination of LUX that can be exported.
			balance = new(uint256.Int).Div(state.GetBalance(addr), X2CRate).Uint64()
		} else {
			balance = state.GetBalanceMultiCoin(addr, common.Hash(assetID)).Uint64()
		}
		if balance == 0 {
			continue
		}
		if amount < balance {
			balance = amount
		}
		nonce := state.GetNonce(addr)

		inputs = append(inputs, EVMInput{
			Address: addr,
			Amount:  balance,
			AssetID: assetID,
			Nonce:   nonce,
		})
		signers = append(signers, []*secp256k1.PrivateKey{key})
		amount -= balance
	}

	if amount > 0 {
		return nil, nil, errInsufficientFunds
	}

	return inputs, signers, nil
}

// GetSpendableLUXWithFee returns a list of EVMInputs and keys (in corresponding
// order) to total [amount] + [fee] of [LUX] owned by [keys].
// This function accounts for the added cost of the additional inputs needed to
// create the transaction and makes sure to skip any keys with a balance that is
// insufficient to cover the additional fee.
// Note: we return [][]*secp256k1.PrivateKey even though each input
// corresponds to a single key, so that the signers can be passed in to
// [tx.Sign] which supports multiple keys on a single input.
func GetSpendableLUXWithFee(
	ctx *consensus.Context,
	state StateDB,
	keys []*secp256k1.PrivateKey,
	amount uint64,
	cost uint64,
	baseFee *big.Int,
) ([]EVMInput, [][]*secp256k1.PrivateKey, error) {
	initialFee, err := CalculateDynamicFee(cost, baseFee)
	if err != nil {
		return nil, nil, err
	}

	newAmount, err := math.Add64(amount, initialFee)
	if err != nil {
		return nil, nil, err
	}
	amount = newAmount

	inputs := []EVMInput{}
	signers := [][]*secp256k1.PrivateKey{}
	// Note: we assume that each key in [keys] is unique, so that iterating over
	// the keys will not produce duplicated nonces in the returned EVMInput slice.
	for _, key := range keys {
		if amount == 0 {
			break
		}

		prevFee, err := CalculateDynamicFee(cost, baseFee)
		if err != nil {
			return nil, nil, err
		}

		newCost := cost + EVMInputGas
		newFee, err := CalculateDynamicFee(newCost, baseFee)
		if err != nil {
			return nil, nil, err
		}

		additionalFee := newFee - prevFee

		addr := key.EthAddress()
		// Since the asset is LUX, we divide by the x2cRate to convert back to
		// the correct denomination of LUX that can be exported.
		balance := new(uint256.Int).Div(state.GetBalance(addr), X2CRate).Uint64()
		// If the balance for [addr] is insufficient to cover the additional cost
		// of adding an input to the transaction, skip adding the input altogether
		if balance <= additionalFee {
			continue
		}

		// Update the cost for the next iteration
		cost = newCost

		newAmount, err := math.Add64(amount, additionalFee)
		if err != nil {
			return nil, nil, err
		}
		amount = newAmount

		// Use the entire [balance] as an input, but if the required [amount]
		// is less than the balance, update the [inputAmount] to spend the
		// minimum amount to finish the transaction.
		inputAmount := balance
		if amount < balance {
			inputAmount = amount
		}
		nonce := state.GetNonce(addr)

		inputs = append(inputs, EVMInput{
			Address: addr,
			Amount:  inputAmount,
			AssetID: ctx.LUXAssetID,
			Nonce:   nonce,
		})
		signers = append(signers, []*secp256k1.PrivateKey{key})
		amount -= inputAmount
	}

	if amount > 0 {
		return nil, nil, errInsufficientFunds
	}

	return inputs, signers, nil
}
