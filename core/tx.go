package blockchain

import (
	"bytes"
	"encoding/gob"

	"linechain/util/env"
	"linechain/wallet"
)

var conf = env.New()
var (
	checkSumlength = conf.WalletAddressChecksum//检查钱包地址的位数
	version        = byte(0x00) // hexadecimal representation of zero
)


//TxInput 交易的输入，也代表借方
//包含的是前一笔交易的一个输出
type TxInput struct {
	ID        []byte//前一笔交易的ID
	Out       int//前一笔交易在该笔交易所有输出中的索引（一笔交易可能有多个输出，需要有信息指明具体是哪一个）
	Signature []byte//输入数字签名

	//PubKey公钥，是发送者的钱包的公钥，用于解锁输出
	//如果PubKey与所引用的锁定输出的PubKey相同，那么引用的输出就会被解锁，然后被解锁的值就可以被用于产生新的输出
	//如果不正确，前一笔交易的输出就无法被引用在输入中，或者说，也就无法使用这个输出
	//这种机制，保证了用户无法花费其他人的币
	PubKey    []byte
}

// TxOutputs 交易的输出集合
type TxOutputs struct {
	Outputs []TxOutput
}

// TxOutput 交易的输出，也代表贷方
type TxOutput struct {
	Value      float64//输出里面存储的“币”数量，注意为浮点数
	PubKeyHash []byte//锁定输出的公钥（比特币里面是一个脚本，这里是公钥）
}

// NewTxOutput 创建一个新的 TXOutput
//注意，这里需要将address进行反编码成实际的地址
func NewTXOutput(value float64, address string) *TxOutput {
	txo := &TxOutput{value, nil}//构建TxOutput，PubKeyHash暂设为nil
	txo.Lock([]byte(address))//接着设定TxOutput的PubKeyHash值，进行锁定

	return txo
}

// Lock 利用公钥锁定输出
func (out *TxOutput) Lock(address []byte) {
	pubKeyHash := wallet.Base58Decode(address)
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-checkSumlength]//计算得到公钥哈希

	out.PubKeyHash = pubKeyHash
}

// IsLockWithKey 检查输出是否被某公钥锁定
func (out *TxOutput) IsLockWithKey(pubKeyHash []byte) bool {
	return bytes.Compare(out.PubKeyHash, pubKeyHash) == 0
}

func (outputs *TxOutputs) Serialize() []byte {
	var res bytes.Buffer
	encoder := gob.NewEncoder(&res)

	err := encoder.Encode(outputs)
	Handle(err)
	return res.Bytes()
}

func DeSerializeOutputs(data []byte) TxOutputs {
	var outputs TxOutputs
	encoder := gob.NewDecoder(bytes.NewReader(data))

	err := encoder.Decode(&outputs)
	Handle(err)
	return outputs
}
