package logging

import (
	"context"
	"log/slog"
	"os"
)

type ctxKey struct{}

type config struct {
	json      bool
	level     slog.Level
	attrs     []slog.Attr
}

type Option func(*config)

func WithJSON() Option {
	return func(c *config) {
		c.json = true
	}
}

func WithLevel(level slog.Level) Option {
	return func(c *config) {
		c.level = level
	}
}

func WithRequestID(id string) Option {
	return func(c *config) {
		c.attrs = append(c.attrs, slog.String("request_id", id))
	}
}

// New creates a configured slog.Logger. Defaults to text output at Info level.
func New(opts ...Option) *slog.Logger {
	cfg := config{
		level: slog.LevelInfo,
	}
	for _, o := range opts {
		o(&cfg)
	}

	handlerOpts := &slog.HandlerOptions{
		Level: cfg.level,
	}

	var handler slog.Handler
	if cfg.json {
		handler = slog.NewJSONHandler(os.Stderr, handlerOpts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, handlerOpts)
	}

	if len(cfg.attrs) > 0 {
		handler = handler.WithAttrs(cfg.attrs)
	}

	return slog.New(handler)
}

// WithContext stores a logger in the context.
func WithContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, logger)
}

// FromContext retrieves the logger from context, or returns the default logger.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}
