package p2p

import (
	blockchain "linechain/core"

	"github.com/libp2p/go-libp2p-core/host"
)

// Network 节点的数据结构
type Network struct {
	Host             host.Host//主机
	GeneralChannel   *Channel//通用节点
	MiningChannel    *Channel//挖矿节点
	FullNodesChannel *Channel//全节点
	Blockchain       *blockchain.Blockchain

	//Blocks和Transactions消息队列，用于存储新产生的Block或Transaction
	//一般而言，我们在主程序执行send交易过程中，根据send参数mineNow决定是否立即挖矿，mineNow为true则存储Block消息队列（立即挖矿）
	//mineNow为false则存储Transaction消息队列（不立即挖矿）。两个消息由节点在startNode后启动消息处理协程进行处理
	//一般情况下，为提高挖矿效率，我们会汇聚几个交易在一起，然后挖矿一次、
	//事实上，仅当启用节点的rpc时候，才会有此两个消息需要发布到全网
	Blocks           chan *blockchain.Block//Block类型的通道（带缓冲的通道：在StartNode函数中构建Network实例，缓冲数量200）
	Transactions     chan *blockchain.Transaction//Transaction类型的通道（带缓冲的通道：在StartNode函数中构建Network实例，缓冲数量200）

	//是否是挖矿节点
	Miner            bool
}

//以下请求命令结构中均有一个成员SendFrom，为发送命令着的peerId，
//网络上节点接收到命令后，将回复消息发给peerId节点

// Version 命令结构
type Version struct {
	Version    int
	BestHeight int
	SendFrom   string//peerId
}

// GetBlocks 命令结构
type GetBlocks struct {
	SendFrom string//节点的peerId
	Height   int
}

// Tx 命令结构
type Tx struct {
	SendFrom    string//节点的peerId
	Transaction []byte
}

// Block 命令结构
type Block struct {
	SendFrom string//节点的peerId
	Block    []byte
}

// TxFromPool 命令结构
type TxFromPool struct {
	SendFrom string//节点的peerId
	Count    int
}

// GetData 命令结构
type GetData struct {
	SendFrom string//节点的peerId
	Type     string
	ID       []byte
}

// Inv 命令结构
type Inv struct {
	SendFrom string//节点的peerId
	Type     string
	Items    [][]byte
}
