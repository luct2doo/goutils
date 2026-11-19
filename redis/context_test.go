package redis

import (
	"context"
	"testing"
)

func TestWithContextDoesNotMutateReceiver(t *testing.T) {
	base := &RedisClient{Context: context.Background()}
	derived, cancel := context.WithCancel(context.Background())
	defer cancel()

	copyClient := base.WithContext(derived)
	if copyClient == base {
		t.Fatal("WithContext 应返回新的浅拷贝")
	}
	if copyClient.Context != derived {
		t.Error("拷贝的 Context 未被替换")
	}
	if base.Context != context.Background() {
		t.Error("原实例的 Context 被意外修改")
	}

	if got := base.WithContext(nil); got != base {
		t.Error("nil context 应返回原实例")
	}
}
