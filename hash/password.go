package hash

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// 默认 Argon2id 参数。
// 取自 OWASP 推荐起点的偏保守配置（memory 64MB）。
// 若服务器内存充裕、想更抗暴破，可调高 argonMemory（cost 线性上升）。
//
// 注意：这些是当前“推荐值”，NeedsRehash 会以它们为基准判断旧哈希是否该升级。
const (
	argonTime    uint32 = 1
	argonMemory  uint32 = 64 * 1024 // 64 MB
	argonThreads uint8  = 4
	argonKeyLen  uint32 = 32
	argonSaltLen uint32 = 16
)

// Argon2idHash 用 Argon2id 对明文密码做加盐哈希，返回自描述格式串：
//
//	$argon2id$v=19$m=65536,t=1,p=4$<base64盐>$<base64哈希>
//
// 盐随机生成，调用方无需单独存盐；整串直接落库即可。
func Argon2idHash(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("生成随机盐失败: %w", err)
	}
	sum := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Sum := base64.RawStdEncoding.EncodeToString(sum)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads, b64Salt, b64Sum), nil
}

// Argon2idCheck 校验明文是否匹配 Argon2id 哈希串（恒定时间比较，防时序攻击）。
func Argon2idCheck(encodedHash, password string) (bool, error) {
	// 期望格式：$argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash>
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("argon2id 哈希格式非法")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, fmt.Errorf("解析 argon2 版本失败: %w", err)
	}
	if version != argon2.Version {
		return false, fmt.Errorf("argon2 版本不匹配: 期望 %d, 实际 %d", argon2.Version, version)
	}

	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false, fmt.Errorf("解析 argon2 参数失败: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("解码盐失败: %w", err)
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("解码哈希失败: %w", err)
	}

	actual := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

// HashPassword 是当前推荐的密码哈希入口，默认使用 Argon2id。
// 新注册、改密、以及老账户迁移重写都走它。
func HashPassword(password string) (string, error) {
	return Argon2idHash(password)
}

// VerifyPassword 根据哈希串前缀自动分派校验算法：
//   - "$argon2id$"        -> Argon2idCheck
//   - "$2a$"/"$2b$"/"$2y$" -> bcrypt（兼容老 bcrypt 账户）
//   - 其它（含 32 位 hex、老 md5 串）-> 一律不匹配
//
// 说明：老 Discuz 的 md5(md5(pw)+salt) 不是自描述格式，必须带上 salt 单独处理，
// 见 rugaо 的 auth.VerifyLegacyDiscuz。这里对无法识别的格式返回 false 是安全的。
func VerifyPassword(encodedHash, password string) bool {
	switch {
	case strings.HasPrefix(encodedHash, "$argon2id$"):
		ok, _ := Argon2idCheck(encodedHash, password)
		return ok
	case strings.HasPrefix(encodedHash, "$2a$") ||
		strings.HasPrefix(encodedHash, "$2b$") ||
		strings.HasPrefix(encodedHash, "$2y$"):
		return BcryptCheck(password, encodedHash)
	default:
		return false
	}
}

// NeedsRehash 判断某哈希是否需要重写为当前推荐的 Argon2id：
//   - 非 argon2id（bcrypt / 未知格式）-> true，应升级
//   - argon2id 且参数与当前默认一致     -> false
//   - argon2id 但参数偏低               -> true，应提高成本
func NeedsRehash(encodedHash string) bool {
	if !strings.HasPrefix(encodedHash, "$argon2id$") {
		return true
	}
	parts := strings.Split(encodedHash, "$")
	if len(parts) < 4 {
		return true
	}
	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return true
	}
	return memory != argonMemory || time != argonTime || threads != argonThreads
}
