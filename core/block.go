package blockchain

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"time"
)

//Block 区块结构新版，增加了计数器nonce，主要目的是为了校验区块是否合法
//即挖出的区块是否满足工作量证明要求的条件
type Block struct {
	Timestamp    int64          `json:"Timestamp"`
	Hash         []byte         `json:"Hash"`
	PrevHash     []byte         `json:"PrevHash"`
	Transactions []*Transaction `json:"Transactions"`
	Nonce        int            `json:"Nonce"`
	Height       int            `json:"Height"`
	MerkleRoot   []byte         `json:"MerkleRoot"`
	Difficulty   int            `json:"Difficulty"`
	TxCount      int            `json:"TxCount"`
}

// HashTransactions 计算交易组合的哈希值，最后得到的是Merkle tree的根节点
//获得每笔交易的哈希，将它们关联起来，然后获得一个连接后的组合哈希
func (block *Block) HashTransactions() []byte {
	var txHashes [][]byte

	for _, tx := range block.Transactions {
		txHashes = append(txHashes, tx.Serializer())
	}

	tree := NewMerkleTree(txHashes)
	return tree.RootNode.Data
}

// CreateBlock挖出区块
func CreateBlock(txs []*Transaction, prevHash []byte, height int) *Block {
	block := &Block{
		time.Now().Unix(),
		[]byte{},
		prevHash,
		txs,
		0,
		height,
		[]byte{},
		Difficulty,
		len(txs),
	}
	pow := NewProof(block)
	nonce, hash := pow.Run()

	block.Hash = hash[:]
	block.Nonce = nonce
	//设置MerkleRoot
	block.MerkleRoot = block.HashTransactions()

	return block
}

// 创建创始区块，创始区块的height为1
func Genesis(MinerTx *Transaction) *Block {
	return CreateBlock([]*Transaction{MinerTx}, []byte{}, 1)
}

// 工具函数，序列化区块链数据
func (b *Block) Serialize() []byte {
	var res bytes.Buffer
	encoder := gob.NewEncoder(&res)

	err := encoder.Encode(b)
	Handle(err)
	return res.Bytes()
}

// 工具函数，反序列化区块链数据
func DeSerialize(data []byte) *Block {
	var block Block
	encoder := gob.NewDecoder(bytes.NewReader(data))

	err := encoder.Decode(&block)
	Handle(err)
	return &block
}
func (b *Block) IsGenesis() bool {
	return b.PrevHash == nil
}

// 通过确认区块中的各种信息来检查该区块是否有效
func (b *Block) IsBlockValid(oldBlock Block) bool {
	if oldBlock.Height+1 != b.Height {
		return false
	}
	res := bytes.Compare(oldBlock.Hash, b.PrevHash)
	if res != 0 {
		return false
	}
	// pow := NewProof(b)
	// validate := pow.Validate()

	return true
}

func ConstructJSON(buffer *bytes.Buffer, block *Block) {
	buffer.WriteString("{")
	buffer.WriteString(fmt.Sprintf("\"%s\":\"%d\",", "Timestamp", block.Timestamp))
	buffer.WriteString(fmt.Sprintf("\"%s\":\"%x\",", "PrevHash", block.PrevHash))

	buffer.WriteString(fmt.Sprintf("\"%s\":\"%x\",", "Hash", block.Hash))

	buffer.WriteString(fmt.Sprintf("\"%s\":%d,", "Difficulty", block.Difficulty))

	buffer.WriteString(fmt.Sprintf("\"%s\":%d,", "Nonce", block.Nonce))

	buffer.WriteString(fmt.Sprintf("\"%s\":\"%x\",", "MerkleRoot", block.MerkleRoot))
	buffer.WriteString(fmt.Sprintf("\"%s\":%d", "TxCount", block.TxCount))
	buffer.WriteString("}")
}

func (bs *Block) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString("[")
	ConstructJSON(buffer, bs)
	buffer.WriteString("]")
	return buffer.Bytes(), nil
}
