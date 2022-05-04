package env

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/joho/godotenv"
)

var (
	_, b, _, _ = runtime.Caller(0)

	// 项目根目录
	Root = filepath.Join(filepath.Dir(b), "../..")
)

type Config struct {
	WalletAddressChecksum int
	MinerAddress          string//矿工钱包地址
	ListenPort            string//监听端口
	Miner                 bool//是否是矿工节点
	FullNode              bool//是否是全节点
}

func New() *Config {
	return &Config{
		WalletAddressChecksum: getEnvAsInt("WALLET_ADDRESS_CHECKSUM", 1),
		MinerAddress:          getEnvAsStr("MINER_ADDRESS", ""),
		ListenPort:            getEnvAsStr("LISTEN_PORT", ""),
		Miner:                 getEnvAsBool("MINER", false),
		FullNode:              getEnvAsBool("FULL_NODE", false),
	}
}

func GetEnvVariable(key string) string {
	// 加载.env文件
	err := godotenv.Load(Root + "/.env")
	if err != nil {
		log.Fatalf("加载.env文件失败")
	}

	return os.Getenv(key)
}

func getEnv(key string, defaultVal string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}

	return defaultVal
}

func getEnvAsInt(name string, defaultVal int) int {
	valueStr := GetEnvVariable(name)
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}

	return defaultVal
}
func getEnvAsStr(name string, defaultVal string) string {
	valueStr := GetEnvVariable(name)
	if valueStr != "" {
		return valueStr
	}

	return defaultVal
}

func getEnvAsBool(name string, defaultVal bool) bool {
	valueStr := GetEnvVariable(name)
	if value, err := strconv.ParseBool(valueStr); err == nil {
		return value
	}

	return defaultVal
}
