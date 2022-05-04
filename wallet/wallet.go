package wallet

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"

	"linechain/util/env"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ripemd160"
)

var conf = env.New()
var (
	checkSumlength = conf.WalletAddressChecksum
	version        = byte(0x00) // //钱包版本，一个字节（0的16进制表示）
)

//Wallet 钱包保存公钥和私钥对
type Wallet struct {
	//椭圆曲线数字算法
	PrivateKey ecdsa.PrivateKey
	PublicKey  []byte
}

// 验证 Wallet Address是否合法
func ValidateAddress(address string) bool {

	if len(address) != 34 {
		return false
	}
	//将 address 转为公钥哈希
	fullHash := Base58Decode([]byte(address))
	//得到Address的 checkSum
	checkSumFromHash := fullHash[len(fullHash)-checkSumlength:]
	//得到 version
	version := fullHash[0]
	pubKeyHash := fullHash[1 : len(fullHash)-checkSumlength]
	checkSum := CheckSum(append([]byte{version}, pubKeyHash...))

	return bytes.Compare(checkSum, checkSumFromHash) == 0
}

// Address 返回钱包地址（可为人识别的地址，Base58编码的字符串）
func (w *Wallet) Address() []byte {
	pubHash := PublicKeyHash(w.PublicKey)
	versionedHash := append([]byte{version}, pubHash...)
	checksum := CheckSum(versionedHash)
	//version-publickeyHash-checksum
	fullHash := append(versionedHash, checksum...)
	address := Base58Encode(fullHash)

	return address
}

// 使用ecdsa生成新的公私钥对
func NewKeyPair() (ecdsa.PrivateKey, []byte) {
	curve := elliptic.P256()//ECDSA基于椭圆曲线，所以我们需要一个椭圆曲线

	private, err := ecdsa.GenerateKey(curve, rand.Reader)//产生私钥
	if err != nil {
		log.Panic(err)
	}

	//从私钥生成一个公钥
	//在基于椭圆曲线的算法中，公钥是曲线上的点，因此，公钥是 X，Y 坐标的组合
	pub := append(private.PublicKey.X.Bytes(), private.PublicKey.Y.Bytes()...)

	return *private, pub
}

// MakeWallet 创建并返回一个钱包
func MakeWallet() *Wallet {
	private, public := NewKeyPair()
	return &Wallet{private, public}
}

// PublicKeyHash 对公钥进行哈希
func PublicKeyHash(pubKey []byte) []byte {
	//使用sha256生成一个新的哈希
	pubHash := sha256.Sum256(pubKey)

	// 使用 ripemd160 再次哈希上面生成的sha256
	hasher := ripemd160.New()
	_, err := hasher.Write(pubHash[:])
	if err != nil {
		log.Panic(err)
	}

	publicRipMd := hasher.Sum(nil)
	return publicRipMd
}

// Checksum 根据公钥生成校验码
func CheckSum(data []byte) []byte {
	firstHash := sha256.Sum256(data)
	secondHash := sha256.Sum256(firstHash[:])

	//只取checkSumlength长度部分的切片作为校验码
	return secondHash[:checkSumlength]
}
