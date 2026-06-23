// dexlive drives the 0x9999 native DEX value path end-to-end against a RUNNING
// localnet C-Chain over JSON-RPC. It deploys two REAL compiled ERC-20s (LETH base,
// LUSD quote), opens a market permissionlessly (initialize), seeds a real maker bid
// (swapDeposit + swapPlace), executes a taker swap through 0x9999 (the synchronous
// value path), and reads the DEXFill event + balance deltas from ACCEPTED chain state.
// It also runs the synthetic/no-code negative control (must revert, no debit).
//
// Every leg is a real signed EVM transaction mined into the chain — no mocks.
package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	dex "github.com/luxfi/precompile/dex"

	"github.com/luxfi/crypto"
	ethereum "github.com/luxfi/geth"
	"github.com/luxfi/geth/accounts/abi"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/ethclient"
)

// toGeth converts a luxfi/crypto address ([20]byte) to a geth/common.Address.
func toGeth(a crypto.Address) common.Address { return common.BytesToAddress(a[:]) }

func ethCallMsg(to common.Address, data []byte) ethereum.CallMsg {
	return ethereum.CallMsg{To: &to, Data: data}
}

func filterQuery(addr common.Address, sig common.Hash) ethereum.FilterQuery {
	return ethereum.FilterQuery{
		FromBlock: big.NewInt(0),
		Addresses: []common.Address{addr},
		Topics:    [][]common.Hash{{sig}},
	}
}

const (
	rpcURL         = "http://127.0.0.1:9660/ext/bc/C/rpc"
	priceMultiCnst = 100000000 // lx.PriceMultiplier
)

// RealToken: a real solc-0.8.35-compiled ERC-20 whose constructor mints the supply to
// the deployer and exposes mint/transfer/transferFrom/approve/balanceOf. Bytecode is read
// at runtime from the solc artifact at /tmp/rt/RealToken.bin (source /tmp/RealToken.sol).
// It has genuine deployed code (EXTCODESIZE>0), so the 0x9999 verifier admits it.
const realTokenBinPath = "/tmp/rt/RealToken.bin"

const ggTokenABI = `[
{"inputs":[{"name":"_name","type":"string"},{"name":"_symbol","type":"string"},{"name":"_supply","type":"uint256"}],"stateMutability":"nonpayable","type":"constructor"},
{"inputs":[{"name":"spender","type":"address"},{"name":"amount","type":"uint256"}],"name":"approve","outputs":[{"name":"","type":"bool"}],"stateMutability":"nonpayable","type":"function"},
{"inputs":[{"name":"","type":"address"}],"name":"balanceOf","outputs":[{"name":"","type":"uint256"}],"stateMutability":"view","type":"function"},
{"inputs":[{"name":"to","type":"address"},{"name":"amount","type":"uint256"}],"name":"transfer","outputs":[{"name":"","type":"bool"}],"stateMutability":"nonpayable","type":"function"},
{"inputs":[{"name":"from","type":"address"},{"name":"to","type":"address"},{"name":"amount","type":"uint256"}],"name":"transferFrom","outputs":[{"name":"","type":"bool"}],"stateMutability":"nonpayable","type":"function"},
{"inputs":[{"name":"to","type":"address"},{"name":"amount","type":"uint256"}],"name":"mint","outputs":[],"stateMutability":"nonpayable","type":"function"}
]` 

var the9999 = common.HexToAddress(dex.DEXPoolManagerAddress)

type wallet struct {
	key  *ecdsa.PrivateKey
	addr common.Address
}

func mustKey(hexkey string) wallet {
	k, err := crypto.HexToECDSA(strings.TrimPrefix(hexkey, "0x"))
	if err != nil {
		panic(err)
	}
	return wallet{key: k, addr: toGeth(crypto.PubkeyToAddress(k.PublicKey))}
}

type harness struct {
	ctx     context.Context
	cl      *ethclient.Client
	chainID *big.Int
	ercABI  abi.ABI
}

func (h *harness) nonce(w wallet) uint64 {
	n, err := h.cl.PendingNonceAt(h.ctx, w.addr)
	if err != nil {
		panic(err)
	}
	return n
}

// send signs and broadcasts a tx, waits for the receipt, returns it.
func (h *harness) send(w wallet, to *common.Address, value *big.Int, data []byte, gas uint64, label string) *types.Receipt {
	gp, err := h.cl.SuggestGasPrice(h.ctx)
	if err != nil {
		panic(err)
	}
	// bump gas price so the dynamic-fee floor is always met on a fresh chain
	gp = new(big.Int).Add(gp, big.NewInt(25_000_000_000))
	n := h.nonce(w)
	var tx *types.Transaction
	if to == nil {
		tx = types.NewContractCreation(n, value, gas, gp, data)
	} else {
		tx = types.NewTransaction(n, *to, value, gas, gp, data)
	}
	signed, err := types.SignTx(tx, types.LatestSignerForChainID(h.chainID), w.key)
	if err != nil {
		panic(err)
	}
	if err := h.cl.SendTransaction(h.ctx, signed); err != nil {
		fmt.Printf("    [%s] send error: %v\n", label, err)
		return nil
	}
	rcpt := h.wait(signed.Hash(), label)
	return rcpt
}

func (h *harness) wait(hash common.Hash, label string) *types.Receipt {
	for i := 0; i < 60; i++ {
		r, err := h.cl.TransactionReceipt(h.ctx, hash)
		if err == nil && r != nil {
			return r
		}
		time.Sleep(250 * time.Millisecond)
	}
	fmt.Printf("    [%s] no receipt for %s after 15s\n", label, hash.Hex())
	return nil
}

func (h *harness) deployERC20(deployer wallet, name, symbol string, supply *big.Int, label string) common.Address {
	binHex, err := os.ReadFile(realTokenBinPath)
	if err != nil {
		panic(fmt.Sprintf("read %s: %v", realTokenBinPath, err))
	}
	creation := common.FromHex(strings.TrimSpace(string(binHex)))
	// ABI-encode constructor args (string,string,uint256) and append to the creation code.
	ctorArgs, err := h.ercABI.Constructor.Inputs.Pack(name, symbol, supply)
	if err != nil {
		panic(fmt.Sprintf("pack ctor: %v", err))
	}
	rcpt := h.send(deployer, nil, big.NewInt(0), append(creation, ctorArgs...), 1_500_000, "deploy "+label)
	if rcpt == nil || rcpt.Status != 1 {
		panic(fmt.Sprintf("deploy %s failed (status=%v)", label, statusOf(rcpt)))
	}
	code, _ := h.cl.CodeAt(h.ctx, rcpt.ContractAddress, nil)
	fmt.Printf("    %s (%s) deployed at %s  tx=%s  block=%d  codeLen=%d\n",
		label, symbol, rcpt.ContractAddress.Hex(), rcpt.TxHash.Hex(), rcpt.BlockNumber.Uint64(), len(code))
	return rcpt.ContractAddress
}

func (h *harness) erc20Transfer(from wallet, token, to common.Address, amount *big.Int, label string) {
	data, _ := h.ercABI.Pack("transfer", to, amount)
	r := h.send(from, &token, big.NewInt(0), data, 120_000, "transfer "+label)
	if r == nil || r.Status != 1 {
		panic("transfer " + label + " failed")
	}
}

func (h *harness) erc20Approve(owner wallet, token, spender common.Address, amount *big.Int, label string) {
	data, _ := h.ercABI.Pack("approve", spender, amount)
	r := h.send(owner, &token, big.NewInt(0), data, 120_000, "approve "+label)
	if r == nil || r.Status != 1 {
		panic("approve " + label + " failed")
	}
}

func (h *harness) erc20BalanceOf(token, who common.Address) *big.Int {
	data, _ := h.ercABI.Pack("balanceOf", who)
	out, err := h.cl.CallContract(h.ctx, ethCallMsg(token, data), nil)
	if err != nil {
		panic(err)
	}
	res, err := h.ercABI.Unpack("balanceOf", out)
	if err != nil {
		panic(err)
	}
	return res[0].(*big.Int)
}

func (h *harness) getCode(a common.Address) []byte {
	c, _ := h.cl.CodeAt(h.ctx, a, nil)
	return c
}

// ---- 0x9999 calldata builders (mirror precompile/dex/swap_sync_e2e_test.go) ----

func poolKey(base, quote common.Address) dex.PoolKey {
	return dex.PoolKey{
		Currency0:   dex.Currency{Address: base},  // currency0 must be < currency1
		Currency1:   dex.Currency{Address: quote},
		Fee:         3000,
		TickSpacing: 60,
		Hooks:       common.Address{},
	}
}

func selPrefix(sel uint32, body []byte) []byte {
	out := make([]byte, 4+len(body))
	out[0] = byte(sel >> 24)
	out[1] = byte(sel >> 16)
	out[2] = byte(sel >> 8)
	out[3] = byte(sel)
	copy(out[4:], body)
	return out
}

func swapDepositCalldata(token common.Address, amount uint64) []byte {
	data := make([]byte, 64)
	copy(data[12:32], token.Bytes())
	new(big.Int).SetUint64(amount).FillBytes(data[32:64])
	return selPrefix(dex.SelectorSwapDeposit, data)
}

func swapPlaceCalldata(key dex.PoolKey, isBid bool, priceGrid, size uint64) []byte {
	data := make([]byte, 256)
	copy(data[0:160], dex.EncodePoolKeyABI(key))
	if isBid {
		data[191] = 1
	}
	new(big.Int).SetUint64(priceGrid).FillBytes(data[192:224])
	new(big.Int).SetUint64(size).FillBytes(data[224:256])
	return selPrefix(dex.SelectorSwapPlace, data)
}

// swapCalldata builds a V4 swap with an optional hookData blob.
func swapCalldata(key dex.PoolKey, zeroForOne bool, amountIn int64, hookData []byte) []byte {
	body := make([]byte, 256)
	copy(body[0:160], dex.EncodePoolKeyABI(key))
	if zeroForOne {
		body[191] = 1
	}
	// amountSpecified = -amountIn (exact input), two's complement int256
	amt := big.NewInt(-amountIn)
	tc := new(big.Int).Add(new(big.Int).Lsh(big.NewInt(1), 256), amt)
	tc.FillBytes(body[192:224])
	// sqrtPriceLimitX96 = 0 (min-out hookData is the protection)
	if len(hookData) > 0 {
		// ABI-encode trailing dynamic bytes: offset(=256) at [224:256], then len+data appended
		new(big.Int).SetUint64(256).FillBytes(body[224:256])
		hd := make([]byte, 32+((len(hookData)+31)/32)*32)
		new(big.Int).SetUint64(uint64(len(hookData))).FillBytes(hd[0:32])
		copy(hd[32:], hookData)
		body = append(body, hd...)
	}
	return selPrefix(dex.SelectorSwap, body)
}

func initializeCalldata(key dex.PoolKey, sqrtPriceX96 *big.Int) []byte {
	body := make([]byte, 192)
	copy(body[0:160], dex.EncodePoolKeyABI(key))
	sqrtPriceX96.FillBytes(body[160:192])
	return selPrefix(dex.SelectorInitialize, body)
}

func statusOf(r *types.Receipt) interface{} {
	if r == nil {
		return "nil"
	}
	return r.Status
}

func main() {
	ctx := context.Background()
	cl, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		fmt.Println("dial:", err)
		os.Exit(1)
	}
	chainID, err := cl.ChainID(ctx)
	if err != nil {
		fmt.Println("chainID:", err)
		os.Exit(1)
	}
	ercABI, err := abi.JSON(strings.NewReader(ggTokenABI))
	if err != nil {
		panic(err)
	}
	h := &harness{ctx: ctx, cl: cl, chainID: chainID, ercABI: ercABI}

	// Three funded accounts from the LIGHT_MNEMONIC alloc (genesis-funded, 500M LUX each).
	deployer := mustKey("e0e748a7cde1caf0f5e2324aabe6c81fbcef1968e654eb38f8e4798ab966188b") // idx47 0x01e2...
	maker := mustKey("c827aac8f0d6c9f705dccdf823a0deab20c6534bbc1da3bfc2c4ee819d7f9774")    // idx24 0x0695...
	taker := mustKey("4a11c618bc0923e5abb221063b459c68eed8024933a4e0f568244a84a004b5f4")    // idx?  derive

	fmt.Printf("chainID(eth)=%s  deployer=%s\n", chainID, deployer.addr.Hex())
	bn, _ := cl.BlockNumber(ctx)
	fmt.Printf("block=%d at start\n\n", bn)

	// STEP 3: getCode(0x9999) from accepted state.
	fmt.Println("== STEP 3: 0x9999 bytecode from boot ==")
	fmt.Printf("    getCode(0x9999) = 0x%x\n", h.getCode(the9999))
	fmt.Printf("    getCode(0x9996) = 0x%x\n\n", h.getCode(common.HexToAddress("0x0000000000000000000000000000000000009996")))

	// STEP 4: deploy two REAL ERC-20s; order them by BYTES so currency0 < currency1.
	fmt.Println("== STEP 4: deploy real ERC-20s (LETH base, LUSD quote) ==")
	supply := new(big.Int).Mul(big.NewInt(1_000_000_000), big.NewInt(1e18)) // 1e9 tokens
	tokA := h.deployERC20(deployer, "Lux Ether", "LETH", supply, "ERC20-A")
	tokB := h.deployERC20(deployer, "Lux USD", "LUSD", supply, "ERC20-B")
	leth, lusd := tokA, tokB
	if bytes.Compare(leth.Bytes(), lusd.Bytes()) > 0 { // V4 invariant: currency0 < currency1 (byte order)
		leth, lusd = lusd, leth
	}
	fmt.Printf("    LETH (currency0/base)  = %s\n", leth.Hex())
	fmt.Printf("    LUSD (currency1/quote) = %s\n", lusd.Hex())
	// Fund maker with LUSD (quote, to back a bid) and taker with LETH (base, to sell).
	// dexcore amounts are integer units; transfer generous balances.
	fund := new(big.Int).Mul(big.NewInt(1_000_000), big.NewInt(1e18))
	h.erc20Transfer(deployer, lusd, maker.addr, fund, "LUSD->maker")
	h.erc20Transfer(deployer, leth, taker.addr, fund, "LETH->taker")
	fmt.Printf("    maker LUSD balance = %s ; taker LETH balance = %s\n\n",
		h.erc20BalanceOf(lusd, maker.addr), h.erc20BalanceOf(leth, taker.addr))

	key := poolKey(leth, lusd)

	// STEP 5: permissionless OpenMarket via initialize (sqrtPriceX96 = 1<<96, price=1.0).
	fmt.Println("== STEP 5: permissionless OpenMarket (initialize) for LETH/LUSD ==")
	sqrt1 := new(big.Int).Lsh(big.NewInt(1), 96) // 1.0 in Q64.96
	rInit := h.send(deployer, &the9999, big.NewInt(0), initializeCalldata(key, sqrt1), 3_000_000, "initialize")
	fmt.Printf("    initialize status=%v tx=%s block=%d gasUsed=%d\n\n",
		statusOf(rInit), txhash(rInit), blockOf(rInit), gasOf(rInit))

	// STEP 6: seed a real maker BID, then taker swap (sync value path).
	fmt.Println("== STEP 6: maker rests real-funded BID, taker swaps through 0x9999 ==")
	// Maker: approve 0x9999 to pull LUSD, deposit 5000 LUSD into the vault, place bid 100 @ 50.
	h.erc20Approve(maker, lusd, the9999, big.NewInt(1_000_000), "maker LUSD->9999")
	rDep := h.send(maker, &the9999, big.NewInt(0), swapDepositCalldata(lusd, 5000), 2_000_000, "maker swapDeposit LUSD")
	fmt.Printf("    maker swapDeposit(LUSD,5000) status=%v block=%d\n", statusOf(rDep), blockOf(rDep))
	rPlace := h.send(maker, &the9999, big.NewInt(0), swapPlaceCalldata(key, true, 50*priceMultiCnst, 100), 2_000_000, "maker swapPlace bid")
	fmt.Printf("    maker swapPlace(bid, price=50, size=100) status=%v block=%d\n", statusOf(rPlace), blockOf(rPlace))

	// Taker: approve + deposit LETH, then SELL 80 LETH with an explicit min-out floor (DM01).
	h.erc20Approve(taker, leth, the9999, big.NewInt(1_000_000), "taker LETH->9999")
	rTDep := h.send(taker, &the9999, big.NewInt(0), swapDepositCalldata(leth, 80), 2_000_000, "taker swapDeposit LETH")
	fmt.Printf("    taker swapDeposit(LETH,80) status=%v block=%d\n", statusOf(rTDep), blockOf(rTDep))

	lethTakerBefore := h.erc20BalanceOf(leth, taker.addr)
	lusdTakerBefore := h.erc20BalanceOf(lusd, taker.addr)

	minOut := uint64(1) // floor; the realized out (~4000 LUSD for 80 LETH @ 50) far exceeds it
	rSwap := h.send(taker, &the9999, big.NewInt(0), swapCalldata(key, true, 80, dex.EncodeMinOutHookData(minOut)), 3_000_000, "taker swap SELL 80 LETH")
	fmt.Printf("    taker swap(SELL 80 LETH, minOut=%d) status=%v tx=%s block=%d gasUsed=%d\n",
		minOut, statusOf(rSwap), txhash(rSwap), blockOf(rSwap), gasOf(rSwap))

	// STEP 7: read DEXFill from accepted state + balance deltas.
	fmt.Println("\n== STEP 7: DEXFill from accepted state + balance deltas ==")
	dexFillSig := common.BytesToHash(crypto.Keccak256([]byte("DEXFill(bytes32,address,uint256,uint256)")))
	if rSwap != nil {
		found := false
		for _, lg := range rSwap.Logs {
			if lg.Address == the9999 && len(lg.Topics) >= 3 && lg.Topics[0] == dexFillSig {
				poolID := lg.Topics[1]
				takerTopic := common.BytesToAddress(lg.Topics[2].Bytes())
				amountOut := new(big.Int).SetBytes(lg.Data[0:32])
				blockNum := new(big.Int).SetBytes(lg.Data[32:64])
				fmt.Printf("    DEXFill: poolID=%s taker=%s amountOut=%s blockNumber=%s (addr=%s)\n",
					poolID.Hex(), takerTopic.Hex(), amountOut, blockNum, lg.Address.Hex())
				found = true
			}
		}
		if !found {
			fmt.Printf("    no DEXFill in swap receipt (logs=%d); dumping topics:\n", len(rSwap.Logs))
			for _, lg := range rSwap.Logs {
				fmt.Printf("      addr=%s topic0=%s\n", lg.Address.Hex(), topic0(lg))
			}
		}
	}
	lethTakerAfter := h.erc20BalanceOf(leth, taker.addr)
	lusdTakerAfter := h.erc20BalanceOf(lusd, taker.addr)
	makerLusdVault := h.erc20BalanceOf(lusd, the9999)
	fmt.Printf("    taker LETH: before=%s after=%s (Δ=%s)\n", lethTakerBefore, lethTakerAfter, new(big.Int).Sub(lethTakerAfter, lethTakerBefore))
	fmt.Printf("    taker LUSD: before=%s after=%s (Δ=%s)\n", lusdTakerBefore, lusdTakerAfter, new(big.Int).Sub(lusdTakerAfter, lusdTakerBefore))
	fmt.Printf("    0x9999 vault LUSD balance = %s ; LETH balance = %s\n", makerLusdVault, h.erc20BalanceOf(leth, the9999))

	// eth_getLogs cross-check from accepted state (independent of the receipt).
	fmt.Println("\n    eth_getLogs(0x9999, DEXFill) over accepted range:")
	logs := h.getDexFillLogs(dexFillSig)
	for _, lg := range logs {
		amountOut := new(big.Int).SetBytes(lg.Data[0:32])
		fmt.Printf("      block=%d tx=%s poolID=%s taker=%s amountOut=%s\n",
			lg.BlockNumber, lg.TxHash.Hex(), lg.Topics[1].Hex(),
			common.BytesToAddress(lg.Topics[2].Bytes()).Hex(), amountOut)
	}
	if len(logs) == 0 {
		fmt.Println("      (none)")
	}

	// STEP 8: negative control — synthetic/no-code asset must REVERT, no debit.
	fmt.Println("\n== STEP 8: negative control — synthetic/no-code asset must REVERT ==")
	synthetic := common.BytesToAddress([]byte("LUSD")) // ASCII 'LUSD' left-padded -> no contract code
	fmt.Printf("    synthetic asset addr = %s  code = 0x%x\n", synthetic.Hex(), h.getCode(synthetic))
	synthKey := poolKey(leth, synthetic)
	if leth.Hex() > synthetic.Hex() {
		synthKey = poolKey(synthetic, leth)
	}
	takerLethBeforeNeg := h.erc20BalanceOf(leth, taker.addr)
	// Try to OpenMarket the synthetic pair, and to swap into it; both must fail closed.
	rNegInit := h.send(deployer, &the9999, big.NewInt(0), initializeCalldata(synthKey, sqrt1), 3_000_000, "initialize SYNTHETIC")
	fmt.Printf("    initialize(SYNTHETIC) status=%v (want 0=revert) block=%d\n", statusOf(rNegInit), blockOf(rNegInit))
	rNegSwap := h.send(taker, &the9999, big.NewInt(0), swapCalldata(synthKey, true, 10, dex.EncodeMinOutHookData(1)), 3_000_000, "swap SYNTHETIC")
	fmt.Printf("    swap(SYNTHETIC) status=%v (want 0=revert) block=%d\n", statusOf(rNegSwap), blockOf(rNegSwap))
	takerLethAfterNeg := h.erc20BalanceOf(leth, taker.addr)
	fmt.Printf("    taker LETH around synthetic attempts: before=%s after=%s (Δ=%s, want 0 = no debit)\n",
		takerLethBeforeNeg, takerLethAfterNeg, new(big.Int).Sub(takerLethAfterNeg, takerLethBeforeNeg))

	fmt.Println("\nDONE")
}

func (h *harness) getDexFillLogs(sig common.Hash) []types.Log {
	q := map[string]interface{}{}
	_ = q
	// Use FilterLogs via ethclient
	logs, err := h.cl.FilterLogs(h.ctx, filterQuery(the9999, sig))
	if err != nil {
		fmt.Println("    FilterLogs error:", err)
		return nil
	}
	return logs
}

func topic0(lg *types.Log) string {
	if len(lg.Topics) == 0 {
		return "(none)"
	}
	return lg.Topics[0].Hex()
}
func txhash(r *types.Receipt) string {
	if r == nil {
		return "nil"
	}
	return r.TxHash.Hex()
}
func blockOf(r *types.Receipt) uint64 {
	if r == nil || r.BlockNumber == nil {
		return 0
	}
	return r.BlockNumber.Uint64()
}
func gasOf(r *types.Receipt) uint64 {
	if r == nil {
		return 0
	}
	return r.GasUsed
}
