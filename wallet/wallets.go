package wallet

import (
	"bytes"
	"crypto/elliptic"
	"encoding/gob"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"

	log "github.com/sirupsen/logrus"
)

var (
	_, b, _, _ = runtime.Caller(0)

	// 项目的根目录
	Root            = filepath.Join(filepath.Dir(b), "../")
	walletsPath     = path.Join(Root, "/tmp/")//钱包目录
	walletsFilename = "mywallet.data"//钱包文件的扩展名
)

// Wallets 保存钱包集合，一个人可能有多个钱包
type Wallets struct {
	Wallets map[string]*Wallet
}
// InitializeWallets 从文件读取生成Wallets
func InitializeWallets(cwd bool,instanceId string) (*Wallets, error) {
	wallets := Wallets{map[string]*Wallet{}}
	err := wallets.LoadFile(cwd,instanceId)

	return &wallets, err
}
// GetWallet 根据地址返回一个钱包
func (ws *Wallets) GetWallet(address string) (Wallet, error) {
	var wallet *Wallet
	var ok bool
	w := *ws
	if wallet, ok = w.Wallets[address]; !ok {
		return *new(Wallet), errors.New("Invalid address")
	}

	return *wallet, nil
}

// AddWallet 创建并添加一个钱包到Wallets
func (ws *Wallets) AddWallet() string {
	wallet := MakeWallet()
	address := fmt.Sprintf("%s", wallet.Address())

	ws.Wallets[address] = wallet

	return address
}

// GetAddresses 从钱包文件中返回所有钱包的地址
func (ws *Wallets) GetAllAddress() []string {
	var addresses []string
	for address := range ws.Wallets {
		addresses = append(addresses, address)
	}
	return addresses
}
// LoadFile 从文件读取wallets
func (ws *Wallets) LoadFile(cwd bool,instanceId string) error {
	walletsFile := path.Join(walletsPath+instanceId+"/", walletsFilename)

	if cwd {
		dir, err := os.Getwd()
		if err != nil {
			log.Fatal(err)
		}
		walletsFile = path.Join(dir, walletsFilename)
	}

	if _, err := os.Stat(walletsFile); os.IsNotExist(err) {
		return err
	}
	var wallets Wallets
	fileContent, err := ioutil.ReadFile(walletsFile)
	if err != nil {
		return err
	}

	//当编解码中有一个字段是interface{}的时候，需要对interface{}具体实现的类型进行注册
	//字段elliptic.Curve是一个interface，具体实现的类型是elliptic.P256()
	gob.Register(elliptic.P256())
	decoder := gob.NewDecoder(bytes.NewReader(fileContent))
	err = decoder.Decode(&wallets)
	if err != nil {
		return err
	}

	ws.Wallets = wallets.Wallets

	return nil
}

// SaveToFile 保存wallets到文件
func (ws *Wallets) SaveFile(cwd bool) {
	walletsFile := path.Join(walletsPath, walletsFilename)

	if cwd {
		dir, err := os.Getwd()
		if err != nil {
			log.Fatal(err)
		}
		walletsFile = path.Join(dir, walletsFilename)
	}
	var content bytes.Buffer

	//gob是Golang包自带的一个数据结构序列化的编码/解码工具，一种典型的应用场景就是RPC(remote procedure calls)
	//需要注意到是，发送方的结构和接受方的结构并不需要完全一致
	//当编解码中有一个字段是interface{}的时候需要对interface{}的可能产生的类型进行注册
    //Wallet的PrivateKey的结构体类型逐层分析下去，有一个结构体字段是priv.PublicKey.Curve，
	//其类型是elliptic.Curve，而elliptic.Curve是一个interface，实际上在产生wallet时候，
	//传递的具体实现类型是curve := elliptic.P256()，所以编码前需要注册具体的类型elliptic.P256()
	gob.Register(elliptic.P256())

	encoder := gob.NewEncoder(&content)
	err := encoder.Encode(ws)
	if err != nil {
		log.Panic(err)
	}

	err = ioutil.WriteFile(walletsFile, content.Bytes(), 0644)
	if err != nil {
		log.Panic(err)
	}
}
