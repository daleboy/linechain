package memopool

import (
	"encoding/hex"
	"sync"

	blockchain "linechain/core"
)

// MemoPool 交易内存池数据结构
type MemoPool struct {
	Pending map[string]blockchain.Transaction//挂起的交易队列
	Queued  map[string]blockchain.Transaction//排队的交易队列
	Wg      sync.WaitGroup
}

// Move 将交易从一个队列中移到另外一个队列
func (memo *MemoPool) Move(tnx blockchain.Transaction, to string) {
	if to == "pending" {
		memo.Remove(hex.EncodeToString(tnx.ID), "queued")
		memo.Pending[hex.EncodeToString(tnx.ID)] = tnx
	}

	if to == "queued" {
		memo.Remove(hex.EncodeToString(tnx.ID), "pending")
		memo.Queued[hex.EncodeToString(tnx.ID)] = tnx
	}
}

// Add 添加新的交易到交易内存池
func (memo *MemoPool) Add(tnx blockchain.Transaction) {
	memo.Pending[hex.EncodeToString(tnx.ID)] = tnx
}

// Remove从某个队列中删除交易
func (memo *MemoPool) Remove(txID string, from string) {
	if from == "queued" {
		delete(memo.Queued, txID)
		return
	}

	if from == "pending" {
		delete(memo.Pending, txID)
		return
	}
}

// GetTransactions 从挂起交易队列中得到指定数量的交易
func (memo *MemoPool) GetTransactions(count int) (txs [][]byte) {
	i := 0
	for _, tx := range memo.Pending {
		txs = append(txs, tx.ID)
		if i == count {
			break
		}
		i++
	}
	return txs
}

// RemoveFromAll 从挂起和排队队列中全部删除某个交易
func (memo *MemoPool) RemoveFromAll(txID string) {
	delete(memo.Queued, txID)
	delete(memo.Pending, txID)
}

// ClearAll 从内存池中清除全部的交易
func (memo *MemoPool) ClearAll() {
	memo.Pending = map[string]blockchain.Transaction{}
	memo.Queued = map[string]blockchain.Transaction{}
}
