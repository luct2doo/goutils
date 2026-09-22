package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	gapp "github.com/luct2doo/goutils/app"
	gconfig "github.com/luct2doo/goutils/config"
)

// 本测试锁定日志里 caller 字段的准确性。
//
// 背景：原实现给基础 logger 加了 AddCallerSkip(2)，导致
//   - 包级函数 logger.Info(...) 把调用点报成 runtime/proc.go
//   - 直接调用实例方法 l.Info(...) 报成 runtime/asm_arm64.s
//
// 单靠一个 skip 常量无法同时满足「包装函数多一层」和「直接调用不多一层」，
// 因此改为：基础 logger skip=0，包级函数走 namedLogger（skip=1）。
// 任何一方被改坏，本测试立刻失败。

// expectCaller 返回「下一行」的期望 caller 字符串。
//
// 调用方式：把 want := expectCaller() 写在目标日志调用语句的紧邻上一行，
// 因为 zap 用的是 ShortCallerEncoder，格式为 <倒数第二级目录>/<文件>:<行号>。
func expectCaller() string {
	_, file, line, _ := runtime.Caller(1)
	return fmt.Sprintf("%s/%s:%d", filepath.Base(filepath.Dir(file)), filepath.Base(file), line+1)
}

// newTestLogger 建一个只写文件、不写终端的 logger，并把日志文件路径返回。
func newTestLogger(t *testing.T) (*Logger, string) {
	t.Helper()
	dir := t.TempDir()
	logFile := filepath.Join(dir, "logs.log")
	a := gapp.NewApp(&gconfig.App{Name: "caller-probe", Env: "production", Timezone: "UTC"})
	l := NewLogger(&gconfig.Log{Filename: logFile, Level: "debug"}, a)
	return l, logFile
}

// callerOf 读出日志文件里第 n 条（从 0 计）记录的 caller 字段。
func callerOf(t *testing.T, logFile string, n int) string {
	t.Helper()
	b, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("读日志文件失败: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) <= n {
		t.Fatalf("日志只有 %d 行，取不到第 %d 条", len(lines), n)
	}
	var rec struct {
		Caller string `json:"caller"`
	}
	if err := json.Unmarshal([]byte(lines[n]), &rec); err != nil {
		t.Fatalf("解析日志行失败: %v\n原始行: %s", err, lines[n])
	}
	return rec.Caller
}

func TestCallerAccuracyPackageFunc(t *testing.T) {
	_, logFile := newTestLogger(t)

	want := expectCaller()
	Info("Probe", zap.String("via", "package func")) // ← caller 必须指向这一行

	if got := callerOf(t, logFile, 0); got != want {
		t.Errorf("包级 logger.Info 的 caller = %q, want %q", got, want)
	}
}

func TestCallerAccuracyMethod(t *testing.T) {
	l, logFile := newTestLogger(t)

	want := expectCaller()
	l.Info("via method") // ← caller 必须指向这一行

	if got := callerOf(t, logFile, 0); got != want {
		t.Errorf("实例方法 l.Info 的 caller = %q, want %q", got, want)
	}
}

func TestCallerAccuracyLogIf(t *testing.T) {
	_, logFile := newTestLogger(t)

	want := expectCaller()
	LogIf(fmt.Errorf("probe error")) // ← caller 必须指向这一行

	if got := callerOf(t, logFile, 0); got != want {
		t.Errorf("logger.LogIf 的 caller = %q, want %q", got, want)
	}
}

func TestCallerAccuracyGormAdapter(t *testing.T) {
	l, logFile := newTestLogger(t)
	gl := NewGormLogger(l)

	want := expectCaller()
	gl.Trace(context.Background(), time.Now(), func() (string, int64) { // ← caller 必须指向这一行
		return "SELECT 1", 1
	}, nil)

	if got := callerOf(t, logFile, 0); got != want {
		t.Errorf("GormLogger.Trace 的 caller = %q, want %q", got, want)
	}
}

// TestPackageFuncBeforeNewLogger 回归测试：NewLogger 之前调用包级函数不能 panic。
//
// 兜底 logger 生效时错误会写到 stderr（这里不关心内容），关键是「不 panic」。
func TestPackageFuncBeforeNewLogger(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("包级函数在 NewLogger 之前被调用时 panic 了: %v", r)
		}
	}()
	LogIf(fmt.Errorf("init 之前的错误"))
	Info("Probe", zap.String("before", "NewLogger"))
}
