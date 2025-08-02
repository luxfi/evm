// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package syncutils

import (
	"github.com/luxfi/evm/v2/v2/core/state/snapshot"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/ethdb"
)

var (
	_ ethdb.Iterator = (*AccountIterator)(nil)
	_ ethdb.Iterator = (*StorageIterator)(nil)
)

// AccountIterator wraps a [snapshot.AccountIterator] to conform to [ethdb.Iterator]
// accounts will be returned in consensus (FullRLP) format for compatibility with trie data.
type AccountIterator struct {
	snapshot.AccountIterator
	err error
	val []byte
}

func (it *AccountIterator) Next() bool {
	if it.err != nil {
		return false
	}
	for it.AccountIterator.Next() {
		_, accountRLP := it.Account()
		it.val, it.err = types.FullAccountRLP(accountRLP)
		return it.err == nil
	}
	it.val = nil
	return false
}

func (it *AccountIterator) Key() []byte {
	if it.err != nil {
		return nil
	}
	hash, _ := it.Account()
	return hash.Bytes()
}

func (it *AccountIterator) Value() []byte {
	if it.err != nil {
		return nil
	}
	return it.val
}

func (it *AccountIterator) Error() error {
	if it.err != nil {
		return it.err
	}
	return it.AccountIterator.Error()
}

// StorageIterator wraps a [snapshot.StorageIterator] to conform to [ethdb.Iterator]
type StorageIterator struct {
	snapshot.StorageIterator
}

func (it *StorageIterator) Key() []byte {
	hash, _ := it.Slot()
	return hash.Bytes()
}

func (it *StorageIterator) Value() []byte {
	_, val := it.Slot()
	return val
}