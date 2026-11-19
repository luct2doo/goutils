package hash

import (
	"crypto/md5"
	"encoding/hex"
	"strings"

	"github.com/luct2doo/goutils/logger"

	"golang.org/x/crypto/bcrypt"
)

func BcryptHash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost+4)
	if err != nil {
		logger.LogIf(err)
		// 不要吞掉错误：失败时返回空哈希 + nil 会让调用方把空串写进密码字段。
		return "", err
	}
	return string(hash), nil
}

// BcryptCheck 对比明文密码和数据库的哈希值
func BcryptCheck(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// BcryptIsHashed 判断字符串是否是 bcrypt 哈希。
// 只看长度会把任意 60 字符的普通文本误判为哈希，这里改为校验版本前缀。
func BcryptIsHashed(str string) bool {
	return strings.HasPrefix(str, "$2a$") ||
		strings.HasPrefix(str, "$2b$") ||
		strings.HasPrefix(str, "$2y$")
}

func MD5Hash(str string) string {
	h := md5.New()
	h.Write([]byte(str))
	return hex.EncodeToString(h.Sum(nil))
}
