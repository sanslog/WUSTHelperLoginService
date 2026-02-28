package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var Log *zap.Logger

func Init(logDir string) error {
	// 确保日志目录存在
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	// 按天命名日志文件
	today := time.Now().Format("2006-01-02")
	logPath := filepath.Join(logDir, fmt.Sprintf("%s.log", today))

	// 文件切割与保留策略
	lumberJack := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    30, // 单个文件最大 30MB（可选）
		MaxBackups: 15, // 最多保留 15 个文件
		MaxAge:     15, // 最多保留 15 天
		Compress:   true,
	}
	bufferedWriter := &zapcore.BufferedWriteSyncer{
		WS:   zapcore.AddSync(lumberJack),
		Size: 256 * 1024,
	}

	encoder := zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
		TimeKey:      "time",
		LevelKey:     "level",
		MessageKey:   "msg",
		CallerKey:    "caller",
		EncodeTime:   zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05"),
		EncodeLevel:  zapcore.CapitalLevelEncoder,
		EncodeCaller: zapcore.ShortCallerEncoder,
	})

	core := zapcore.NewCore(
		encoder,
		zapcore.NewMultiWriteSyncer(
			bufferedWriter,             // 文件（异步）
			zapcore.AddSync(os.Stdout), // 控制台
		),
		zap.InfoLevel,
	)

	Log = zap.New(
		core,
		zap.AddCaller(),
		zap.AddCallerSkip(1),
	)

	return nil
}

// 程序退出前调用，确保缓冲日志刷盘
func Sync() {
	if Log != nil {
		_ = Log.Sync()
	}
}
