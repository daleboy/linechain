package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	badger "github.com/dgraph-io/badger"
	log "github.com/sirupsen/logrus"
)

// Blockchain 区块链数据结构
// 我们不在里面存储所有的区块了，而是仅存储区块链的lastHash，它代表账本上的最后一个区块的哈希
// 另外，我们存储了一个数据库连接。因为我们想要一旦打开它的话，就让它一直运行，直到程序运行结束
type Blockchain struct {
	LastHash   []byte
	Database   *badger.DB
	InstanceId string
}

var (
	mutex      = &sync.Mutex{}
	_, b, _, _ = runtime.Caller(0)

	// 项目的根目录
	Root        = filepath.Join(filepath.Dir(b), "../")
	genesisData = "genesis"
)

// DBExists 检查区块链数据库是否存在
func DBExists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}
	return true
}

// Exists 根据实例ID检查对应的数据库是否存在
func Exists(instanceId string) bool {
	return DBExists(GetDatabasePath(instanceId))
}
// GetDatabasePath 根据instanceId得到数据库目录
func GetDatabasePath(instanceId string) string {
	sysType := runtime.GOOS

	if sysType == "linux" {
		if instanceId != "" {
			return filepath.Join(Root, fmt.Sprintf("./tmp/blocks_%s", instanceId))
		}
		return filepath.Join(Root, "./tmp/blocks")
	} else if sysType == "windows" {
		if instanceId != "" {
			return filepath.Join(Root, fmt.Sprintf("/tmp/blocks_%s", instanceId))
		}
		return filepath.Join(Root, "/tmp/blocks")
	}else{
		if instanceId != "" {
			return filepath.Join(Root, fmt.Sprintf("./tmp/blocks_%s", instanceId))
		}
		return filepath.Join(Root, "./tmp/blocks")
	}
}

// OpenBardgerDB 根据实例ID打开Bardger数据库
func OpenBardgerDB(instanceId string) (*badger.DB,error) {
	path := GetDatabasePath(instanceId)
	opts := badger.DefaultOptions(path)
	opts.ValueDir = path
	db, err := OpenDB(path, opts)
	Handle(err)

	return db,err
}

// ContinueBlockchain 从数据库中取出最后一个区块的哈希，构建一个区块链实例
func (chain *Blockchain) ContinueBlockchain() *Blockchain {
	var lastHash []byte
	var db *badger.DB
	if chain.Database == nil {
		db,_= OpenBardgerDB(chain.InstanceId)
	} else {
		db = chain.Database
	}

	//Read-Write Operations
	err := db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		if err == nil {
			lastHash, err = item.ValueCopy(nil)
		}

		return err
	})

	if err != nil {
		lastHash = nil
	}
	// log.Infof("LastHash: %x", lastHash)
	return &Blockchain{lastHash, db, chain.InstanceId}
}


// InitBlockchain 创建一个全新区块链
func InitBlockchain(address string, instanceId string) *Blockchain {
	var lastHash []byte
	path := GetDatabasePath(instanceId)

	if DBExists(path) {
		log.Info("Blockchain already exist")
		runtime.Goexit()
	}
	// 打开位于/tmp/blocks的Badger数据库
	// 如果数据库不存在将创建一个
	//badger数据库专为SSD硬盘设计，value和key分开存储，内存只有value的指针和key
	opts := badger.DefaultOptions(path)
	opts.ValueDir = path
	db, err := OpenDB(path, opts)
	Handle(err)

	//Read-Write 操作
	err = db.Update(func(txn *badger.Txn) error {
		cbtx := MinerTx(address, genesisData)
		log.Info("没有找到已经存在的区块链")//创建创始区块交易
		genesis := Genesis(cbtx)//挖出创始区块
		//将创始区块存入到本地数据库
		err = txn.Set(genesis.Hash, genesis.Serialize())
		Handle(err)
		//链最后一个节点key为"1h"，value是lastHash，存入数据库
		err = txn.Set([]byte("lh"), genesis.Hash)
		
		lastHash = genesis.Hash

		return err
	})
	Handle(err)

	return &Blockchain{lastHash, db, instanceId}
}

// AddBlock 将一个区块加入到区块链
func (chain *Blockchain) AddBlock(block *Block) *Block {
	mutex.Lock()//数据库锁

	//读-写操作
	err := chain.Database.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(block.Hash); err == nil {
			return nil//如果区块已经存在于数据库，直接返回（所以如果是来自本地的区块，不会再次加入）
		}

		blockData := block.Serialize()
		err := txn.Set(block.Hash, blockData)
		Handle(err)

		// 得到最后一个区块
		item, err := txn.Get([]byte("lh"))//最后一个区块的键值为“1h”
		if err == nil {
			lastHash, _ := item.ValueCopy(nil)
			item, err = txn.Get(lastHash)
			Handle(err)
			lastBlockData, _ := item.ValueCopy(nil)
			lastBlock := DeSerialize(lastBlockData)

			// 检查当前区块的height是否比lastBlock的大
			if block.Height > lastBlock.Height {
				err := txn.Set([]byte("lh"), block.Hash)//修改最后一个区块的hash
				Handle(err)
				chain.LastHash = block.Hash
			}
		} else {//如果数据库找不到最后一个区块，将当前区块设置为最后的区块（这种情况是存在的：某个本地数据库没有键值为1h的区块）
			err = txn.Set([]byte("lh"), block.Hash)
			chain.LastHash = block.Hash
		}

		return err
	})

	Handle(err)
	mutex.Unlock()
	return block
}

// 根据哈希值从区块链中得到一个区块
func (chain *Blockchain) GetBlock(blockHash []byte) (Block, error) {
	var block Block
	//Read Operations
	err := chain.Database.View(func(txn *badger.Txn) error {
		if item, err := txn.Get(blockHash); err != nil {
			return errors.New("Block does not exist")
		} else {
			blockData, _ := item.ValueCopy(nil)
			// 反序列化区块数据
			block = *DeSerialize(blockData)
		}
		return nil
	})

	if err != nil {
		return block, err
	}

	return block, nil
}

// GetBlockHashes 总计得到区块链中的所有区块哈希数组
func (chain *Blockchain) GetBlockHashes(height int) [][]byte {
	var blocks [][]byte//[]byte为单个block的哈希值

	iter := chain.Iterator()
	if iter == nil {
		return blocks
	}
	for {
		block := iter.Next()
		prevHash := block.PrevHash
		if block.Height == height {
			break
		}
		blocks = append([][]byte{block.Hash}, blocks...)//[][]byte{block.Hash}为只有一个元素的切片，append要求两个连接的切片类型必须相同

		if prevHash == nil {
			break
		}
	}

	return blocks
}

// GetBestHeight 得到最佳height基本上是获取最后区块的height（index）
func (chain *Blockchain) GetBestHeight() int {
	var lastBlock Block

	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		if err == nil {
			lastHash, _ := item.ValueCopy(nil)

			item, err = txn.Get(lastHash)
			Handle(err)
			lastBlockData, _ := item.ValueCopy(nil)
			lastBlock = *DeSerialize(lastBlockData)
		}

		return err
	})

	if err == nil {
		return lastBlock.Height
	}

	return 0
}

// MineBlock 挖矿：挖出一个新区块，并将它添加到区块链中
func (chain *Blockchain) MineBlock(transactions []*Transaction) *Block {
	var lastHash []byte
	var lastHeight int

	for _, tx := range transactions {
		if chain.VerifyTransaction(tx) != true {
			log.Panic("Invalid Transaction")
		}
	}
	lastHash = chain.LastHash
	//填充lastHeight
	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		Handle(err)
		lastHash, err = item.ValueCopy(nil)

		item, err = txn.Get(lastHash)
		Handle(err)
		lastBlockData, _ := item.ValueCopy(nil)

		lastBlock := DeSerialize(lastBlockData)

		lastHeight = lastBlock.Height
		return err
	})

	Handle(err)

	block := CreateBlock(transactions, lastHash, lastHeight+1)//区块高度+1
	// 读-写操作
	err = chain.Database.Update(func(txn *badger.Txn) error {
		err := txn.Set(block.Hash, block.Serialize())
		Handle(err)
		err = txn.Set([]byte("lh"), block.Hash)

		chain.LastHash = lastHash

		return err
	})

	Handle(err)
	return block
}

// DeserializeTransaction 反序列化交易对象
func DeserializeTransaction(data []byte) Transaction {
	var transaction Transaction

	dec := gob.NewDecoder(bytes.NewReader(data))
	err := dec.Decode(&transaction)
	Handle(err)
	return transaction
}

// 来自区块链的总计所有未花费交易输出
func (chain *Blockchain) FindUTXO() map[string]TxOutputs {
	UTXOs := make(map[string]TxOutputs)
	spentTXOs := make(map[string][]int)

	iter := chain.Iterator()

	for {
		block := iter.Next()

		for _, tx := range block.Transactions {
			//交易ID转为字符串
			txID := hex.EncodeToString(tx.ID)

		Outputs:
			for outIdx, out := range tx.Outputs {
				if spentTXOs[txID] != nil {
					for _, spentOut := range spentTXOs[txID] {
						if spentOut == outIdx {
							continue Outputs
						}
					}
				}
				//加入到UTXO
				outs := UTXOs[txID]
				outs.Outputs = append(outs.Outputs, out)
				UTXOs[txID] = outs
			}
			if !tx.IsMinerTx() {
				//持续跟踪已花费交易输出（Spent Transaction Outputs）
				for _, in := range tx.Inputs {
					inTxID := hex.EncodeToString(in.ID)
					spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Out)
				}
			}
		}
		if len(block.PrevHash) == 0 {
			break
		}
	}
	return UTXOs
}

//根据ID查找指定的交易
func (chain *Blockchain) FindTransaction(ID []byte) (Transaction, error) {
	iter := chain.Iterator()

	for {
		block := iter.Next()

		for _, tx := range block.Transactions {
			if bytes.Compare(tx.ID, ID) == 0 {
				return *tx, nil
			}
		}
		if len(block.PrevHash) == 0 {
			break
		}
	}
	log.Error("错误: 不存在ID的交易")

	return Transaction{}, errors.New("不存在ID的交易")
}
// GetTransaction 得到交易的map格式
func (chain *Blockchain) GetTransaction(transaction *Transaction) map[string]Transaction {
	txs := make(map[string]Transaction)
	for _, in := range transaction.Inputs {
		// get all transaction with in.ID
		tx, err := chain.FindTransaction(in.ID)
		if err != nil {
			log.Error("Error: Invalid Transaction Ewwww")
		}
		Handle(err)
		txs[hex.EncodeToString(tx.ID)] = tx
	}

	return txs
}

// SignTransaction 签名一个交易
func (chain *Blockchain) SignTransaction(privKey ecdsa.PrivateKey, tx *Transaction) {
	prevTxs := chain.GetTransaction(tx)
	tx.Sign(privKey, prevTxs)
}

// VerifyTransaction 验证交易
func (chain *Blockchain) VerifyTransaction(tx *Transaction) bool {
	if tx.IsMinerTx() {
		return true
	}
	prevTxs := chain.GetTransaction(tx)

	return tx.Verify(prevTxs)
}
// retry 删除lock为尾缀的数据库文件，并再次打开数据库
func retry(dir string, originalOpts badger.Options) (*badger.DB, error) {
	lockPath := filepath.Join(dir, "LOCK")

	if err := os.Remove(lockPath); err != nil {
		return nil, fmt.Errorf(`removing "LOCK": %s`, err)
	}

	retryOpts := originalOpts
	retryOpts.Truncate = true
	db, err := badger.Open(retryOpts)
	return db, err
}
// OpenDB 打开数据库（如果存因为存在LOCK文件打开失败，执行retry确保打开
func OpenDB(dir string, opts badger.Options) (*badger.DB, error) {

	if db, err := badger.Open(opts); err != nil {

		if strings.Contains(err.Error(), "LOCK") {

			if db, err := retry(dir, opts); err == nil {
				log.Panicln("数据库解锁 , value log 被截断 ")
				return db, nil
			}

			log.Panicln("无法解锁数据库", err)
		}

		return nil, err
	} else {
		return db, nil
	}
}