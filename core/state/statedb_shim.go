//go:build evm_shims

package state

import "github.com/luxfi/geth/common"

func (db *StateDB) IntermediateRoot(deleteEmptyObjects bool) common.Hash {
    // Call the embedded StateDB's method with the correct signature
    // go-ethereum 1.13 changed the signature to only take deleteEmptyObjects
    return db.StateDB.IntermediateRoot(deleteEmptyObjects)
}