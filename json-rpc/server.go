package rpc

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"

	"linechain/console/utils"
	blockchain "linechain/core"
	appUtils "linechain/util/utils"

	log "github.com/sirupsen/logrus"
)

var (
	port = "5000"
)

type API struct {
	RPCEnabled bool
	cmd        *utils.CommandLine
}

type HttpConn struct {
	in  io.Reader
	out io.Writer
}

func (conn *HttpConn) Read(p []byte) (n int, err error) {
	return conn.in.Read(p)
}
func (conn *HttpConn) Write(d []byte) (n int, err error) {
	return conn.out.Write(d)
}
func (conn *HttpConn) Close() error {
	return nil
}
/**
* instanceId 与blockchain实例保持一致，这里是为了指定钱包目录
**/
func (api *API) CreateWallet(args Args,instanceId string, address *string) error {
	*address = api.cmd.CreateWallet(instanceId)
	return nil
}

func (api *API) GetBalance(args Args, balance *utils.BalanceResponse) error {
	*balance = api.cmd.GetBalance(args.Address)
	return nil
}

func (api *API) GetBlockchain(args Args, data *Blocks) error {
	*data = api.cmd.GetBlockchain()
	return nil
}

func (api *API) GetBlockByHeight(args BlockArgs, data *blockchain.Block) error {
	*data = api.cmd.GetBlockByHeight(args.Height)
	return nil
}

func (api *API) Send(args SendArgs, data *utils.SendResponse) error {
	*data = api.cmd.Send(args.SendFrom, args.SendTo, args.Amount, args.Mine)
	return nil
}

// StartServer 启动节点RPC服务，默认的 rpcPort 为5000
// 注意，节点的RPC服务并非是节点服务的必须，节点是否提供了RPC服务，由启动节点时提供的rpc参数决定（等同于StartServer的rpcEnabled参数）
// 因为 StartServer 是被cli.StartNode调用的，而 StartServer 被调用的时机是以 rpcEnabled=true 为前提条件
// 因此这里的 StartServer 中的参数rpcEnabled一定是 true
// 实际上，在函数StartServer体中，rpcEnabled的值的意义没有实际被用到
func StartServer(cli *utils.CommandLine, rpcEnabled bool, rpcPort string, rpcAddr string) {
	if rpcPort != "" {
		port = rpcPort
	}

	publicAPI := &API{
		rpcEnabled,
		cli,
	}
	defer cli.Blockchain.Database.Close()

	// 启动协程，程序退出后进行资源清理
	go appUtils.CloseDB(cli.Blockchain)

	//服务器需要注册对象实例， 通过对象的类型名暴露服务。
	//注册后这个对象的输出方法就可以远程调用，rpc库封装了底层传输的细节，包括序列化(默认Gob序列化器)
	//Go 的 RPC 和传统的 RPC 系统不同，它只支持 Go 开发的服务器与客户端之间的交互（因为在内部采用了 Gob 来编码）
	err := rpc.Register(publicAPI)//通过 API 的实例，将 API 的方法注册在 DefaultServer 中
	checkError("注册API出错:", err)

	//将 RPC 服务绑定到 HTTP 服务中去，利用HTTP传输数据（而不是直接通过TCP）
	rpc.HandleHTTP()

	tcpAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%s", rpcAddr, port))
	checkError("解析监听TCP地址和端口错误:", err)

	listener, err := net.ListenTCP("tcp", tcpAddr)
	checkError("监听服务启动错误:", err)

	//网站 root 路由
	//可通过网页打开http://localhost:port/来测试
	http.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		io.WriteString(res, "RPC服务正在运行!")
	})
	log.Infof("rpc服务运行于端口: %s", port)

	//循环处理来自客户端的请求
	for {
		conn, err := listener.Accept()//阻塞，直到有客户端连接进来
		if err != nil {
			continue
		}
		jsonrpc.ServeConn(conn)
		http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			//URL:[scheme:][//[userinfo@]host][/]path[?query][#fragment]
			if r.URL.Path == "/_jsonrpc" {
				serverCodec := jsonrpc.NewServerCodec(&HttpConn{in: r.Body, out: w})
				w.Header().Set("Content-type", "application/json")
				w.WriteHeader(200)
				err := rpc.ServeRequest(serverCodec)
				if err != nil {
					log.Errorf("在服务于JSON请求时出错: %v", err)
					http.Error(w, "在服务于JSON请求时出错", 500)
					return
				}
			}
		}))
	}

}

func checkError(message string, err error) {
	if err != nil {
		log.Info(message, err.Error())
		os.Exit(1)
	}
}
