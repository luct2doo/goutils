package hash

import (
	"crypto/md5"
	"encoding/hex"

	"github.com/luct2doo/goutils/logger"
	"golang.org/x/crypto/bcrypt"
)

// BcryptHash 生成密码哈希。
//
// 注意：bcrypt 对超过 72 字节的输入会报错，此时返回空字符串与具体 error，
// 调用方必须检查 error，不要只判断返回的字符串是否为空。
func BcryptHash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost+4)
	if err != nil {
		logger.LogIf(err)
		return "", err
	}
	return string(hash), nil
}

// BcryptCheck 对比明文密码和数据库的哈希值
func BcryptCheck(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func BcryptIsHashed(str string) bool {
	return len(str) == 60
}

func MD5Hash(str string) string {
	h := md5.New()
	h.Write([]byte(str))
	return hex.EncodeToString(h.Sum(nil))
}
