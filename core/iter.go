package blockchain

import badger "github.com/dgraph-io/badger"

// BlockchainIterator 区块链迭代器结构
// 区块之间通过前一个区块的哈希进行连接，因此也是通过前一个区块的哈希进行迭代计算
type BlockchainIterator struct {
	CurrentHash []byte
	Database    *badger.DB
}

func (chain *Blockchain) Iterator() *BlockchainIterator {
	if chain.LastHash == nil {
		return nil
	}
	return &BlockchainIterator{chain.LastHash, chain.Database}
}

func (iter *BlockchainIterator) Next() *Block {
	var block *Block
	var encodedBlock []byte

	//读操作
	err := iter.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get(iter.CurrentHash)//根据CurrentHash得到当前区块
		Handle(err)
		encodedBlock, err = item.ValueCopy(nil)
		block = DeSerialize(encodedBlock)
		return err
	})
	Handle(err)

	iter.CurrentHash = block.PrevHash//更新CurrentHash为当前区块的PreHash值：区块迭代计算是从后往前计算的
	return block
}
