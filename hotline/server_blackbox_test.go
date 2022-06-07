package hotline

import (
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"testing"
)

func NewTestLogger() *zap.SugaredLogger {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "timestamp"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderCfg),
		zapcore.Lock(os.Stdout),
		zap.DebugLevel,
	)

	cores := []zapcore.Core{core}
	l := zap.New(zapcore.NewTee(cores...))
	defer func() { _ = l.Sync() }()
	return l.Sugar()
}

// tranAssertEqual compares equality of transactions slices after stripping out the random ID
func tranAssertEqual(t *testing.T, tran1, tran2 []Transaction) bool {
	var newT1 []Transaction
	var newT2 []Transaction
	for _, trans := range tran1 {
		trans.ID = []byte{0, 0, 0, 0}
		newT1 = append(newT1, trans)
	}

	for _, trans := range tran2 {
		trans.ID = []byte{0, 0, 0, 0}
		newT2 = append(newT2, trans)

	}

	return assert.Equal(t, newT1, newT2)
}
