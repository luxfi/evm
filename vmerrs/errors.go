package vmerrs

import "errors"

// Common VM errors
var (
    ErrInvalidJump = errors.New("invalid jump")
    ErrOutOfGas = errors.New("out of gas")
    ErrExecutionReverted = errors.New("execution reverted")
    ErrWriteProtection = errors.New("write protection")
    ErrInsufficientBalance = errors.New("insufficient balance")
    ErrDepth = errors.New("max call depth exceeded")
    ErrAddrProhibited = errors.New("address prohibited")
    ErrMaxCodeSizeExceeded = errors.New("max code size exceeded")
    ErrInvalidCode = errors.New("invalid code")
    ErrCodeStoreOutOfGas = errors.New("contract creation code storage out of gas")
    ErrNonceUintOverflow = errors.New("nonce uint64 overflow")
    ErrContractAddressCollision = errors.New("contract address collision")
    ErrGasUintOverflow = errors.New("gas uint64 overflow")
    ErrReturnDataOutOfBounds = errors.New("return data out of bounds")
)
