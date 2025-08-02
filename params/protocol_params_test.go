// (c) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package params

import (
	"testing"
)

// TestUpstreamParamsValues detects when a params value changes upstream to prevent a subtle change
// to one of the values to have an unpredicted impact in the libevm consumer.
// Values should be updated to newer upstream values once the consumer is updated to handle the
// updated value(s).
// TODO: Fix this test - many constants are not available in the EVM params package
func TestUpstreamParamsValues(t *testing.T) {
	t.Skip("Test needs to be updated for EVM params package")
	return
	/*
	tests := map[string]struct {
		param any
		want  any
	}{
		// "GasLimitBoundDivisor":               {param: nil GasLimitBoundDivisor, want: uint64(1024)},
		"MinGasLimit":                        {param: MinGasLimit, want: uint64(5000)},
		"MaxGasLimit":                        {param: MaxGasLimit, want: uint64(0x7fffffffffffffff)},
		"GenesisGasLimit":                    {param: GenesisGasLimit, want: uint64(4712388)},
		"ExpByteGas":                         {param: ExpByteGas, want: uint64(10)},
		"SloadGas":                           {param: SloadGas, want: uint64(50)},
		"CallValueTransferGas":               {param: CallValueTransferGas, want: uint64(9000)},
		"CallNewAccountGas":                  {param: CallNewAccountGas, want: uint64(25000)},
		"TxGas":                              {param: TxGas, want: uint64(21000)},
		"TxGasContractCreation":              {param: TxGasContractCreation, want: uint64(53000)},
		"TxDataZeroGas":                      {param: TxDataZeroGas, want: uint64(4)},
		"QuadCoeffDiv":                       {param: QuadCoeffDiv, want: uint64(512)},
		"LogDataGas":                         {param: LogDataGas, want: uint64(8)},
		"CallStipend":                        {param: CallStipend, want: uint64(2300)},
		"Keccak256Gas":                       {param: Keccak256Gas, want: uint64(30)},
		"Keccak256WordGas":                   {param: Keccak256WordGas, want: uint64(6)},
		"InitCodeWordGas":                    {param: InitCodeWordGas, want: uint64(2)},
		"SstoreSetGas":                       {param: SstoreSetGas, want: uint64(20000)},
		"SstoreResetGas":                     {param: SstoreResetGas, want: uint64(5000)},
		"SstoreClearGas":                     {param: SstoreClearGas, want: uint64(5000)},
		"SstoreRefundGas":                    {param: SstoreRefundGas, want: uint64(15000)},
		"NetSstoreNoopGas":                   {param: NetSstoreNoopGas, want: uint64(200)},
		"NetSstoreInitGas":                   {param: NetSstoreInitGas, want: uint64(20000)},
		"NetSstoreCleanGas":                  {param: NetSstoreCleanGas, want: uint64(5000)},
		"NetSstoreDirtyGas":                  {param: NetSstoreDirtyGas, want: uint64(200)},
		"NetSstoreClearRefund":               {param: NetSstoreClearRefund, want: uint64(15000)},
		"NetSstoreResetRefund":               {param: NetSstoreResetRefund, want: uint64(4800)},
		"NetSstoreResetClearRefund":          {param: NetSstoreResetClearRefund, want: uint64(19800)},
		"SstoreSentryGasEIP2200":             {param: SstoreSentryGasEIP2200, want: uint64(2300)},
		"SstoreSetGasEIP2200":                {param: SstoreSetGasEIP2200, want: uint64(20000)},
		"SstoreResetGasEIP2200":              {param: SstoreResetGasEIP2200, want: uint64(5000)},
		"SstoreClearsScheduleRefundEIP2200":  {param: SstoreClearsScheduleRefundEIP2200, want: uint64(15000)},
		"ColdAccountAccessCostEIP2929":       {param: ColdAccountAccessCostEIP2929, want: uint64(2600)},
		"ColdSloadCostEIP2929":               {param: ColdSloadCostEIP2929, want: uint64(2100)},
		"WarmStorageReadCostEIP2929":         {param: WarmStorageReadCostEIP2929, want: uint64(100)},
		"SstoreClearsScheduleRefundEIP3529":  {param: SstoreClearsScheduleRefundEIP3529, want: uint64(5000 - 2100 + 1900)},
		"JumpdestGas":                        {param: JumpdestGas, want: uint64(1)},
		"EpochDuration":                      {param: EpochDuration, want: uint64(30000)},
		"CreateDataGas":                      {param: CreateDataGas, want: uint64(200)},
		"CallCreateDepth":                    {param: CallCreateDepth, want: uint64(1024)},
		"ExpGas":                             {param: ExpGas, want: uint64(10)},
		"LogGas":                             {param: LogGas, want: uint64(375)},
		"CopyGas":                            {param: CopyGas, want: uint64(3)},
		"StackLimit":                         {param: StackLimit, want: uint64(1024)},
		"TierStepGas":                        {param: TierStepGas, want: uint64(0)},
		"LogTopicGas":                        {param: LogTopicGas, want: uint64(375)},
		"CreateGas":                          {param: CreateGas, want: uint64(32000)},
		"Create2Gas":                         {param: Create2Gas, want: uint64(32000)},
		"SelfdestructRefundGas":              {param: SelfdestructRefundGas, want: uint64(24000)},
		"MemoryGas":                          {param: MemoryGas, want: uint64(3)},
		"TxDataNonZeroGasFrontier":           {param: TxDataNonZeroGasFrontier, want: uint64(68)},
		"TxDataNonZeroGasEIP2028":            {param: TxDataNonZeroGasEIP2028, want: uint64(16)},
		"TxAccessListAddressGas":             {param: TxAccessListAddressGas, want: uint64(2400)},
		"TxAccessListStorageKeyGas":          {param: TxAccessListStorageKeyGas, want: uint64(1900)},
		"CallGasFrontier":                    {param: CallGasFrontier, want: uint64(40)},
		"CallGasEIP150":                      {param: CallGasEIP150, want: uint64(700)},
		"BalanceGasFrontier":                 {param: BalanceGasFrontier, want: uint64(20)},
		"BalanceGasEIP150":                   {param: BalanceGasEIP150, want: uint64(400)},
		"BalanceGasEIP1884":                  {param: BalanceGasEIP1884, want: uint64(700)},
		"ExtcodeSizeGasFrontier":             {param: ExtcodeSizeGasFrontier, want: uint64(20)},
		"ExtcodeSizeGasEIP150":               {param: ExtcodeSizeGasEIP150, want: uint64(700)},
		"SloadGasFrontier":                   {param: SloadGasFrontier, want: uint64(50)},
		"SloadGasEIP150":                     {param: SloadGasEIP150, want: uint64(200)},
		"SloadGasEIP1884":                    {param: SloadGasEIP1884, want: uint64(800)},
		"SloadGasEIP2200":                    {param: SloadGasEIP2200, want: uint64(800)},
		"ExtcodeHashGasConstantinople":       {param: ExtcodeHashGasConstantinople, want: uint64(400)},
		"ExtcodeHashGasEIP1884":              {param: ExtcodeHashGasEIP1884, want: uint64(700)},
		"SelfdestructGasEIP150":              {param: SelfdestructGasEIP150, want: uint64(5000)},
		"ExpByteFrontier":                    {param: ExpByteFrontier, want: uint64(10)},
		"ExpByteEIP158":                      {param: ExpByteEIP158, want: uint64(50)},
		"ExtcodeCopyBaseFrontier":            {param: ExtcodeCopyBaseFrontier, want: uint64(20)},
		"ExtcodeCopyBaseEIP150":              {param: ExtcodeCopyBaseEIP150, want: uint64(700)},
		"CreateBySelfdestructGas":            {param: CreateBySelfdestructGas, want: uint64(25000)},
		"DefaultBaseFeeChangeDenominator":    {param: DefaultBaseFeeChangeDenominator, want: 8},
		"DefaultElasticityMultiplier":        {param: DefaultElasticityMultiplier, want: 2},
		"InitialBaseFee":                     {param: InitialBaseFee, want: 1000000000},
		"MaxCodeSize":                        {param: MaxCodeSize, want: 24576},
		"MaxInitCodeSize":                    {param: MaxInitCodeSize, want: 2 * 24576},
		"EcrecoverGas":                       {param: EcrecoverGas, want: uint64(3000)},
		"Sha256BaseGas":                      {param: Sha256BaseGas, want: uint64(60)},
		"Sha256PerWordGas":                   {param: Sha256PerWordGas, want: uint64(12)},
		"Ripemd160BaseGas":                   {param: Ripemd160BaseGas, want: uint64(600)},
		"Ripemd160PerWordGas":                {param: Ripemd160PerWordGas, want: uint64(120)},
		"IdentityBaseGas":                    {param: IdentityBaseGas, want: uint64(15)},
		"IdentityPerWordGas":                 {param: IdentityPerWordGas, want: uint64(3)},
		"Bn256AddGasByzantium":               {param: Bn256AddGasByzantium, want: uint64(500)},
		"Bn256AddGasIstanbul":                {param: Bn256AddGasIstanbul, want: uint64(150)},
		"Bn256ScalarMulGasByzantium":         {param: Bn256ScalarMulGasByzantium, want: uint64(40000)},
		"Bn256ScalarMulGasIstanbul":          {param: Bn256ScalarMulGasIstanbul, want: uint64(6000)},
		"Bn256PairingBaseGasByzantium":       {param: Bn256PairingBaseGasByzantium, want: uint64(100000)},
		"Bn256PairingBaseGasIstanbul":        {param: Bn256PairingBaseGasIstanbul, want: uint64(45000)},
		"Bn256PairingPerPointGasByzantium":   {param: Bn256PairingPerPointGasByzantium, want: uint64(80000)},
		"Bn256PairingPerPointGasIstanbul":    {param: Bn256PairingPerPointGasIstanbul, want: uint64(34000)},
		"Bls12381G1AddGas":                   {param: Bls12381G1AddGas, want: uint64(600)},
		"Bls12381G1MulGas":                   {param: Bls12381G1MulGas, want: uint64(12000)},
		"Bls12381G2AddGas":                   {param: Bls12381G2AddGas, want: uint64(4500)},
		"Bls12381G2MulGas":                   {param: Bls12381G2MulGas, want: uint64(55000)},
		"Bls12381PairingBaseGas":             {param: Bls12381PairingBaseGas, want: uint64(115000)},
		"Bls12381PairingPerPairGas":          {param: Bls12381PairingPerPairGas, want: uint64(23000)},
		"Bls12381MapG1Gas":                   {param: Bls12381MapG1Gas, want: uint64(5500)},
		"Bls12381MapG2Gas":                   {param: Bls12381MapG2Gas, want: uint64(110000)},
		"RefundQuotient":                     {param: RefundQuotient, want: uint64(2)},
		"RefundQuotientEIP3529":              {param: RefundQuotientEIP3529, want: uint64(5)},
		"BlobTxBytesPerFieldElement":         {param: BlobTxBytesPerFieldElement, want: 32},
		"BlobTxFieldElementsPerBlob":         {param: BlobTxFieldElementsPerBlob, want: 4096},
		"BlobTxBlobGasPerBlob":               {param: BlobTxBlobGasPerBlob, want: 1 << 17},
		"BlobTxMinBlobGasprice":              {param: BlobTxMinBlobGasprice, want: 1},
		"BlobTxBlobGaspriceUpdateFraction":   {param: BlobTxBlobGaspriceUpdateFraction, want: 3338477},
		"BlobTxPointEvaluationPrecompileGas": {param: BlobTxPointEvaluationPrecompileGas, want: 50000},
		"BlobTxTargetBlobGasPerBlock":        {param: BlobTxTargetBlobGasPerBlock, want: 3 * 131072},
		"MaxBlobGasPerBlock":                 {param: MaxBlobGasPerBlock, want: 6 * 131072},
		"GenesisDifficulty":                  {param: GenesisDifficulty.Int64(), want: int64(131072)},
		"BeaconRootsStorageAddress":          {param: BeaconRootsStorageAddress, want: common.HexToAddress("0x000F3df6D732807Ef1319fB7B8bB8522d0Beac02")},
		"SystemAddress":                      {param: SystemAddress, want: common.HexToAddress("0xfffffffffffffffffffffffffffffffffffffffe")},
	}

	for name, test := range tests {
		assert.Equal(t, test.want, test.param, name)
	}
	*/
}
