package main

import (
	"log/slog"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/sokoloff/aws-datalake-tools/internal/logging"
	"github.com/sokoloff/aws-datalake-tools/internal/streamhandler"
)

func main() {
	levelStr := strings.ToUpper(os.Getenv("LOG_LEVEL"))
	var level slog.Level
	switch levelStr {
	case "DEBUG":
		level = slog.LevelDebug
	case "WARN", "WARNING":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	logger := logging.New(logging.WithJSON(), logging.WithLevel(level))

	h := streamhandler.New(logger)
	lambda.Start(h.Handle)
}
