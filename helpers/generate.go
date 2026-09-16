package helpers

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	nanoid "github.com/matoous/go-nanoid/v2"
)

func GenerateUUID() string {
	return uuid.New().String()
}

// 创建时：生成无横杠 UUID
func GenerateShortUUID(uuid string) string {
	return strings.ReplaceAll(uuid, "-", "")
}

// 查询时：后端自动补回横杠再解析（或直接存32位到数据库）
func ParseUUID(words string) (uuid.UUID, error) {
	// 兼容 32 位和 36 位输入
	if len(words) == 32 {
		// 手动插入短横杠还原标准格式
		words = fmt.Sprintf("%s-%s-%s-%s-%s", words[0:8], words[8:12], words[12:16], words[16:20], words[20:])
	}
	return uuid.Parse(words)
}

func GenerateNanoID(length int) string {
	// 1. 拦截程序员错误（参数不合法）
	if length <= 0 {
		panic(fmt.Errorf("invalid nanoid length: %d, must be > 0", length))
	}

	id, err := nanoid.New(length)
	if err != nil {
		// 2. 系统级致命错误，直接 panic 并保留堆栈信息
		panic(fmt.Errorf("critical: failed to generate nanoid: %w", err))
	}
	return id
}
