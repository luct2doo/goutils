package hash

import (
	"strings"
	"testing"
)

func TestBcryptHashReturnsErrorOnFailure(t *testing.T) {
	// bcrypt 只接受 72 字节以内，超长时必须把错误返回给调用方
	h, err := BcryptHash(strings.Repeat("a", 73))
	if err == nil {
		t.Fatal("超长密码应返回错误")
	}
	if h != "" {
		t.Errorf("失败时不应返回哈希，得到 %q", h)
	}
}

func TestBcryptIsHashed(t *testing.T) {
	h, err := BcryptHash("secret")
	if err != nil {
		t.Fatalf("BcryptHash: %v", err)
	}
	if !BcryptIsHashed(h) {
		t.Errorf("BcryptIsHashed(%q) = false", h)
	}
	if BcryptIsHashed(strings.Repeat("x", 60)) {
		t.Error("普通 60 字符文本不应被判定为 bcrypt 哈希")
	}
}

func TestArgon2idCheckRejectsHugeParams(t *testing.T) {
	// m 值过大时必须拒绝，不能按哈希串里的参数直接分配内存
	huge := "$argon2id$v=19$m=1048576,t=1,p=4$c2FsdA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	if ok, err := Argon2idCheck(huge, "x"); err == nil || ok {
		t.Errorf("超大 memory 参数应返回错误，ok=%v err=%v", ok, err)
	}
}
