package compact

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/assert"
)

func TestNewRollingWriter_Error(t *testing.T) {
	ctx := context.Background()
	schema := parquet.NewSchema("test", parquet.Group{"f1": parquet.Int(32)})

	t.Run("mkdir fail", func(t *testing.T) {
		t.Setenv("TMPDIR", "/non/existent/path")
		_, err := NewRollingWriter(ctx, schema, 100, "b", "p", nil)
		assert.Error(t, err)
	})
}

func TestRollingWriter_OpenNewFileError(t *testing.T) {
	ctx := context.Background()
	schema := parquet.NewSchema("test", parquet.Group{"f1": parquet.Int(32)})

	w, err := NewRollingWriter(ctx, schema, 100, "b", "p", nil)
	assert.NoError(t, err)

	os.RemoveAll(w.tmpDir)
	err = w.openNewFile()
	assert.Error(t, err)
}

func TestRollingWriter_Errors(t *testing.T) {
	ctx := context.Background()
	node := parquet.Group{"f1": parquet.Int(32)}
	schema := parquet.NewSchema("test", node)

	t.Run("upload fail during close", func(t *testing.T) {
		failUpload := func(ctx context.Context, key string, body io.Reader, size int64) error {
			return assert.AnError
		}
		w, err := NewRollingWriter(ctx, schema, 128*1024*1024, "b", "p", failUpload)
		assert.NoError(t, err)

		type Row struct{ F1 int32 }
		buf := new(bytes.Buffer)
		pw := parquet.NewGenericWriter[Row](buf)
		pw.Write([]Row{{1}})
		pw.Close()

		pf, err := parquet.OpenFile(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		assert.NoError(t, err)

		conv, _ := BuildConversion(schema, pf.Schema())
		err = w.WriteConvertedRowGroup(pf.RowGroups()[0], conv)
		assert.NoError(t, err)

		err = w.Close()
		assert.Error(t, err)
	})

	t.Run("stat fail during roll", func(t *testing.T) {
		failUpload := func(ctx context.Context, key string, body io.Reader, size int64) error { return nil }
		w, err := NewRollingWriter(ctx, schema, 10, "b", "p", failUpload)
		assert.NoError(t, err)

		// Break the file handle to make Stat fail
		w.curFile.Close()

		err = w.roll()
		assert.Error(t, err)
	})

	t.Run("flush fail", func(t *testing.T) {
		w, err := NewRollingWriter(ctx, schema, 10, "b", "p", nil)
		assert.NoError(t, err)

		// Break the file to make Flush fail
		w.curFile.Close()

		// Write group expects valid writer
		type Row struct{ F1 int32 }
	})
	}
