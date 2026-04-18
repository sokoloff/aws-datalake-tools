package compact

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	gluetypes "github.com/aws/aws-sdk-go-v2/service/glue/types"
	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockGlueAPI struct {
	mock.Mock
}

func (m *mockGlueAPI) GetTable(ctx context.Context, params *glue.GetTableInput, optFns ...func(*glue.Options)) (*glue.GetTableOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*glue.GetTableOutput), args.Error(1)
}

func (m *mockGlueAPI) CreateTable(ctx context.Context, params *glue.CreateTableInput, optFns ...func(*glue.Options)) (*glue.CreateTableOutput, error) {
	return nil, nil
}
func (m *mockGlueAPI) UpdateTable(ctx context.Context, params *glue.UpdateTableInput, optFns ...func(*glue.Options)) (*glue.UpdateTableOutput, error) {
	return nil, nil
}
func (m *mockGlueAPI) DeleteTable(ctx context.Context, params *glue.DeleteTableInput, optFns ...func(*glue.Options)) (*glue.DeleteTableOutput, error) {
	return nil, nil
}

func TestBuildPlan_GlueSuccess(t *testing.T) {
	ctx := context.Background()
	s3api := newFakeS3()
	glueapi := new(mockGlueAPI)
	
	type Row struct{ ID int }
	buf := new(bytes.Buffer)
	pw := parquet.NewGenericWriter[Row](buf)
	pw.Write([]Row{{1}})
	pw.Close()
	s3api.put("b", "s/f1.parquet", buf.Bytes())
	
	glueapi.On("GetTable", ctx, mock.Anything).Return(&glue.GetTableOutput{
		Table: &gluetypes.Table{
			StorageDescriptor: &gluetypes.StorageDescriptor{
				Columns:  []gluetypes.Column{{Name: aws.String("id"), Type: aws.String("int")}},
				Location: aws.String("loc"),
			},
		},
	}, nil)

	p, err := BuildPlan(ctx, Deps{S3: s3api, Glue: glueapi}, Config{
		SourceURI: "s3://b/s",
		TargetURI: "s3://b/t",
		Database:  "db",
		Table:     "tbl",
	})
	assert.NoError(t, err)
	assert.Equal(t, ModeGlueCoerced, p.Mode)
}



func TestBuildPlan_Errors(t *testing.T) {
	ctx := context.Background()
	
	t.Run("invalid source uri", func(t *testing.T) {
		_, err := BuildPlan(ctx, Deps{}, Config{SourceURI: "invalid"})
		assert.Error(t, err)
	})

	t.Run("invalid target uri", func(t *testing.T) {
		_, err := BuildPlan(ctx, Deps{}, Config{SourceURI: "s3://b/s", TargetURI: "invalid"})
		assert.Error(t, err)
	})

	t.Run("list fail", func(t *testing.T) {
		s3api := &fakeS3{listErr: assert.AnError}
		_, err := BuildPlan(ctx, Deps{S3: s3api}, Config{SourceURI: "s3://b/s", TargetURI: "s3://b/t"})
		assert.Error(t, err)
	})

	t.Run("first file schema fail", func(t *testing.T) {
		s3api := newFakeS3()
		s3api.put("b", "s/f1.parquet", []byte("invalid"))
		_, err := BuildPlan(ctx, Deps{S3: s3api}, Config{SourceURI: "s3://b/s", TargetURI: "s3://b/t"})
		assert.Error(t, err)
	})

	t.Run("validation schema fail", func(t *testing.T) {
		s3api := newFakeS3()
		// First file is valid
		type row struct{ ID int }
		buf := new(bytes.Buffer)
		pw := parquet.NewGenericWriter[row](buf)
		pw.Write([]row{{1}})
		pw.Close()
		s3api.put("b", "s/f1.parquet", buf.Bytes())
		// Second file is invalid
		s3api.put("b", "s/f2.parquet", []byte("invalid"))
		
		_, err := BuildPlan(ctx, Deps{S3: s3api}, Config{SourceURI: "s3://b/s", TargetURI: "s3://b/t"})
		assert.Error(t, err)
	})
}


func TestCompactionPlan_Describe(t *testing.T) {
	p := &CompactionPlan{
		SourceBucket: "b1",
		TargetBucket: "b2",
		Mode:         ModePassThrough,
	}
	p.Describe(io.Discard)
	
	p.Mode = ModeGlueCoerced
	p.Incompatible = []string{"err1"}
	p.Describe(io.Discard)
}
