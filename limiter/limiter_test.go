package limiter

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	limiterlib "github.com/ulule/limiter/v3"
)

// stubStore 实现 limiterlib.Store，让 CheckRate 可以在没有 Redis 的情况下被测试。
type stubStore struct {
	gotCalled bool
	peeked    bool
}

func (s *stubStore) Get(ctx context.Context, key string, rate limiterlib.Rate) (limiterlib.Context, error) {
	s.gotCalled = true
	return limiterlib.Context{Limit: rate.Limit, Remaining: rate.Limit - 1}, nil
}

func (s *stubStore) Peek(ctx context.Context, key string, rate limiterlib.Rate) (limiterlib.Context, error) {
	s.peeked = true
	return limiterlib.Context{Limit: rate.Limit, Remaining: rate.Limit}, nil
}

func (s *stubStore) Reset(ctx context.Context, key string, rate limiterlib.Rate) (limiterlib.Context, error) {
	return limiterlib.Context{}, nil
}

func (s *stubStore) Increment(ctx context.Context, key string, count int64, rate limiterlib.Rate) (limiterlib.Context, error) {
	return limiterlib.Context{}, nil
}

func newTestContext() *gin.Context {
	gin.SetMode(gin.ReleaseMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/v1/users", nil)
	return c
}

// TestCheckRateTypeAssertion 回归测试：limiterlib.New 返回 *Limiter，
// 早期实现断言成值类型 limiterlib.Limiter，导致 CheckRate 必定 panic。
func TestCheckRateTypeAssertion(t *testing.T) {
	store := &stubStore{}
	l := &Limiter{store: store} // initErr 为 nil，走正常路径

	got, err := l.CheckRate(newTestContext(), "1.2.3.4", "10-M")
	if err != nil {
		t.Fatalf("CheckRate 返回错误: %v", err)
	}
	if !store.gotCalled {
		t.Fatal("首次调用应走 Store.Get")
	}
	if got.Limit != 10 {
		t.Errorf("Limit = %d, want 10", got.Limit)
	}
	if got.Remaining != 9 {
		t.Errorf("Remaining = %d, want 9", got.Remaining)
	}

	// 第二次调用（同一个 *gin.Context 已置 limiter-once）应走 Peek 而不是 Get
	store2 := &stubStore{}
	l2 := &Limiter{store: store2}
	c := newTestContext()
	if _, err := l2.CheckRate(c, "1.2.3.4", "10-M"); err != nil {
		t.Fatalf("第一次 CheckRate 失败: %v", err)
	}
	if _, err := l2.CheckRate(c, "1.2.3.4", "10-M"); err != nil {
		t.Fatalf("第二次 CheckRate 失败: %v", err)
	}
	if !store2.peeked {
		t.Error("同一上下文第二次调用应走 Store.Peek")
	}
}

// TestCheckRateReturnsInitError store 初始化失败时必须返回 error，而不是继续走到 nil 解引用。
func TestCheckRateReturnsInitError(t *testing.T) {
	boom := errors.New("redis 不可用")
	l := &Limiter{initErr: boom}

	_, err := l.CheckRate(newTestContext(), "1.2.3.4", "10-M")
	if err == nil {
		t.Fatal("store 初始化失败时 CheckRate 应返回错误")
	}
	if !errors.Is(err, boom) {
		t.Errorf("错误未被包装保留: %v", err)
	}
}

// TestCheckRateBadFormatted 非法限流格式应返回错误而不是 panic。
func TestCheckRateBadFormatted(t *testing.T) {
	l := &Limiter{store: &stubStore{}}
	if _, err := l.CheckRate(newTestContext(), "1.2.3.4", "not-a-rate"); err == nil {
		t.Fatal("非法 formatted 应返回错误")
	}
}
