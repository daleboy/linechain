package blockchain

import (
	"bytes"
	"encoding/hex"

	badger "github.com/dgraph-io/badger"
	log "github.com/sirupsen/logrus"
)

var (
	utxoPrefix  = []byte("utxo-")//键值前缀
	prefiLength = len(utxoPrefix)
)

// UTXOSet unspent transaction outputs（未花费交易输出集合）
type UTXOSet struct {
	Blockchain *Blockchain
}

// FindSpendableOutputs 从数据库中找到足够的输入引用的未花费输出，为交易做准备
//从未花费交易里取出未花费的输出，直至取出输出的币总数大于或等于需要send的币数为止
func (u *UTXOSet) FindSpendableOutputs(pubKeyHash []byte, amount float64) (float64, map[string][]int) {
	unspentOuts := make(map[string][]int)
	accumulated := float64(0)

	db := u.Blockchain.Database

	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		// 要启用仅可以用键迭代，需要将IteratorOptions.PrefetchValues字段设置为false
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(utxoPrefix); it.ValidForPrefix(utxoPrefix); it.Next() {
			item := it.Item()
			k := item.KeyCopy(nil)

			v, err := item.ValueCopy(nil)
			Handle(err)
			outs := DeSerializeOutputs(v)

			k = bytes.TrimPrefix(k, utxoPrefix)
			txID := hex.EncodeToString(k)

			for outIdx, out := range outs.Outputs {
				if out.IsLockWithKey(pubKeyHash) && accumulated < amount {
					accumulated += out.Value
					unspentOuts[txID] = append(unspentOuts[txID], outIdx)
					if accumulated >= amount {//足够交易，停止继续取出
						break
					}
				}
			}
		}

		return nil
	})
	Handle(err)
	return accumulated, unspentOuts
}

// FindUnSpentTransactions 根据公钥哈希，得到所有UTXO(给出地址余额)
func (u UTXOSet) FindUnSpentTransactions(pubKeyHash []byte) []TxOutput {
	var UTXOs []TxOutput
	db := u.Blockchain.Database

	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(utxoPrefix); it.ValidForPrefix(utxoPrefix); it.Next() {
			item := it.Item()
			v, err := item.ValueCopy(nil)

			Handle(err)
			outs := DeSerializeOutputs(v)

			for _, out := range outs.Outputs {
				if out.IsLockWithKey(pubKeyHash) {
					UTXOs = append(UTXOs, out)
				}
			}
		}

		return nil
	})
	Handle(err)

	return UTXOs
}

// CountTransactions 从数据库的UTXO表中查找某个UTXO集合中交易的数量
func (u *UTXOSet) CountTransactions() int {
	db := u.Blockchain.Database
	counter := 0
	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek(utxoPrefix); it.ValidForPrefix(utxoPrefix); it.Next() {
			counter++
		}
		return nil
	})
	Handle(err)
	return counter
}

// Update 根据区块中的交易更新数据库的UTXO表和UTXOBlock表
// 该区块是区块链的Tip区块
//需要处理的问题是：由于奖励固定，对于同一挖矿人，coinbasetx的ID相同，因此更新时候，
//如果交易中包含coinbase，那么只能删除一个，不能把UTXO中的该ID对应的coinbase全部删了
func (u *UTXOSet) Update(block *Block) {
	db := u.Blockchain.Database
	err := db.Update(func(txn *badger.Txn) error {
		for _, tx := range block.Transactions {
			if tx.IsMinerTx() == false {//创始区块交易不含实质的输入，也就不对该交易的输入进行处理
				for _, in := range tx.Inputs {
					updatedOutputs := TxOutputs{}
					inID := append(utxoPrefix, in.ID...)
					item, err := txn.Get(inID)
					Handle(err)
					v, err := item.ValueCopy(nil)
					Handle(err)

					outs := DeSerializeOutputs(v)
					for outIdx, out := range outs.Outputs {
						if outIdx != in.Out {//如果UTXO中的输出不包含在当前交易中，保留到更新的UTXO集中
							updatedOutputs.Outputs = append(updatedOutputs.Outputs, out)
						}
					}
					if len(updatedOutputs.Outputs) == 0 {//如果更新的UTXO的元素个数为0，从UTXO集中删除它
						if err := txn.Delete(inID); err != nil {
							log.Panic(err)
						}
					} else {
						//如果更新的UTXO的元素个数不为0，更新UTXO
						if err := txn.Set(inID, updatedOutputs.Serialize()); err != nil {
							log.Panic(err)
						}
					}
				}
				newOutputs := TxOutputs{}
				for _, out := range tx.Outputs {
					//将新交易的输出加入到UTXO中
					newOutputs.Outputs = append(newOutputs.Outputs, out)
				}
				txID := append(utxoPrefix, tx.ID...)
				err := txn.Set(txID, newOutputs.Serialize())
				Handle(err)
			} else {//创始区块
				//为矿工（受益者）挖矿交易更新UXTO
				newOutputs := TxOutputs{}
				for _, out := range tx.Outputs {
					newOutputs.Outputs = append(newOutputs.Outputs, out)
				}
				txID := append(utxoPrefix, tx.ID...)
				err := txn.Set(txID, newOutputs.Serialize())
				Handle(err)
			}
		}
		return nil
	})

	Handle(err)
}

// 更新UTXOSet
func (u *UTXOSet) Compute() {
	db := u.Blockchain.Database

	u.DeleteByPrefix(utxoPrefix)

	UTXO := u.Blockchain.FindUTXO()

	err := db.Update(func(txn *badger.Txn) error {
		for txId, outs := range UTXO {
			key, err := hex.DecodeString(txId)
			Handle(err)

			key = append(utxoPrefix, key...)
			err = txn.Set(key, outs.Serialize())
			Handle(err)
		}
		return nil
	})

	Handle(err)
}

func (u *UTXOSet) DeleteByPrefix(prefix []byte) {
	deleteKeys := func(keysForDelete [][]byte) error {
		if err := u.Blockchain.Database.Update(func(txn *badger.Txn) error {
			for _, key := range keysForDelete {
				if err := txn.Delete(key); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
		return nil
	}
	
	// 这是badgerDB一次可以删除的最大记录数, 
	// 因此我们必须汇总所有带有utxo前缀的键值的记录并批量删除
	collectSize := 100000
	u.Blockchain.Database.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		keysForDelete := make([][]byte, 0, collectSize)
		keysCollected := 0
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().KeyCopy(nil)
			keysForDelete = append(keysForDelete, key)
			keysCollected++
			if keysCollected == collectSize {
				if err := deleteKeys(keysForDelete); err != nil {
					log.Panic(err)
				}
				// 复位keys，继续删除指定集合大小的记录
				keysForDelete = make([][]byte, 0, collectSize)
				keysCollected = 0
			}
		}

		if keysCollected > 0 {
			if err := deleteKeys(keysForDelete); err != nil {
				log.Panic(err)
			}
		}

		return nil
	})

}
