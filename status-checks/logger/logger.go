package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger interface {
	Info(msg string, fields ...zapcore.Field)
	Fatal(msg string, fields ...zapcore.Field)
	Error(msg string, fields ...zapcore.Field)
}

type zapLogger struct {
	logger *zap.Logger
}

func (z *zapLogger) Info(msg string, fields ...zapcore.Field) {
	z.logger.Info(msg, fields...)
}

func (z *zapLogger) Fatal(msg string, fields ...zapcore.Field) {
	z.logger.Fatal(msg, fields...)
}

func (z *zapLogger) Error(msg string, fields ...zapcore.Field) {
	z.logger.Error(msg, fields...)
}

func String(key string, val string) zapcore.Field {
	return zap.String(key, val)
}

func Int(key string, val int) zapcore.Field {
	return zap.Int(key, val)
}

func NewZapLogger() Logger {
	l, err := zap.NewProduction()
	if err != nil {
		panic("failed to initialize zap logger: " + err.Error())
	}
	return &zapLogger{
		logger: l,
	}
}

var Log Logger = NewZapLogger()
