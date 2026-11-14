package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

// vaultIDFile 保存仓库在系统密钥库中的随机标识
const vaultIDFile = "vault.id"

// blobVersion 标识使用随机 nonce 的 AES-GCM 资源格式
const blobVersion byte = 1

// Open 使用系统密钥库解锁历史记录和资源
func Open(dir string) (*Store, error) {
	secrets := mygo.Secrets.Service("dev.gopaste.app")
	key, err := historyKey(dir, secrets.Get, secrets.Set)
	if err != nil {
		return nil, fmt.Errorf("解锁剪贴板历史失败：%w", err)
	}
	defer clear(key)
	return OpenWithKey(dir, key)
}

// historyKey 取回仓库主密钥并仅为首次初始化的仓库创建密钥
func historyKey(dir string, get func(string) ([]byte, error), set func(string, []byte) error) ([]byte, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, vaultIDFile)
	id, err := os.ReadFile(path)
	if err == nil {
		if _, err := hex.DecodeString(string(id)); err != nil || len(id) != 32 {
			return nil, errors.New("仓库标识损坏")
		}
		return get("history-" + string(id))
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, name := range []string{databaseFile, encryptedBlobs} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return nil, errors.New("已有加密历史缺少仓库标识")
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	var randomID [16]byte
	rand.Read(randomID[:])
	id = []byte(hex.EncodeToString(randomID[:]))
	key := make([]byte, 32)
	rand.Read(key)
	if err := set("history-"+string(id), key); err != nil {
		return nil, err
	}
	if err := writeAtomic(path, id); err != nil {
		return nil, err
	}
	return key, nil
}

// vaultCipher 分离数据库密钥、资源密钥和资源文件名密钥
type vaultCipher struct {
	database []byte
	blob     cipher.AEAD
	names    []byte
}

// newVaultCipher 从仓库主密钥派生独立用途的密钥
func newVaultCipher(master []byte) (vaultCipher, error) {
	if len(master) != 32 {
		return vaultCipher{}, errors.New("剪贴板历史密钥长度无效")
	}
	derive := func(label string) []byte {
		key, _ := hkdf.Key(sha256.New, master, nil, "GoPaste/v1/"+label, 32)
		return key
	}
	block, err := aes.NewCipher(derive("blob"))
	if err != nil {
		return vaultCipher{}, err
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	return vaultCipher{database: derive("database"), blob: aead, names: derive("name")}, err
}

// diskName 为资源逻辑名称生成仅限当前仓库的文件名
func (v vaultCipher) diskName(name string) string {
	h := hmac.New(sha256.New, v.names)
	h.Write([]byte(name))
	return hex.EncodeToString(h.Sum(nil)) + ".bin"
}
