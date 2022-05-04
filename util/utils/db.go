package utils

import (
	"os"
	"runtime"
	"syscall"

	blockchain "linechain/core"

	"github.com/vrecan/death/v3"
)

// CloseDB 关闭区块链数据库
// 同步执行：阻塞，直到收到程序强行终止信号关闭数据库，退出程序（一般遇到非常严重的业务逻辑错误时候调用，如检查出现了非法的区块）
// 异步执行：启动协程，如在程序运行过程中遇到程序强行终止信号，关闭数据库，退出程序（本程序有两处调用：StartNode和StartServer）
//
func CloseDB(chain *blockchain.Blockchain) {
	//death 管理应用程序的生命终止
	//syscall.SIGINT ctr+c触发 
	//syscall.SIGTERM 当前进程被kill(即收到SIGTERM)
	//os.Interrupt 确保在所有系统上的os软件包中存在的两个信号值是os.Interrupt（向进程发送中断）和os.Kill（迫使进程退出）--
	//os.Interrupt 在Windows上，使用os.Process.Signal将os.Interrupt发送到进程的功能没有实现。 它会返回错误而不是发送信号
	d := death.NewDeath(syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	d.WaitForDeathWithFunc(func() {
		defer os.Exit(1)
		defer runtime.Goexit()
		chain.Database.Close()
	})
}
