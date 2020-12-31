package hotline

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.SugaredLogger

func newStdoutCore() zapcore.Core {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "timestamp"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	encoder := zapcore.NewConsoleEncoder(encoderCfg)
	writer := zapcore.Lock(os.Stdout)

	return zapcore.NewCore(
		encoder,
		writer,
		zap.InfoLevel,
	)
}
