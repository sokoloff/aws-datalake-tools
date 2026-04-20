package compact

import (
	"context"
	"io"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/parquet-go/parquet-go"
	"github.com/sokoloff/aws-datalake-tools/pkg/s3util"
)

// downloadResult carries a single file downloaded into a local temp file,
// ready to be read as a parquet.File. The consumer must call Cleanup when done.
type downloadResult struct {
	Index   int
	Key     string
	File    *parquet.File
	RawFile *os.File
	TmpPath string
	Err     error
}

// Cleanup closes the raw file handle and removes the temp file. Safe to call
// even when Err is non-nil (fields are zero-valued in that case).
func (r *downloadResult) Cleanup() {
	if r.RawFile != nil {
		r.RawFile.Close()
	}
	if r.TmpPath != "" {
		os.Remove(r.TmpPath)
	}
}

// streamDownloads downloads files in parallel (capped worker pool) and emits
// results on the returned channel in source order. The channel is closed once
// every file has been delivered. Consumers must drain fully and call Cleanup
// on each result. Per-file progress is logged at Debug level when log != nil.
func streamDownloads(ctx context.Context, api S3API, bucket string, files []s3util.Object, log *slog.Logger) <-chan downloadResult {
	numWorkers := runtime.NumCPU()
	if numWorkers > 8 {
		numWorkers = 8
	}
	if numWorkers > len(files) {
		numWorkers = len(files)
	}

	if log != nil {
		log.Debug("starting parallel download",
			"bucket", bucket,
			"files", len(files),
			"workers", numWorkers,
		)
	}

	type work struct {
		index int
		key   string
	}
	workCh := make(chan work, len(files))
	rawCh := make(chan downloadResult, len(files))

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for w := range workCh {
				res := downloadResult{Index: w.index, Key: w.key}

				if log != nil {
					log.Debug("downloading file",
						"bucket", bucket,
						"key", w.key,
						"index", w.index+1,
						"total", len(files),
					)
				}

				started := time.Now()
				f, raw, tmp, err := downloadAndOpenFile(ctx, api, bucket, w.key)
				elapsed := time.Since(started)

				if err != nil {
					res.Err = err
					if log != nil {
						log.Debug("download failed",
							"bucket", bucket,
							"key", w.key,
							"duration", elapsed,
							"error", err,
						)
					}
				} else {
					res.File = f
					res.RawFile = raw
					res.TmpPath = tmp
					if log != nil {
						var bytes int64
						if info, statErr := raw.Stat(); statErr == nil {
							bytes = info.Size()
						}
						log.Debug("download complete",
							"bucket", bucket,
							"key", w.key,
							"bytes", bytes,
							"duration", elapsed,
						)
					}
				}
				rawCh <- res
			}
		}()
	}

	for i, f := range files {
		workCh <- work{index: i, key: f.Key}
	}
	close(workCh)

	go func() {
		wg.Wait()
		close(rawCh)
	}()

	ordered := make(chan downloadResult, len(files))
	go func() {
		defer close(ordered)
		pending := make(map[int]downloadResult)
		next := 0
		for res := range rawCh {
			pending[res.Index] = res
			for {
				item, ok := pending[next]
				if !ok {
					break
				}
				delete(pending, next)
				next++
				ordered <- item
			}
		}
	}()

	return ordered
}

func downloadAndOpenFile(ctx context.Context, api S3API, bucket, key string) (*parquet.File, *os.File, string, error) {
	resp, err := api.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, nil, "", err
	}
	defer resp.Body.Close()

	tmp, err := os.CreateTemp("", "datalake-src-*.parquet")
	if err != nil {
		return nil, nil, "", err
	}

	size, err := io.Copy(tmp, resp.Body)
	if err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return nil, nil, "", err
	}

	if _, err := tmp.Seek(0, 0); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return nil, nil, "", err
	}

	pf, err := parquet.OpenFile(tmp, size)
	if err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return nil, nil, "", err
	}

	return pf, tmp, tmp.Name(), nil
}
