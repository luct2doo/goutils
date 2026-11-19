package logger

import (
	"context"
	"testing"
	"time"

	gormlogger "gorm.io/gorm/logger"
)

func TestGormLoggerLogModeIsRespected(t *testing.T) {
	l := NewGormLogger(nil)
	silent, ok := l.LogMode(gormlogger.Silent).(GormLogger)
	if !ok {
		t.Fatal("LogMode 返回值类型应为 GormLogger")
	}
	if silent.LogLevel != gormlogger.Silent {
		t.Errorf("LogLevel = %v, want Silent", silent.LogLevel)
	}

	// Silent 模式下 Trace 不应回调 fc（回归：旧实现会打印所有 SQL）
	called := false
	silent.Trace(context.Background(), time.Now(), func() (string, int64) {
		called = true
		return "SELECT 1", 0
	}, nil)
	if called {
		t.Error("Silent 模式仍执行了 SQL 日志回调")
	}
}
