package main

import (
	"fmt"
	"log"
	"net/rpc/jsonrpc"

	rpc "linechain/json-rpc"
)

//JSONRPC RPC client
//Go 标准包提供了对 RPC 的支持，且支持三个级别的 RPC：TCP、HTTP、JSONRPC
//这里没有通过API服务调用RPC，而是直接连接到RPC服务器
func main() {
	//假定RPC服务器运行在端口5000
	client, err := jsonrpc.Dial("tcp", "localhost:5000")
	if err != nil {
		log.Fatal("dialing:", err)
	}
	args := rpc.Args{
		Address: "14RwDN6Pj4zFUzdjiB8qUkVMC1QvRG5Cmr",
	}
	var bs rpc.Blocks
	err = client.Call("API.GetBlockchain", args, &bs)//远程调用RPC服务器进程的方法
	if err != nil {
		log.Fatal("API error:", err.Error())
	}
	for _, block := range bs {
		fmt.Printf("%x", block.PrevHash)
	}
}
