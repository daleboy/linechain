package main

import (
	"fmt"
	"os"

	"linechain/console/utils"
	blockchain "linechain/core"
	jsonrpc "linechain/json-rpc"
	"linechain/p2p"
	"linechain/util/env"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func main() {
	defer os.Exit(0)
	var conf = env.New()
	var address string
	var instanceId string

	var rpcPort string
	var rpcAddr string
	var rpc bool

	cli := utils.CommandLine{
		Blockchain: &blockchain.Blockchain{
			Database:   nil,
			InstanceId: instanceId,
		},
		Network: nil,
	}

	// 说明：下面每一个命令均各自独立，但部分命令执行的前提是本地区块链数据库已经存在
	// 依赖于本地区块链数据库已经存在的命令，在执行过程中并没有检测本地区块链数据库是否存在
	// 因此在执行命令前，需要确保本地区块链数据库已经存在

	//下面每一个命令，如果与intanceid有关，必须在RUN调用方法执行实际命令之前，先调用cli := cli.UpdateInstance(instanceId, true)

	/*
	* INIT 命令，执行本地操作，与P2P网络无关
	 */
	var initCmd = &cobra.Command{
		Use:   "init",
		Short: "初始化区块链并创建创始区块",
		Args:  cobra.MinimumNArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			//执行init命令，必须提供 instanceid 参数，因为创建区块链数据库文件需要用到它
			// init执行完毕，cli的Blockchain的Database为nil
			cli := cli.UpdateInstance(instanceId, true)

			//创建全新的区块链
			cli.CreateBlockchain(address)
		},
	}

	/*
	* WALLET 命令，执行本地操作，与P2P网络无关
	 */
	var walletCmd = &cobra.Command{
		Use:   "wallet",
		Short: "管理多个钱包",
	}
	var newWalletCmd = &cobra.Command{
		Use:   "new",
		Short: "创建一个新的钱包",
		Args:  cobra.MinimumNArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			cli.CreateWallet(instanceId)
		},
	}

	var listWalletAddressCmd = &cobra.Command{
		Use:   "listaddress",
		Short: "列出所有可用的钱包地址",
		Args:  cobra.MinimumNArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			cli.ListAddresses(instanceId)
		},
	}
	var walletBalanceCmd = &cobra.Command{
		Use:   "balance",
		Short: "得到钱包地址的余额",
		Run: func(cmd *cobra.Command, args []string) {
			//执行 balance 命令，必须提供 instanceid 参数，因为需要读取本地区块链数据
			cli := cli.UpdateInstance(instanceId, true)
			cli.GetBalance(address)
		},
	}
	walletCmd.AddCommand(newWalletCmd, listWalletAddressCmd, walletBalanceCmd)

	/*
	* UTXOS 命令 执行本地操作，与P2P网络无关
	 */
	var computeutxosCmd = &cobra.Command{
		Use:   "computeutxos",
		Short: "重建和计算未花费交易输出",
		Args:  cobra.MinimumNArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			//执行 computeutxos 命令，必须提供 instanceid 参数，因为需要读取本地区块链数据
			cli := cli.UpdateInstance(instanceId, true)
			cli.ComputeUTXOs()
		},
	}
	/*
	* PRINT 命令 执行本地操作，与P2P网络无关
	 */
	var printCmd = &cobra.Command{
		Use:   "print",
		Short: "打印区块链中的所有区块",
		Args:  cobra.MinimumNArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			//执行 print 命令，必须提供 instanceid 参数，因为需要读取本地区块链数据
			cli := cli.UpdateInstance(instanceId, true)
			cli.PrintBlockchain()
		},
	}

	/*
	* NODE 命令 执行本地和网络操作操作，与P2P网络相关
	 */
	var minerAddress string
	var miner bool
	var fullNode bool
	var listenPort string
	var nodeCmd = &cobra.Command{
		Use:   "startnode",
		Short: "开始一个节点",
		Args:  cobra.MinimumNArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(miner)
			if miner && len(minerAddress) == 0 { //节点类型为矿工
				log.Fatalln("需要矿工地址 --address")
			}

			cli := cli.UpdateInstance(instanceId, false)
			cli.StartNode(listenPort, minerAddress, miner, fullNode, func(net *p2p.Network) { //最后一个参数是回调函数，获得net实例
				if rpc {
					//如果启用rpc，则启动节点后设置cli的P2P实例，net为启动节点函数的回调函数参数被回调后返回的Network实例
					//如果不启用rpc，则cli.P2p为nil
					cli.Network = net
					go jsonrpc.StartServer(cli, rpc, rpcPort, rpcAddr)
				}
			})
		},
	}
	//从命令行参数中读取命令所需的各参数，从config中读取默认参数
	nodeCmd.Flags().StringVar(&listenPort, "port", conf.ListenPort, "节点监听端口")
	nodeCmd.Flags().StringVar(&minerAddress, "address", conf.MinerAddress, "设置矿工钱包地址")
	nodeCmd.Flags().BoolVar(&miner, "miner", conf.Miner, "如果以矿工的身份加入网络，设置为true")
	nodeCmd.Flags().BoolVar(&fullNode, "fullnode", conf.FullNode, "如果以全节点身份加入网络，设置为true")

	/*
	* SEND 命令 执行本地和网络操作，与P2P网络相关
	 */
	var mine bool
	var sendFrom string
	var sendTo string
	var amount float64

	var sendCmd = &cobra.Command{
		Use:   "send",
		Short: "从本地钱包地址发送x数量的代币到另外一个地址",
		Args:  cobra.MinimumNArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			//发送代币命令与instanceid有关
			cli := cli.UpdateInstance(instanceId, true)
			cli.Send(sendFrom, sendTo, amount, mine)
		},
	}
	//从命令行参数中读取命令所需的各参数
	sendCmd.Flags().StringVar(&sendFrom, "sendfrom", "", "发送方的钱包地址")
	sendCmd.Flags().StringVar(&sendTo, "sendto", "", "接收方的钱包地址")
	sendCmd.Flags().Float64Var(&amount, "amount", float64(0), "将要发送到代币数量，注意为浮点数")
	sendCmd.Flags().BoolVar(&mine, "mine", false, "如果你想要你的节点马上挖矿该交易，设置为true")

	//rootCmd 命令 与P2P网络无关，与本地区块链相关
	//rootCmd 命令直接执行的只有一个方式：
	//linux下：./demon --rpc true --rpcport 4000 --instanceid INSTANCE_ID
	//windows下：demon.exe --rpc true --rpcport 4000 --instanceid INSTANCE_ID
	//注意，提供的命令实参如果是字符串，直接写字符串内容，不需要用引号括起来
	var rootCmd = &cobra.Command{
		Use: "demon",
		Run: func(cmd *cobra.Command, args []string) {
			//执行 demon 命令，必须提供 instanceid 参数，因为jsonrpc.StartServer需要读取本地区块链数据
			cli := cli.UpdateInstance(instanceId, true)

			if rpc {
				//启动后，第三方客户端可以通过jsonrpc访问本节点服务器的接口
				jsonrpc.StartServer(cli, rpc, rpcPort, rpcAddr)
			}
		},
	}
	//从命令行参数中读取命令所需的各参数
	rootCmd.PersistentFlags().StringVar(&address, "address", "", "钱包地址")

	/*
	* HTTP FLAGS
	 */
	//从命令行参数中读取命令所需的各参数
	//PersistentFlags为根命令保存的参数，可供它及它下面的子命令使用
	//根命令即可执行程序demon命令
	rootCmd.PersistentFlags().StringVar(&rpcPort, "rpcport", "", "HTTP-RPC服务器正在监听的端口 (默认: 5000)")
	rootCmd.PersistentFlags().StringVar(&rpcAddr, "rpcaddr", "", "HTTP-RPC服务器监听地址 (默认: localhost)")
	rootCmd.PersistentFlags().BoolVar(&rpc, "rpc", false, "启用HTTP-RPC服务器")

	//instanceid参数为必须参数，设置instanceId，这是唯一获取instanceid的地方
	rootCmd.PersistentFlags().StringVar(&instanceId, "instanceid", "", "Blockchain实例")
	rootCmd.MarkFlagRequired("instanceid")

	rootCmd.AddCommand(
		initCmd,
		walletCmd,
		computeutxosCmd,
		sendCmd,
		printCmd,
		nodeCmd,
	)

	//执行rootCmd
	rootCmd.Execute() //除非提供参数rpc且rpc为true，否则除了更新instanceID并读取本地区块链，不会执行任何操作
}
