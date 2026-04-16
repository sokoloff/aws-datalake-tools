package main

import (
	"context"
	"fmt"

	"github.com/sokoloff/aws-datalake-tools/internal/logging"
)

func main() {
	logger := logging.New(logging.WithJSON())
	ctx := logging.WithContext(context.Background(), logger)
	logger.InfoContext(ctx, "stream-processor not yet implemented")
	fmt.Println("stream-processor lambda: not yet implemented")
}
