package main

import (
	"fmt"
	"log"
	"strings"

	"linechain/wallet"

	"github.com/spf13/cobra"
)
var instanceId string//并非必须
const (
	cwd = true
)

func PrintWalletAddress(address string, w wallet.Wallet) {
	var lines []string
	lines = append(lines, fmt.Sprintf("======ADDRESS:======\n %s ", address))
	lines = append(lines, fmt.Sprintf("======PUBLIC KEY:======\n %x", w.PublicKey))
	lines = append(lines, fmt.Sprintf("======PRIVATE KEY:======\n %x", w.PrivateKey.D.Bytes()))
	fmt.Println(strings.Join(lines, "\n"))
}

func main() {
	var cmdNew = &cobra.Command{
		Use:   "new",
		Short: "生成新钱包并打印钱包地址",
		Run: func(cmd *cobra.Command, args []string) {
			wallets, _ := wallet.InitializeWallets(cwd,instanceId)
			address := wallets.AddWallet()
			wallets.SaveFile(cwd)
			w , _ := wallets.GetWallet(address)
			PrintWalletAddress(address, w)
		},
	}
	var Address string
	var cmdPrint = &cobra.Command{
		Use:   "print",
		Short: "打印钱包地址",
		Run: func(cmd *cobra.Command, args []string) {
			var w wallet.Wallet
			var address string
			wallets, _ := wallet.InitializeWallets(cwd,instanceId)
			if Address != "" {
				if !wallet.ValidateAddress(Address) {
					log.Panic("非法地址")
				}
				w , _ = wallets.GetWallet(Address)
				PrintWalletAddress(Address, w)
			} else {
				count := 1
				for address = range wallets.Wallets {
					w = *wallets.Wallets[address]
					fmt.Println("")
					PrintWalletAddress(address, w)
					count++
				}
			}
		},
	}

	// 从命令行参数中获取打印命令参数
	cmdPrint.PersistentFlags().StringVar(&Address, "address", "", "钱包地址")
	
	var rootCmd = &cobra.Command{Use: "wallet"}
	rootCmd.PersistentFlags().StringVar(&instanceId, "instanceid", "", "与Blockchain实例id对应，这里是为指定钱包目录")
	rootCmd.AddCommand(cmdNew, cmdPrint)
	rootCmd.Execute()
}
