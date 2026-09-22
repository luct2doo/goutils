package logger

import (
	"encoding/json"
	"fmt"
	"github.com/luct2doo/goutils/app"
	"github.com/luct2doo/goutils/config"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// logger 是包级函数（LogIf / Info / Error / Dump …）的落点。
//
// 默认给一个「兜底 logger」：在调用 NewLogger 之前，任何包级函数都会被安全地
// 写到 stderr，而不是对 nil 做方法调用导致 panic。
//
// 这一点对本库尤其重要：redis / cache / limiter / paginator / response / jwt
// 内部都会调用 logger.LogIf 之类的包级函数。若没有兜底，使用方只要漏调
// NewLogger（或初始化顺序不对），就会在一个普通的构造函数里崩溃。
var logger = newFallbackLogger()

// namedLogger 是「包级包装函数」专用的 zap 视图，比 logger.Logger 多跳一帧。
//
// 为什么需要两个视图：
//   - 直接调用实例方法（l.Info(...)）时，栈是 [zap, 调用方]，基础 logger 跳 0 帧即正确；
//   - 包级函数（logger.Info(...)）多了一层包装函数，栈是 [zap, 包装函数, 调用方]，需要跳 1 帧。
//
// 只用一个 AddCallerSkip 常量无法同时满足两者——这正是原实现
// AddCallerSkip(2) 会把调用点报成 runtime/proc.go 的原因。
var namedLogger = logger.Logger.WithOptions(zap.AddCallerSkip(1))

// newFallbackLogger 构造兜底 logger：Warn 及以上级别输出到 stderr。
//
// 选 stderr 而不是 no-op，是为了不静默吞掉错误——出现这类输出即意味着
// NewLogger 没有被正确调用。
func newFallbackLogger() *Logger {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     customTimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.AddSync(os.Stderr),
		zapcore.WarnLevel,
	)
	return &Logger{
		Logger: zap.New(core, zap.AddCaller()),
	}
}

type Logger struct {
	*zap.Logger
	app    *app.App
	logCfg *config.Log
}

func NewLogger(logCfg *config.Log, app *app.App) *Logger {
	writeSyncer := getLogWriter(
		logCfg.Filename,
		logCfg.MaxSize,
		logCfg.MaxBackups,
		logCfg.MaxAge,
		logCfg.Compress,
		logCfg.Type,
		app,
	)
	// 设置日志等级
	logLevel := new(zapcore.Level)
	if err := logLevel.UnmarshalText([]byte(logCfg.Level)); err != nil {
		fmt.Println("日志初始化错误，日志级别设置有误，请检查 log.level 配置项")
	}
	// 初始化 core
	core := zapcore.NewCore(getEncoder(app), writeSyncer, logLevel)
	// 初始化 Logger
	//
	// 基础 logger 不加 AddCallerSkip：这样 l.Info(...)（内嵌 zap 方法）报出的
	// caller 就是真实调用点。包级包装函数走 namedLogger（多跳一帧）。
	zapLogger := zap.New(core,
		zap.AddCaller(),                   // 调用文件和行号，内部使用 runtime.Caller
		zap.AddStacktrace(zap.ErrorLevel), // Error 时才会显示 stacktrace
	)
	// 将自定义的 logger 替换为全局的 logger
	zap.ReplaceGlobals(zapLogger)

	// 创建Logger实例
	loggerInstance := &Logger{
		Logger: zapLogger,
		app:    app,
		logCfg: logCfg,
	}

	// 重要：将实例赋值给全局变量，并同步刷新包级函数的视图
	logger = loggerInstance
	namedLogger = zapLogger.WithOptions(zap.AddCallerSkip(1))

	return loggerInstance
}

func getEncoder(app *app.App) zapcore.Encoder {
	// 日志格式规则
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller", // 代码调用，如 paginator/paginator.go:148
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,      // 每行日志的结尾添加 "\n"
		EncodeLevel:    zapcore.CapitalLevelEncoder,    // 日志级别名称大写，如 ERROR、INFO
		EncodeTime:     customTimeEncoder,              // 时间格式，我们自定义为 2006-01-02 15:04:05
		EncodeDuration: zapcore.SecondsDurationEncoder, // 执行时间，以秒为单位
		EncodeCaller:   zapcore.ShortCallerEncoder,     // Caller 短格式，如：types/converter.go:17，长格式为绝对路径
	}

	// 本地环境配置
	if app.IsLocal() {
		// 终端输出的关键词高亮
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		// 本地设置内置的 Console 解码器（支持 stacktrace 换行）
		return zapcore.NewConsoleEncoder(encoderConfig)
	}

	// 线上环境使用 JSON 编码器
	return zapcore.NewJSONEncoder(encoderConfig)
}

func customTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format(time.DateTime))
}

func getLogWriter(filename string, maxSize, maxBackup, maxAge int, compress bool, logType string, app *app.App) zapcore.WriteSyncer {
	// 如果配置了按照日期记录日志文件
	if logType == "daily" {
		logname := time.Now().Format(time.DateOnly + ".log")
		filename = strings.ReplaceAll(filename, "logs.log", logname)
	}

	// 滚动日志，详见 config/log.go
	lumberjackLogger := &lumberjack.Logger{
		Filename:   filename,
		MaxSize:    maxSize,
		MaxBackups: maxBackup,
		MaxAge:     maxAge,
		Compress:   compress,
	}

	if app.IsLocal() {
		// 本地开发终端打印和记录文件
		return zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout), zapcore.AddSync(lumberjackLogger))
	} else {
		// 生产环境只记录文件
		return zapcore.AddSync(lumberjackLogger)
	}
}

func Dump(value any, msg ...string) {
	valueString := jsonString(value)
	// 判断第二个参数是否传参 msg
	if len(msg) > 0 {
		namedLogger.Warn("Dump", zap.String(msg[0], valueString))
	} else {
		namedLogger.Warn("Dump", zap.String("data", valueString))
	}
}

// LogIf 当 err != nil 时记录 error 等级的日志
func LogIf(err error) {
	if err != nil {
		namedLogger.Error("Error Occurred:", zap.Error(err))
	}
}

// LogWarnIf 当 err != nil 时记录 warning 等级的日志
func LogWarnIf(err error) {
	if err != nil {
		namedLogger.Warn("Error Occurred:", zap.Error(err))
	}
}

// LogInfoIf 当 err != nil 时记录 info 等级的日志
func LogInfoIf(err error) {
	if err != nil {
		namedLogger.Info("Error Occurred:", zap.Error(err))
	}
}

// Debug 调试日志，详尽的程序日志
// 调用示例：
// logger.Debug("Database", zap.String("sql", sql))
func Debug(moduleName string, fields ...zap.Field) {
	namedLogger.Debug(moduleName, fields...)
}

// Info 告知类日志
func Info(moduleName string, fields ...zap.Field) {
	namedLogger.Info(moduleName, fields...)
}

// Warn 警告类
func Warn(moduleName string, fields ...zap.Field) {
	namedLogger.Warn(moduleName, fields...)
}

// Error 错误时记录，不应该中断程序，查看日志时重点关注
func Error(moduleName string, fields ...zap.Field) {
	namedLogger.Error(moduleName, fields...)
}

// Fatal 级别同 Error(), 写完 log 后调用 os.Exit(1) 退出程序
func Fatal(moduleName string, fields ...zap.Field) {
	namedLogger.Fatal(moduleName, fields...)
}

// DebugString 记录一条字符串类型的 debug 日志，调用示例：
// logger.DebugString("SMS", "短信内容", string(result.RawResponse))
func DebugString(moduleName, name, msg string) {
	namedLogger.Debug(moduleName, zap.String(name, msg))
}

func InfoString(moduleName, name, msg string) {
	namedLogger.Info(moduleName, zap.String(name, msg))
}

func WarnString(moduleName, name, msg string) {
	namedLogger.Warn(moduleName, zap.String(name, msg))
}

func ErrorString(moduleName, name, msg string) {
	namedLogger.Error(moduleName, zap.String(name, msg))
}

func FatalString(moduleName, name, msg string) {
	namedLogger.Fatal(moduleName, zap.String(name, msg))
}

// DebugJSON 记录对象类型的 debug 日志，使用 json.Marshal 进行编码。调用示例：
// logger.DebugJSON("Auth", "读取登录用户", auth.CurrentUser())
func DebugJSON(moduleName, name string, value any) {
	namedLogger.Debug(moduleName, zap.String(name, jsonString(value)))
}

func InfoJSON(moduleName, name string, value any) {
	namedLogger.Info(moduleName, zap.String(name, jsonString(value)))
}

func WarnJSON(moduleName, name string, value any) {
	namedLogger.Warn(moduleName, zap.String(name, jsonString(value)))
}

func ErrorJSON(moduleName, name string, value any) {
	namedLogger.Error(moduleName, zap.String(name, jsonString(value)))
}

func FatalJSON(moduleName, name string, value any) {
	namedLogger.Fatal(moduleName, zap.String(name, jsonString(value)))
}

func jsonString(value any) string {
	b, err := json.Marshal(value)
	if err != nil {
		// jsonString 是被包级包装函数调用的，比包装函数本身还多一层，
		// 因此要在 namedLogger 的基础上再跳一帧才能指到真实调用点。
		namedLogger.WithOptions(zap.AddCallerSkip(1)).
			Error("Logger", zap.String("JSON marshal error", err.Error()))
	}
	return string(b)
}
