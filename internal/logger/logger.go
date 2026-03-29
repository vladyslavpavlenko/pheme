package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// LevelDebug and LevelProd are the level strings accepted by New.
const (
	LevelDebug = "DEBUG"
	LevelProd  = "PROD"
)

// Logger wraps zap for structured, leveled output with Field helpers.
type Logger struct {
	z *zap.Logger
}

// NewNop returns a logger that discards all output.
func NewNop() *Logger {
	return &Logger{z: zap.NewNop()}
}

// New returns a stdout console logger; LevelProd uses production config, otherwise development.
func New(level string) *Logger {
	var cfg zap.Config
	switch level {
	case LevelProd:
		cfg = zap.NewProductionConfig()
	default:
		cfg = zap.NewDevelopmentConfig()
	}

	cfg.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05")
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder

	sink := zapcore.AddSync(os.Stdout)
	encoder := zapcore.NewConsoleEncoder(cfg.EncoderConfig)
	core := zapcore.NewCore(encoder, sink, cfg.Level)

	return &Logger{z: zap.New(core)}
}

// Field is a marshaling operation used to add a key-value pair to a logger's context.
type Field struct {
	zField zap.Field
}

// Param takes a key and an arbitrary value and chooses the best way to represent them as a field.
func Param(key string, val any) Field {
	return Field{zField: zap.Any(key, val)}
}

// Error is shorthand for the common idiom NamedError("error", err).
func Error(err error) Field {
	return Field{zField: zap.Error(err)}
}

// With returns a new Logger with extra fields.
func (l *Logger) With(fields ...Field) *Logger {
	return &Logger{z: l.z.With(l.toZapFields(fields)...)}
}

// Debug writes msg with optional structured fields at debug severity.
func (l *Logger) Debug(msg string, fields ...Field) {
	l.z.Debug(msg, l.toZapFields(fields)...)
}

// Info writes msg with optional structured fields at info severity.
func (l *Logger) Info(msg string, fields ...Field) {
	l.z.Info(msg, l.toZapFields(fields)...)
}

// Warn writes msg with optional structured fields at warn severity.
func (l *Logger) Warn(msg string, fields ...Field) {
	l.z.Warn(msg, l.toZapFields(fields)...)
}

// Error writes msg with optional structured fields at error severity.
func (l *Logger) Error(msg string, fields ...Field) {
	l.z.Error(msg, l.toZapFields(fields)...)
}

// Fatal writes msg with optional structured fields at fatal severity, then stops the process.
func (l *Logger) Fatal(msg string, fields ...Field) {
	l.z.Fatal(msg, l.toZapFields(fields)...)
}

func (l *Logger) toZapFields(fields []Field) []zap.Field {
	if len(fields) == 0 {
		return nil
	}
	zFields := make([]zap.Field, len(fields))
	for i, f := range fields {
		zFields[i] = f.zField
	}
	return zFields
}
