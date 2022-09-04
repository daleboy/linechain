package p2p

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	discovery "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/multiformats/go-multiaddr"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	mplex "github.com/libp2p/go-libp2p/p2p/muxer/mplex"
	yamux "github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	quic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	tcp "github.com/libp2p/go-libp2p/p2p/transport/tcp"
	ws "github.com/libp2p/go-libp2p/p2p/transport/websocket"
	log "github.com/sirupsen/logrus"

	blockchain "linechain/core"
	"linechain/memopool"
	appUtils "linechain/util/utils"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
)

const (
	version       = 1
	commandLength = 20 //命令长度为20个字节
)

// 说明：以下很多由Network实例调用的方法，方法的实例net由startNode调用获得

//定义全局变量
var (
	GeneralChannel   = "general-channel"
	MiningChannel    = "mining-channel"
	FullNodesChannel = "fullnodes-channel"
	MinerAddress     = ""
	blocksInTransit  = [][]byte{}         //待交换中所有block的哈希（通过发送inv，获取的block可能有多个，可以先缓存于此）
	memoryPool       = memopool.MemoPool{ //交易池
		Pending: map[string]blockchain.Transaction{},
		Queued:  map[string]blockchain.Transaction{},
		Wg:      sync.WaitGroup{},
	}
)

// SendBlock 将block发送给peerId节点（通过general通道，这个通道的消息所有节点均需要订阅）
// 如果指定peerId，则只发给指定的节点；如果peerId为空，则发布给全网
func (net *Network) SendBlock(peerId string, b *blockchain.Block) {
	data := Block{net.Host.ID().Pretty(), b.Serialize()}
	payload := GobEncode(data)

	//命令构成：cmd+payload，连接两个相同类型的切片，构成新切片
	//slice = append(slice, anotherSlice...)
	request := append(CmdToBytes("block"), payload...)

	if peerId != "" {
		net.GeneralChannel.Publish("发送 block 命令", request, peerId)
	} else {
		net.GeneralChannel.Publish("发送 block 命令", request, "")
	}
}

//处理收到的block消息
func (net *Network) HandleBlock(content *ChannelContent) {
	var buff bytes.Buffer
	var payload Block

	buff.Write(content.Payload[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)

	if err != nil {
		log.Panic(err)
	}

	blockData := payload.Block
	block := blockchain.DeSerialize(blockData)

	// fmt.Printf("Valid: %s\n", strconv.FormatBool(validate))
	// 验证区块后再将其加入到区块链中

	if block.IsGenesis() {
		net.Blockchain.AddBlock(block)
	} else {
		lastBlock, err := net.Blockchain.GetBlock(net.Blockchain.LastHash)
		if err != nil {
			log.Info(err)
		}
		log.Info(block.Height)
		valid := block.IsBlockValid(lastBlock)
		log.Info("Block validity:", strconv.FormatBool(valid))
		if valid {
			net.Blockchain.AddBlock(block)

			//从内存池中移除交易
			for _, tx := range block.Transactions {
				txID := hex.EncodeToString(tx.ID)
				memoryPool.RemoveFromAll(txID)
			}
		} else {
			log.Fatalf("发现一个非法区块，其 height 是: %d", block.Height)

			//出现非法区块是非常严重的业务逻辑错误，程序需要终止执行
			//同步调用CloseDB进行阻塞，等待程序强行终止信号，退出程序（不会继续执行本行代码之后的代码）
			appUtils.CloseDB(net.Blockchain)
		}
	}

	if len(block.Transactions) > 0 {
		for _, tx := range block.Transactions {
			memoryPool.RemoveFromAll(hex.EncodeToString(tx.ID))
		}
	}

	log.Infof("Added block %x \n", block.Hash)
	log.Infof("Block in transit %d", len(blocksInTransit))

	if len(blocksInTransit) > 0 {
		//取出第一个待交换的block的hash
		blockHash := blocksInTransit[0]
		//发送getdata（block）命令，请求完整区块
		net.SendGetData(payload.SendFrom, "block", blockHash)
		//将此block的hash从待交换block hashes列表中移除
		blocksInTransit = blocksInTransit[1:]
	} else {
		UTXO := blockchain.UTXOSet{Blockchain: net.Blockchain}
		UTXO.Compute()
	}
}
func (net *Network) SendGetData(peerId string, _type string, id []byte) {
	payload := GobEncode(GetData{net.Host.ID().Pretty(), _type, id})
	request := append(CmdToBytes("getdata"), payload...)
	net.GeneralChannel.Publish("发送 getdata 命令", request, peerId)
}

func (net *Network) HandleGetData(content *ChannelContent) {
	var buff bytes.Buffer
	var payload GetData

	buff.Write(content.Payload[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)

	if err != nil {
		log.Panic(err)
	}

	if payload.Type == "block" {
		block, err := net.Blockchain.GetBlock([]byte(payload.ID))
		if err != nil {
			return
		}

		//将block发送给请求者（peerId）
		net.SendBlock(payload.SendFrom, &block)
	}

	if payload.Type == "tx" {
		txID := hex.EncodeToString(payload.ID)
		tx := memoryPool.Pending[txID]
		if net.BelongsToMiningGroup(payload.SendFrom) {
			memoryPool.Move(tx, "queued")
			net.SendTxFromPool(payload.SendFrom, &tx)
		} else {
			net.SendTx(payload.SendFrom, &tx)
		}
	}
}

// SendInv 发送本地区块链拥有的交易或区块的清单（只有交易或区块的hash值），
// 对方处理Inv命令得到清单后，可比对本地的交易或区块，如有缺失，对方可发送getdata命令下载。
// 本项目中只发送本地区块链拥有的区块或交易清单，适用于普通节点。
// 一般情况下，在本地区块链的区块或交易发生新增后（特别是挖出新的区块后），发送Inv命令
func (net *Network) SendInv(peerId string, _type string, items [][]byte) {
	inventory := Inv{net.Host.ID().Pretty(), _type, items}
	payload := GobEncode(inventory)
	request := append(CmdToBytes("inv"), payload...)
	net.GeneralChannel.Publish("发送 inv 命令", request, peerId)
}

func (net *Network) HandleInv(content *ChannelContent) {
	var buff bytes.Buffer
	var payload Inv

	buff.Write(content.Payload[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)

	if err != nil {
		log.Panic(err)
	}
	log.Infof("收到库存消息： %d %s \n", len(payload.Items), payload.Type)

	if payload.Type == "block" {
		if len(payload.Items) >= 1 {
			//修复bug：应当请求 payload.Items 中所有的区块，而不是一个区块
			for _, blockHash := range payload.Items {
				net.SendGetData(payload.SendFrom, "block", blockHash) //请求一个完整区块
				//检查下收到的block的hash是否存在于待交换列表blocksInTransit
				for _, b := range blocksInTransit {
					if !bytes.Equal(b, blockHash) {
						blocksInTransit = append(blocksInTransit, b)
					}
				}
			}
		} else {
			log.Info("区块哈希项为空")
		}
	}

	if payload.Type == "tx" {
		if len(payload.Items) == 0 {
			memoryPool.Wg.Done()
		}
		for _, txID := range payload.Items {
			if memoryPool.Pending[hex.EncodeToString(txID)].ID == nil {
				net.SendGetData(payload.SendFrom, "tx", txID)
			}
		}
	}
}

func (net *Network) SendGetBlocks(peerId string, height int) {
	payload := GobEncode(GetBlocks{net.Host.ID().Pretty(), height})
	request := append(CmdToBytes("getblocks"), payload...)
	net.GeneralChannel.Publish("发送 getblocks 命令", request, peerId)
}

func (net *Network) HandleGetBlocks(content *ChannelContent) {
	var buff bytes.Buffer
	var payload GetBlocks

	buff.Write(content.Payload[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)

	if err != nil {
		log.Panic(err)
	}

	chain := net.Blockchain.ContinueBlockchain()
	blockHashes := chain.GetBlockHashes(payload.Height)
	log.Info("LENGTH:", len(blockHashes))
	net.SendInv(payload.SendFrom, "block", blockHashes)
}

func (net *Network) SendVersion(peer string) {
	bestHeight := net.Blockchain.GetBestHeight()
	payload := GobEncode(Version{
		version,
		bestHeight,
		net.Host.ID().Pretty(),
	})
	request := append(CmdToBytes("version"), payload...)
	net.GeneralChannel.Publish("发送 version 命令", request, peer)
}

func (net *Network) HandleVersion(content *ChannelContent) {
	var buff bytes.Buffer
	var payload Version

	buff.Write(content.Payload[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)

	if err != nil {
		log.Panic(err)
	}

	bestHeight := net.Blockchain.GetBestHeight()
	otherHeight := payload.BestHeight
	log.Info("BEST HEIGHT: ", bestHeight, " OTHER HEIGHT:", otherHeight)
	if bestHeight < otherHeight {
		net.SendGetBlocks(payload.SendFrom, bestHeight)
	} else if bestHeight > otherHeight {
		net.SendVersion(payload.SendFrom)
	}
}

func (net *Network) SendTx(peerId string, transaction *blockchain.Transaction) {
	memoryPool.Add(*transaction)

	tnx := Tx{net.Host.ID().Pretty(), transaction.Serializer()}
	payload := GobEncode(tnx)
	request := append(CmdToBytes("tx"), payload...)

	// 给全节点通道发布此消息，全节点（FullNode）将进行处理
	net.FullNodesChannel.Publish("发送 tx 命令", request, peerId)
}

func (net *Network) SendTxPoolInv(peerId string, _type string, items [][]byte) {
	inventory := Inv{net.Host.ID().Pretty(), _type, items}
	payload := GobEncode(inventory)
	request := append(CmdToBytes("inv"), payload...)
	// 给挖矿节点的通信通道发布此消息，挖矿节点将进行处理
	net.MiningChannel.Publish("发送 tx 类型的 inv 命令", request, peerId)
}

func (net *Network) SendTxFromPool(peerId string, transaction *blockchain.Transaction) {

	tnx := Tx{net.Host.ID().Pretty(), transaction.Serializer()}
	payload := GobEncode(tnx)
	request := append(CmdToBytes("tx"), payload...)

	// 给挖矿节点的通信通道发布此消息，挖矿节点将进行处理
	net.MiningChannel.Publish("发送 tx 命令", request, peerId)
}

func (net *Network) HandleGetTxFromPool(content *ChannelContent) {
	var buff bytes.Buffer
	var payload TxFromPool

	buff.Write(content.Payload[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)

	if err != nil {
		log.Panic(err)
	}

	if len(memoryPool.Pending) >= payload.Count { //如果挂起的交易数量达到指定的数量，交给挖矿节点挖矿
		txs := memoryPool.GetTransactions(payload.Count)
		net.SendTxPoolInv(payload.SendFrom, "tx", txs)
	} else {
		net.SendTxPoolInv(payload.SendFrom, "tx", [][]byte{})
	}
}

// HandleTx 全节点和挖矿节点处理tx命令消息
func (net *Network) HandleTx(content *ChannelContent) {
	var buff bytes.Buffer
	var payload Tx

	buff.Write(content.Payload[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)

	if err != nil {
		log.Panic(err)
	}

	txData := payload.Transaction
	tx := blockchain.DeserializeTransaction(txData)

	log.Infof("%s, %d", payload.SendFrom, len(memoryPool.Pending))
	chain := net.Blockchain.ContinueBlockchain()

	// 若是全节点，只负责验证交易，并将交易放到内存池中
	// 若是挖矿节点负责挖矿
	if chain.VerifyTransaction(&tx) {
		//如果tx来自本地节点，说明本地节点不是挖矿节点，也没有挖出它，就将它加入到Pending中，成为tx类型的inv，
		//在其它节点请求tx时候，将本地tx发给对方处理
		memoryPool.Add(tx)
		if net.Miner { //当前节点为矿工节点
			//将交易移到排队队列
			memoryPool.Move(tx, "queued")
			log.Info("MINING")
			//立即挖出排队中的所有交易
			net.MineTx(memoryPool.Queued)
		}
	}
}
func (net *Network) MineTx(memopoolTxs map[string]blockchain.Transaction) {
	var txs []*blockchain.Transaction
	log.Infof("挖矿的交易数: %d", len(memopoolTxs))
	chain := net.Blockchain.ContinueBlockchain()

	for id := range memopoolTxs {
		log.Infof("tx: %s \n", memopoolTxs[id].ID)
		tx := memopoolTxs[id]

		log.Info("tx校验: ", chain.VerifyTransaction(&tx))
		if chain.VerifyTransaction(&tx) {
			txs = append(txs, &tx)
		}
	}

	if len(txs) == 0 {
		log.Info("无合法的交易")
	}

	cbTx := blockchain.MinerTx(MinerAddress, "")
	txs = append(txs, cbTx)
	newBlock := chain.MineBlock(txs)
	UTXOs := blockchain.UTXOSet{Blockchain: chain}
	UTXOs.Compute()

	log.Info("挖出新的区块")

	//peerId为空，SendInv发布给全网
	net.SendInv("", "block", [][]byte{newBlock.Hash})
	memoryPool.ClearAll() //清除内存池中的全部交易
	memoryPool.Wg.Done()
}

func (net *Network) BelongsToMiningGroup(PeerId string) bool {
	peers := net.MiningChannel.ListPeers()
	for _, peer := range peers {
		Id := peer.Pretty()

		if Id == PeerId {
			return true
		}
	}

	return false
}
func (net *Network) MinersEventLoop(ui *CLIUI) {
	//秒定时器
	poolCheckTicker := time.NewTicker(time.Second)
	defer poolCheckTicker.Stop()

	for {
		select {
		case <-poolCheckTicker.C:
			tnx := TxFromPool{net.Host.ID().Pretty(), 1} //每次取一条交易，可以优化为每次取多条交易
			payload := GobEncode(tnx)
			request := append(CmdToBytes("gettxfrompool"), payload...)
			net.FullNodesChannel.Publish("内存池中交易 gettxfrompool 命令", request, "")
			memoryPool.Wg.Add(1)

		case <-ui.doneCh:
			return
		}
	}
}

// StartNode 启动一个节点
func StartNode(chain *blockchain.Blockchain, listenPort, minerAddress string, miner, fullNode bool, callback func(*Network)) {
	//Reader 是加密安全随机数生成器的全局共享实例
	var r io.Reader = rand.Reader //没有指定seed，使用随机种子

	MinerAddress = minerAddress
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() //释放相关资源

	defer chain.Database.Close() //函数运行结束，关闭区块链数据库
	go appUtils.CloseDB(chain)   //启动协程，遇到程序强行终止信号时关闭数据库，退出程序

	// 为本主机（host）创建一对新的 RSA 密钥
	// 一般情况下，是事先创建本机的密钥文件，然后调用LoadKeyFromFile来读取私钥
	prvKey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
	if err != nil {
		panic(err)
	}

	//go-ws-transport：ws协议
	//go-tcp-transport：tcp协议（连接三次握手）
	//go-libp2p-quic-transport：quic协议（连接0次握手，移动时代的协议）
	//go-udp-transport：udp协议
	//go-utp-transport：uTorrent 协议(UTP)
	//go-libp2p-circuit：relay协议
	//go-libp2p-transport-upgrader：upgrades multiaddr-net connections into full libp2p transports
	transports := libp2p.ChainOptions(
		libp2p.Transport(tcp.NewTCPTransport), //支持TCP传输协议
		libp2p.Transport(ws.New),              //支持websocket传输协议
		libp2p.Transport(quic.NewTransport),   //支持quic传输协议
	)

	muxers := libp2p.ChainOptions(
		libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport), //支持"/yamux/1.0.0"流连接(基于可靠连接的多路I/O复用)
		libp2p.Muxer("/mplex/6.7.0", mplex.DefaultTransport), //支持"/mplex/6.7.0"流连接（二进制流多路I/O复用），由LibP2P基于multiplex创建
	)

	if len(listenPort) == 0 {
		listenPort = "0"
	}

	listenAddrs := libp2p.ListenAddrStrings(
		fmt.Sprintf("/ip4/0.0.0.0/tcp/%s", listenPort),    //tcp传输
		fmt.Sprintf("/ip4/0.0.0.0/tcp/%s/ws", listenPort), //websorket传输
	)

	// Host是参与p2p网络的对象，它实现协议或提供服务。
	// 它像服务器一样处理请求，像客户端一样发出请求。
	// 之所以称为 Host，是因为它既是 Server 又是 Client（而 Peer 可能会混淆）。
	// 1、创建host
	// 重要：创建主机host
	//-如果没有提供transport和listen addresses，节点将监听在多地址（mutiaddresses）： "/ip4/0.0.0.0/tcp/0" 和 "/ip6/::/tcp/0";
	//-如果没有提供transport的选项，节点使用TCP和websorcket传输协议
	//-如果multiplexer配置没有提供，节点缺省使用"yamux/1.0.0" 和 "mplux/6.7.0"流连接配置
	//-如果没有提供security transport，主机使用go-libp2p的noise和/或tls加密的transport来加密所有的traffic(新版本libp2p已经不再支持security transport参数设置)
	//-如果没有提供peer的identity，它产生一个随机RSA 2048键值对，并由它导出一个新的identity
	//-如果没有提供peerstore，主机使用一个空的peerstore来进行初始化

	host, err := libp2p.New(
		//ctx,
		transports,
		listenAddrs,
		muxers,
		libp2p.Identity(prvKey),
		libp2p.EnableNATService(),
		libp2p.ForceReachabilityPublic(),
	)
	if err != nil {
		panic(err)
	}
	for _, addr := range host.Addrs() {
		fmt.Println("正在监听在", addr)
	}
	log.Info("主机已创建: ", host.ID())

	// 2、使用GossipSub路由，创建一个新的基于Gossip 协议的 PubSub 服务系统
	// 任何一个主机节点，都是一个订阅发布服务系统
	// 这是整个区块链网络运行的关键所在
	pubsub, err := pubsub.NewGossipSub(ctx, host)
	if err != nil {
		panic(err)
	}

	// 3、构建三个通信通道，通信通道使用发布-订阅系统，在不同节点之间传递信息
	// 之所以需要三个通道，是因为未来规划不同节点拥有不同的功能，不同功能的节点完成不同类型的任务。
	// 三个通道的消息独立，只有订阅了该通道消息的节点，才能收到该通道的消息，然后进行处理，以完成相应的任务。
	// 任何一个节点，均创建了三个通道实例，这意味着人一个节点都可以根据需要，选择任意一个通道发送消息
	// 在订阅上，一个具体的节点， GeneralChannel 订阅将消息，如果是采矿节点（miner==true），miningChannel 会接收到消息，
	// 如果是全节点（fullNode==true），fullNodesChannel会接受到消息

	//GeneralChannel 通道订阅消息
	generalChannel, _ := JoinChannel(ctx, pubsub, host.ID(), GeneralChannel, true)

	//如果是挖矿节点， miningChannel 订阅消息，否则 miningChannel 不订阅消息
	subscribe := false
	if miner {
		subscribe = true
	}
	miningChannel, _ := JoinChannel(ctx, pubsub, host.ID(), MiningChannel, subscribe)

	//如果是全节点， fullNodesChannel 订阅消息，否则 fullNodesChannel 不订阅消息
	subscribe = false
	if fullNode {
		subscribe = true
	}
	fullNodesChannel, _ := JoinChannel(ctx, pubsub, host.ID(), FullNodesChannel, subscribe)

	// 3、为各通信通道建立命令行界面对象，命令行监控来自三个通道的消息
	ui := NewCLIUI(generalChannel, miningChannel, fullNodesChannel)

	// 4、建立对等端（peer）发现机制（discovery），使得本节点可以被网络上的其它节点发现
	// 同时将主机（host）连接到所有已经发现的对等端（peer）
	var bootstraps []multiaddr.Multiaddr
	err = SetupDiscovery(ctx, host, bootstraps)
	if err != nil {
		panic(err)
	}
	network := &Network{
		Host:             host,
		GeneralChannel:   generalChannel,
		MiningChannel:    miningChannel,
		FullNodesChannel: fullNodesChannel,
		Blockchain:       chain,
		Blocks:           make(chan *blockchain.Block, 200),       //新Block数量不超过200个
		Transactions:     make(chan *blockchain.Transaction, 200), //新Tansaction数量不超过200个
		Miner:            miner,
	}

	// 5、回调，将节点（network）实例传回
	callback(network)

	// 6、向全网请求区块信息，以补全本地区块链
	// 每一个节点均有区块链的一个完整副本
	err = RequestBlocks(network)

	// 7、启用协程，处理网络节点事件
	go HandleEvents(network)

	// 8、如果是矿工节点，启用协程，不断发送ping命令给全节点
	if miner {
		// 矿工事件循环，以不断地发送一个 ping 给全节点，目的是得到新的交易，为新交易挖矿，并添加到区块链
		go network.MinersEventLoop(ui)
	}

	if err != nil {
		panic(err)
	}

	// 9、运行UI界面，将在Run函数体中启动协程，循环接收并处理全网通道publish的消息
	//（包括generalChannel, miningChannel, fullNodesChannel通道）
	if err = ui.Run(network); err != nil {
		log.Error("运行文字UI发生错误: %s", err)
	}
}

// 这里只拦截处理Blocks和Transaction两个通道的消息，三个Channel通道消息这里不处理
// 仅当启用节点的rpc时候，才会有此两个消息需（系统仅仅支持通过rpc方式发送交易）（cmdutil：send命令）
// 当本地节点发送交易，如果立如果命令参数指示立即挖矿，则挖矿成功后，会产生blocks消息，在这里发布给全网
// 如果命令参数指示不立即挖矿，则会产生transactions消息，在这里发布给全网
func HandleEvents(net *Network) {
	for {
		select {
		// mine := true
		case block := <-net.Blocks: //如果 Blocks 队列新增数据（block数据），全网广播
			net.SendBlock("", block)
		//mine := false
		case tnx := <-net.Transactions: //如果 Transactions 队列新增数据（Transaction数据），全网广播
			net.SendTx("", tnx)
		}
	}
}
func RequestBlocks(net *Network) error {
	// 列出 GerneralChannel 通道中已经连接的节点
	peers := net.GeneralChannel.ListPeers()
	// 发送 version 命令
	if len(peers) > 0 {
		net.SendVersion(peers[0].Pretty())
	}
	return nil
}

// SetupDiscovery 建立发现机制，并将本地主机连接到所有已经发现的对等端（peer）
func SetupDiscovery(ctx context.Context, host host.Host, bootstrapPeers []multiaddr.Multiaddr) error {
	var options []dht.Option
	//如果没有任何启动节点，本节点以server模式运行（作为启动节点）
	if len(bootstrapPeers) == 0 {
		//由于mode的值为int，因此缺省情况下为0，即节点运行模式为client
		options = append(options, dht.Mode(dht.ModeServer))
	}
	// 开启一个DHT，用于对等端（peer）发现。
	// DHT全称叫分布式哈希表(Distributed Hash Table)，是一种分布式存储方法。IpfsDHT是Kademlia算法的一个实现
	// Kademlia算法是一种分布式存储及路由的算法
	// 我们不仅仅是创建一个新的DHT，因为我们要求每一个端维护它自己的本地DHT副本，这样
	// 以来，DHT的引导节点可以关闭，不会影响后续的对等端发现
	kademliaDHT, err := dht.New(ctx, host, options...)
	if err != nil {
		panic(err)
	}

	//如果在startnode中创建本地缓存数据库，可以执行如下开启DHT的方式：
	/*
		dataStorePath := fmt.Sprintf(".dht-%s-%s", *ip, *port)
		dataStore, err := badger.NewDatastore(dataStorePath, nil)
		if err != nil {
			utils.FatalErrMsg(err, "cannot initialize DHT cache at %s", dataStorePath)
		}
		dht := kaddht.NewDHT(context.Background(), host.GetP2PHost(), dataStore)

		if err := dht.Bootstrap(context.Background()); err != nil {
			utils.FatalErrMsg(err, "cannot bootstrap DHT")
		}*/

	// 引导DHT。在缺省设置下，这生成一个后台线程，每5分钟刷新对等端表格
	log.Info("引导DHT")
	if err = kademliaDHT.Bootstrap(ctx); err != nil {
		panic(err)
	}

	// 让我们首先连接到所有的引导节点（bootstrap nodes），它们会告诉我们网络中的其他节点
	var wg sync.WaitGroup
	for _, peerAddr := range dht.DefaultBootstrapPeers {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)
		wg.Add(1)
		//使用多个协程，加快连接处理
		go func() {
			defer wg.Done()
			if err := host.Connect(ctx, *peerinfo); err != nil {
				log.Error(err)
			} else {
				log.Info("连接已建立，使用的引导节点是:", *peerinfo)
			}
		}()
	}
	wg.Wait() //阻塞，确保所有的协程全部返回

	// 我们使用一个会合点“wlsell.com”来宣布我们的位置
	// 这就像告诉你的朋友在某个具体的地点会合
	log.Info("宣布我们自己...")
	routingDiscovery := discovery.NewRoutingDiscovery(kademliaDHT)
	routingDiscovery.Advertise(ctx, "rendezvous:wlsell.com")
	log.Info("成功宣布!")

	// 现在，查找那些已经宣布的对等端
	// 这就像你的朋友告诉你会合的地点
	log.Info("搜索其它的对等端...")
	peerChan, err := routingDiscovery.FindPeers(ctx, "rendezvous:wlsell.com")
	if err != nil {
		panic(err)
	}

	// 连接到所有新发现的对等端（peer）
	for peer := range peerChan {
		if peer.ID == host.ID() {
			continue //不连接自己
		}
		log.Debug("找到对等端:", peer)

		log.Debug("正在连接到:", peer)
		err := host.Connect(context.Background(), peer)
		if err != nil {
			log.Warningf("连接到对等端 %s:失败 %s\n", peer.ID.Pretty(), err)
			continue
		}
		log.Info("已经连接到:", peer)
	}

	return nil
}
