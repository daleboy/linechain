package utils

import (
	"fmt"
	"strconv"
	"time"

	blockchain "linechain/core"
	"linechain/p2p"
	"linechain/util/utils"
	"linechain/wallet"

	log "github.com/sirupsen/logrus"
)

type CommandLine struct {
	Blockchain    *blockchain.Blockchain
	//如果启用rpc，则启动节点后设置cli的P2P实例，net为启动节点函数的回调函数参数被回调后返回的Network实例
	//如果不启用rpc，则P2p一直为nil
	P2p           *p2p.Network
	CloseDbAlways bool//每次命令执行完毕是否关闭数据库
}

type Error struct {
	Code    int
	Message string
}
type BalanceResponse struct {
	Balance   float64
	Address   string
	Timestamp int64
	Error     *Error
}

type SendResponse struct {
	SendTo    string
	SendFrom  string
	Amount    float64
	Timestamp int64
	Error     *Error
}
// StartNode 启动节点，其中fn为回调函数，p2p.StartNode调用过程中调用fn，设置p2p.Network实例
func (cli *CommandLine) StartNode(listenPort, minerAddress string, miner, fullNode bool, fn func(*p2p.Network)) {
	if miner {
		log.Infof("作为矿工正在启动节点： %s\n", listenPort)
		if len(minerAddress) > 0 {
			if wallet.ValidateAddress(minerAddress) {
				log.Info("正在挖矿，接收奖励的地址是:", minerAddress)
			} else {
				log.Fatal("请提供一个合法的矿工地址")
			}
		}
	} else {
		log.Infof("在: %s\n端口上启动节点", listenPort)
	}

	chain := cli.Blockchain.ContinueBlockchain()
	p2p.StartNode(chain, listenPort, minerAddress, miner, fullNode, fn)
}


// UpdateInstance 设置区块链的instanceid（从命令行参数中读取instanceid参数，设定为cli.Blockchain的InstanceId）
// 所有与instanceid相关的命令在执行前必须先调用它（无论区块的数据库是否存在）（调用地方是main函数定义命令的RUN实现内）
func (cli *CommandLine) UpdateInstance(InstanceId string, closeDbAlways bool) *CommandLine {
	utils.SetLog(InstanceId)
	cli.Blockchain.InstanceId = InstanceId
	if blockchain.Exists(InstanceId) {
		cli.Blockchain = cli.Blockchain.ContinueBlockchain()
	}
	cli.CloseDbAlways = closeDbAlways

	return cli
}

// Send 发送代币
func (cli *CommandLine) Send(from string, to string, amount float64, mineNow bool) SendResponse {
	
	if !wallet.ValidateAddress(from) {
		log.Error("sendFrom地址非法")
		return SendResponse{
			Error: &Error{
				Code:    5028,
				Message: "sendFrom地址非法",
			},
		}
	}
	if !wallet.ValidateAddress(to) {
		log.Error("sendTo地址非法 ")
		return SendResponse{
			Error: &Error{
				Code:    5028,
				Message: "SendTo地址非法",
			},
		}
	}


	//从本地数据库读取区块链实例
	chain := cli.Blockchain.ContinueBlockchain()
	if cli.CloseDbAlways {
		defer chain.Database.Close()
	}

	utxos := blockchain.UTXOSet{Blockchain:chain}
	cwd := false
	wallets, err := wallet.InitializeWallets(cwd,chain.InstanceId)
	if err != nil {
		chain.Database.Close()
		log.Panic(err)
	}
	
	wallet, err := wallets.GetWallet(from)
	if err != nil {
		log.Error("请导入sendfrom的钱包到此节点")
		return SendResponse{
			Error: &Error{
				Code:    5028,
				Message: "请导入sendfrom的钱包到此节点",
			},
		}
	}

	tx, err := blockchain.NewTransaction(&wallet, to, amount, &utxos)
	if err != nil {
		log.Error(err)
		return SendResponse{
			Error: &Error{
				Code:    5028,
				Message: "执行交易失败",
			},
		}
	}
	if mineNow {
		//如果需要立即挖矿，则自己作为矿工立即挖矿
		cbTx := blockchain.MinerTx(from, "")
		txs := []*blockchain.Transaction{cbTx, tx}
		
		block := chain.MineBlock(txs)
		log.Info("交易已执行")
		utxos.Update(block)

		//仅当节点启用rpc时候，cli.P2p才不会为nil
		if cli.P2p != nil {
			cli.P2p.Blocks <- block
		}
	} else {
		//不需要立即挖矿，将tx放进节点（network）的Transaction消息队列中
		//仅当节点启用rpc时候，cli.P2p才不会为nil
		if cli.P2p != nil {
			cli.P2p.Transactions <- tx
			log.Info("交易送到本地节点交易内存池")
		}
	}

	return SendResponse{
		SendTo:    to,
		SendFrom:  from,
		Amount:    amount,
		Timestamp: time.Now().Unix(),
	}
}
// CreateBlockchain 创建全新区块
func (cli *CommandLine) CreateBlockchain(address string) {
	if !wallet.ValidateAddress(address) {
		log.Panic("非法地址")
	}

	chain := blockchain.InitBlockchain(address, cli.Blockchain.InstanceId)
	if cli.CloseDbAlways {
		defer chain.Database.Close()
	}
	utxos := blockchain.UTXOSet{Blockchain:chain}
	utxos.Compute()
	log.Info("初始化区块链成功")
}

// ComputeUTXOs 计算UTXOs
func (cli *CommandLine) ComputeUTXOs() {
	chain := cli.Blockchain.ContinueBlockchain()

	if cli.CloseDbAlways {
		defer chain.Database.Close()
	}
	utxos := blockchain.UTXOSet{Blockchain:chain}
	utxos.Compute()
	count := utxos.CountTransactions()
	log.Infof("重建完成!!!!, utxos集合中现有 %d 个交易", count)
}
// GetBalance 得到某个钱包地址的余额
func (cli *CommandLine) GetBalance(address string) BalanceResponse {
	if !wallet.ValidateAddress(address) {
		log.Panic("非法地址")
	}
	chain := cli.Blockchain.ContinueBlockchain()
	if cli.CloseDbAlways {
		defer chain.Database.Close()
	}
	balance := float64(0)
	publicKeyHash := wallet.Base58Decode([]byte(address))
	publicKeyHash = publicKeyHash[1 : len(publicKeyHash)-4]
	utxos := blockchain.UTXOSet{Blockchain:chain}

	UTXOs := utxos.FindUnSpentTransactions(publicKeyHash)
	for _, out := range UTXOs {
		balance += out.Value
	}

	log.Infof("%s的余额是:%f\n", address, balance)

	return BalanceResponse{
		balance,
		address,
		time.Now().Unix(),
		&Error{},
	}
}

// CreateWallet 创建一个钱包
func (cli *CommandLine) CreateWallet(instanceId string) string {
	cwd := false
	wallets, _ := wallet.InitializeWallets(cwd,instanceId)
	address := wallets.AddWallet()
	wallets.SaveFile(cwd)

	log.Info("钱包地址:", address)
	return address
}

// ListAddresses 列出所有钱包地址
func (cli *CommandLine) ListAddresses(instanceId string) {
	cwd := false
	wallets, err := wallet.InitializeWallets(cwd,instanceId)
	if err != nil {
		log.Panic(err)
	}
	addresses := wallets.GetAllAddress()

	for _, address := range addresses {
		fmt.Println(address)
	}
}

// PrintBlockchain 打印区块链
func (cli *CommandLine) PrintBlockchain() {
	chain := cli.Blockchain.ContinueBlockchain()
	if cli.CloseDbAlways {
		defer chain.Database.Close()
	}
	iter := chain.Iterator()

	for {
		block := iter.Next()
		fmt.Printf("PrevHash: %x\n", block.PrevHash)
		fmt.Printf("Hash: %x\n", block.Hash)
		fmt.Printf("Height: %d\n", block.Height)
		pow := blockchain.NewProof(block)
		validate := pow.Validate()
		fmt.Printf("Valid: %s\n", strconv.FormatBool(validate))
		for _, tx := range block.Transactions {
			fmt.Println(tx)
		}
		fmt.Println()

		if len(block.PrevHash) == 0 {
			break
		}
	}
}

// GetBlockchain 得到区块链中的所有区块
func (cli *CommandLine) GetBlockchain() []*blockchain.Block {
	var blocks []*blockchain.Block
	chain := cli.Blockchain.ContinueBlockchain()
	if cli.CloseDbAlways {
		defer chain.Database.Close()
	}
	iter := chain.Iterator()

	for {
		block := iter.Next()
		blocks = append(blocks, block)

		if len(block.PrevHash) == 0 {
			break
		}
	}

	return blocks
}

// GetBlockByHeight 根据高度值获得区块
func (cli *CommandLine) GetBlockByHeight(height int) blockchain.Block {
	var block blockchain.Block
	chain := cli.Blockchain.ContinueBlockchain()
	if cli.CloseDbAlways {
		defer chain.Database.Close()
	}
	iter := chain.Iterator()

	for {
		block = *iter.Next()
		if block.Height == height-1 {
			return block
		}
		if len(block.PrevHash) == 0 {
			break
		}
	}

	return block
}
