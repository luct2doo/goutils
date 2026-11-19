package limiter

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCheckRateSkipsWhenFormatEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	l := &Limiter{}
	ctx, err := l.CheckRate(c, "1.2.3.4", "")
	if err != nil {
		t.Fatalf("空 rate 应视为不限流，实际错误: %v", err)
	}
	if ctx.Reached {
		t.Error("空 rate 不应判为超额")
	}
}
