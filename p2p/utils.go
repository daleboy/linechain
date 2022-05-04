package p2p

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	mrand "math/rand"
	"net"
	"os"
	"regexp"
	"strconv"
	"sync"

	p2p_crypto "github.com/libp2p/go-libp2p-core/crypto"
	log "github.com/sirupsen/logrus"
)
var lock sync.Mutex
var privateNets []*net.IPNet

// PrivKeyStore is used to persist private key to/from file
type PrivKeyStore struct {
	Key string `json:"key"`
}
// CmdToBytes 返回类型为切片
func CmdToBytes(cmd string) []byte {
	var bytes [commandLength]byte//bytes是一个数组，声明数组时所有的元素都会被自动初始化为默认值 0

	//使用ruin类型（用它来区分字符值和整数值），go用于处理字符串的便捷方法
	//注意这里不能直接将cmd转为[]byte（使用GobEncode），
	//因为首先cmd有严格的长度要求，另外cmd在对方接收到后需要能解析出来（实际的命令内容是cmd+payload）
	for i, c := range cmd {//每一个ruin代表一个完整的字符（不管是中文字符还是英文字符）
		bytes[i] = byte(c)
	}
	//返回切片
	return bytes[:]//=bytes[0:commandLength],如果cmd转为byte后长度不足元素为0
}

func BytesToCmd(bytes []byte) string {
	var cmd []byte
	for _, b := range bytes {
		if b != byte(0) {//不解析无效的字节（0）
			cmd = append(cmd, b)
		}
	}
	return fmt.Sprintf("%s", cmd)
}

// GobEncode 将任何类型的数据转为[]byte
func GobEncode(data interface{}) []byte {
	var buff bytes.Buffer

	enc := gob.NewEncoder(&buff)
	err := enc.Encode(data)
	if err != nil {
		log.Panic(err)
	}

	return buff.Bytes()
}
// GenKeyP2PRand generates a pair of RSA keys used in libp2p host, using random seed
func GenKeyP2PRand() (p2p_crypto.PrivKey, p2p_crypto.PubKey, error) {
	return p2p_crypto.GenerateKeyPair(p2p_crypto.RSA, 2048)
}
// Unmarshal is a function that unmarshals the data from the
// reader into the specified value.
func Unmarshal(r io.Reader, v interface{}) error {
	return json.NewDecoder(r).Decode(v)
}

// Marshal is a function that marshals the object into an
// io.Reader.
func Marshal(v interface{}) (io.Reader, error) {
	b, err := json.MarshalIndent(v, "", "\t")
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}

// GetUniqueIDFromIPPort --
func GetUniqueIDFromIPPort(ip, port string) uint32 {
	reg, _ := regexp.Compile("[^0-9]+")
	socketID := reg.ReplaceAllString(ip+port, "") // A integer Id formed by unique IP/PORT pair
	value, _ := strconv.Atoi(socketID)
	return uint32(value)
}
// Save saves a representation of v to the file at path.
func Save(path string, v interface{}) error {
	lock.Lock()
	defer lock.Unlock()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r, err := Marshal(v)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, r)
	return err
}
// GenKeyP2P generates a pair of RSA keys used in libp2p host
func GenKeyP2P(ip, port string) (p2p_crypto.PrivKey, p2p_crypto.PubKey, error) {
	r := mrand.New(mrand.NewSource(int64(GetUniqueIDFromIPPort(ip, port))))
	return p2p_crypto.GenerateKeyPairWithReader(p2p_crypto.RSA, 2048, r)
}
// Load loads the file at path into v.
func Load(path string, v interface{}) error {
	lock.Lock()
	defer lock.Unlock()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return err
		}
	}
	defer f.Close()
	return Unmarshal(f, v)
}
// SaveKeyToFile save private key to keyfile
func SaveKeyToFile(keyfile string, key p2p_crypto.PrivKey) (err error) {
	str, err := SavePrivateKey(key)
	if err != nil {
		return
	}

	keyStruct := PrivKeyStore{Key: str}

	err = Save(keyfile, &keyStruct)
	return
}
// SavePrivateKey convert the PrivKey to base64 format and return string
func SavePrivateKey(key p2p_crypto.PrivKey) (string, error) {
	if key != nil {
		b, err := p2p_crypto.MarshalPrivateKey(key)
		if err != nil {
			return "", fmt.Errorf("failed to marshal private key: %v", err)
		}
		str := p2p_crypto.ConfigEncodeKey(b)
		return str, nil
	}
	return "", fmt.Errorf("key is nil")
}
// LoadPrivateKey parses the key string in base64 format and return PrivKey
func LoadPrivateKey(key string) (p2p_crypto.PrivKey, p2p_crypto.PubKey, error) {
	if key != "" {
		k1, err := p2p_crypto.ConfigDecodeKey(key)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decode key: %v", err)
		}
		priKey, err := p2p_crypto.UnmarshalPrivateKey(k1)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal private key: %v", err)
		}
		pubKey := priKey.GetPublic()
		return priKey, pubKey, nil
	}
	return nil, nil, fmt.Errorf("empty key string")
}
// LoadKeyFromFile load private key from keyfile
// If the private key is not loadable or no file, it will generate
// a new random private key
func LoadKeyFromFile(keyfile string) (key p2p_crypto.PrivKey, pk p2p_crypto.PubKey, err error) {
	var keyStruct PrivKeyStore
	err = Load(keyfile, &keyStruct)
	if err != nil {
		Logger().Info().
			Str("keyfile", keyfile).
			Msg("No private key can be loaded from file")
		Logger().Info().Msg("Using random private key")
		key, pk, err = GenKeyP2PRand()
		if err != nil {
			Logger().Error().
				AnErr("GenKeyP2PRand Error", err).
				Msg("LoadedKeyFromFile")
			panic(err)
		}
		err = SaveKeyToFile(keyfile, key)
		if err != nil {
			Logger().Error().
				AnErr("keyfile", err).
				Msg("failed to save key to keyfile")
		}
		return key, pk, nil
	}
	key, pk, err = LoadPrivateKey(keyStruct.Key)
	return key, pk, err
}
