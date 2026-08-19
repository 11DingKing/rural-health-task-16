package logging

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

type Config struct {
	Level      string `yaml:"level" env:"LOG_LEVEL"`
	Format     string `yaml:"format" env:"LOG_FORMAT"`
	PrettyTime bool   `yaml:"pretty_time" env:"LOG_PRETTY_TIME"`
}

// Logger is the structured logging interface used across the codebase.
type Logger interface {
	Debug(ctx context.Context, msg string, keyvals ...any)
	Info(ctx context.Context, msg string, keyvals ...any)
	Warn(ctx context.Context, msg string, keyvals ...any)
	Error(ctx context.Context, msg string, keyvals ...any)
	With(keyvals ...any) Logger
}

type zerologLogger struct {
	logger zerolog.Logger
}

func New(cfg Config, out io.Writer) Logger {
	if out == nil {
		out = os.Stderr
	}
	level := parseLevel(cfg.Level)
	var writer io.Writer = out
	if cfg.Format == "console" || cfg.Format == "" {
		writer = zerolog.ConsoleWriter{Out: out, TimeFormat: time.RFC3339}
	}
	logger := zerolog.New(writer).Level(level).With().Timestamp().Logger()
	if cfg.PrettyTime {
		zerolog.TimeFieldFormat = time.RFC3339Nano
	}
	return &zerologLogger{logger: logger}
}

func NewNop() Logger {
	return &zerologLogger{logger: zerolog.Nop()}
}

func (zl *zerologLogger) Debug(ctx context.Context, msg string, keyvals ...any) {
	zl.log(ctx, zerolog.DebugLevel, msg, keyvals)
}

func (zl *zerologLogger) Info(ctx context.Context, msg string, keyvals ...any) {
	zl.log(ctx, zerolog.InfoLevel, msg, keyvals)
}

func (zl *zerologLogger) Warn(ctx context.Context, msg string, keyvals ...any) {
	zl.log(ctx, zerolog.WarnLevel, msg, keyvals)
}

func (zl *zerologLogger) Error(ctx context.Context, msg string, keyvals ...any) {
	zl.log(ctx, zerolog.ErrorLevel, msg, keyvals)
}

func (zl *zerologLogger) With(keyvals ...any) Logger {
	return &zerologLogger{logger: zl.logger.With().Fields(keyvals).Logger()}
}

func (zl *zerologLogger) log(ctx context.Context, level zerolog.Level, msg string, keyvals []any) {
	evt := zl.logger.WithLevel(level)
	if ctx != nil {
		evt = evt.Ctx(ctx)
	}
	if len(keyvals) > 0 {
		fields := make(map[string]any, len(keyvals)/2)
		for i := 0; i+1 < len(keyvals); i += 2 {
			if key, ok := keyvals[i].(string); ok {
				fields[key] = keyvals[i+1]
			}
		}
		evt = evt.Fields(fields)
	}
	evt.Msg(msg)
}

func parseLevel(s string) zerolog.Level {
	switch s {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "disabled":
		return zerolog.Disabled
	default:
		return zerolog.InfoLevel
	}
}
