package helpers

import (
	"strings"
	"testing"
)

func TestRandomString(t *testing.T) {
	if got := RandomString(0); got != "" {
		t.Errorf("RandomString(0) = %q, want empty", got)
	}
	if got := RandomString(-3); got != "" {
		t.Errorf("RandomString(-3) = %q, want empty", got)
	}

	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	for _, n := range []int{1, 16, 64} {
		got := RandomString(n)
		if len(got) != n {
			t.Fatalf("RandomString(%d) len = %d", n, len(got))
		}
		for _, r := range got {
			if !strings.ContainsRune(letters, r) {
				t.Fatalf("RandomString(%d) 含非字母字符 %q", n, r)
			}
		}
	}

	// 连续两次不应相同（crypto/rand 落回全局 math/rand 的概率极低）
	if a, b := RandomString(32), RandomString(32); a == b {
		t.Error("连续两次 RandomString(32) 相同，随机源可疑")
	}
}

func TestNewIPAPIProvider(t *testing.T) {
	p := NewIPAPIProvider("", nil)
	if p.BaseURL != DefaultIPAPIBaseURL {
		t.Errorf("BaseURL = %q, want %q", p.BaseURL, DefaultIPAPIBaseURL)
	}
	if p.Client == nil {
		t.Error("Client 为 nil，应使用默认带超时的 client")
	}
	if p.Client.Timeout <= 0 {
		t.Errorf("默认 client Timeout = %v, want > 0", p.Client.Timeout)
	}

	p2 := NewIPAPIProvider("https://example.com/api/", nil)
	if p2.BaseURL != "https://example.com/api" {
		t.Errorf("结尾斜杠未去掉: %q", p2.BaseURL)
	}

	// 本地回环不走网络
	if got := p2.Lookup("127.0.0.1"); got != "本地登录" {
		t.Errorf("Lookup(127.0.0.1) = %q", got)
	}
	if got := p2.Lookup("::1"); got != "本地登录" {
		t.Errorf("Lookup(::1) = %q", got)
	}
}
