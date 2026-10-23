package limiter

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	memorystore "github.com/ulule/limiter/v3/drivers/store/memory"
)

// newMemoryLimiter 用内存 store 构造一个不依赖 Redis 的限流器，只用于单测。
func newMemoryLimiter() *Limiter {
	return &Limiter{store: memorystore.NewStore()}
}

func newTestContext(fullPath string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, fullPath, nil)
	c.Set("limiter-once", false)
	return c
}

// TestCheckRate_StoreIsPointer 回归测试：
// limiterlib.New 返回 *Limiter，此前 sync.Map 里存的是指针、断言却写成值类型，
// 导致第一次调用 CheckRate 就 panic:
// interface conversion: interface {} is *limiter.Limiter, not limiter.Limiter
func TestCheckRate_StoreIsPointer(t *testing.T) {
	l := newMemoryLimiter()
	c := newTestContext("/v1/auth/login")

	rate, err := l.CheckRate(c, "1.2.3.4", "2-M")
	if err != nil {
		t.Fatalf("首次调用不应报错: %v", err)
	}
	if rate.Reached {
		t.Fatalf("第 1 次请求不应触发限流, context=%+v", rate)
	}
}

// TestCheckRate_ReachLimit 校验超额判定与「同一 rate 复用缓存实例」都不会出问题。
func TestCheckRate_ReachLimit(t *testing.T) {
	l := newMemoryLimiter()

	for i := 1; i <= 2; i++ {
		c := newTestContext("/v1/auth/login")
		rate, err := l.CheckRate(c, "1.2.3.4", "2-M")
		if err != nil {
			t.Fatalf("第 %d 次调用报错: %v", i, err)
		}
		if rate.Reached {
			t.Fatalf("第 %d 次请求不应触发限流, context=%+v", i, rate)
		}
	}

	c := newTestContext("/v1/auth/login")
	rate, err := l.CheckRate(c, "1.2.3.4", "2-M")
	if err != nil {
		t.Fatalf("第 3 次调用报错: %v", err)
	}
	if !rate.Reached {
		t.Fatalf("第 3 次请求应触发限流（2 次/分钟）, context=%+v", rate)
	}
}

// TestCheckRate_NilStore 未初始化（Redis 连不上）时应返回错误而不是 panic。
func TestCheckRate_NilStore(t *testing.T) {
	l := &Limiter{}
	c := newTestContext("/v1/auth/login")

	if _, err := l.CheckRate(c, "1.2.3.4", "2-M"); err == nil {
		t.Fatal("store 为空时应返回错误")
	}
}
