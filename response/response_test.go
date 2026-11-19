package response

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newCtx() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

func TestBadRequestWithNilError(t *testing.T) {
	c, w := newCtx()
	BadRequest(c, nil) // 回归：曾因 err.Error() 空指针 panic
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestErrorWithNilError(t *testing.T) {
	c, w := newCtx()
	Error(c, nil)
	if w.Code != 422 {
		t.Errorf("status = %d, want 422", w.Code)
	}
}
