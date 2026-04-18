package compact

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/assert"
)

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
		
		// Write some rows
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

	t.Run("upload fail during roll", func(t *testing.T) {
		failUpload := func(ctx context.Context, key string, body io.Reader, size int64) error {
			return assert.AnError
		}
		// Tiny rollBytes to trigger roll early
		w, err := NewRollingWriter(ctx, schema, 10, "b", "p", failUpload)
		assert.NoError(t, err)
		
		type Row struct{ F1 int32 }
		var buf bytes.Buffer
		pw := parquet.NewGenericWriter[Row](&buf)
		// Write enough rows to trigger w.curRows >= 10000 logic or just enough to hit rollBytes
		// Actually WriteConvertedRowGroup checks Size() only after Flush() every 10000 rows.
		// I'll write 10000 rows.
		rows := make([]Row, 10001)
		pw.Write(rows)
		pw.Close()
		
		pf, err := parquet.OpenFile(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		assert.NoError(t, err)
		
		conv, _ := BuildConversion(schema, pf.Schema())
		err = w.WriteConvertedRowGroup(pf.RowGroups()[0], conv)
		assert.Error(t, err) // Should fail during roll()
		w.Close()
	})
}

