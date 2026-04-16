package schema

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	gluetypes "github.com/aws/aws-sdk-go-v2/service/glue/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockGlueAPI struct {
	mock.Mock
}

func (m *mockGlueAPI) GetTable(ctx context.Context, params *glue.GetTableInput, optFns ...func(*glue.Options)) (*glue.GetTableOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*glue.GetTableOutput), args.Error(1)
}

func (m *mockGlueAPI) CreateTable(ctx context.Context, params *glue.CreateTableInput, optFns ...func(*glue.Options)) (*glue.CreateTableOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*glue.CreateTableOutput), args.Error(1)
}

func (m *mockGlueAPI) UpdateTable(ctx context.Context, params *glue.UpdateTableInput, optFns ...func(*glue.Options)) (*glue.UpdateTableOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*glue.UpdateTableOutput), args.Error(1)
}

func (m *mockGlueAPI) DeleteTable(ctx context.Context, params *glue.DeleteTableInput, optFns ...func(*glue.Options)) (*glue.DeleteTableOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*glue.DeleteTableOutput), args.Error(1)
}

func TestFetchTableSchema(t *testing.T) {
	api := new(mockGlueAPI)
	ctx := context.Background()

	api.On("GetTable", ctx, &glue.GetTableInput{
		DatabaseName: aws.String("db"),
		Name:         aws.String("tbl"),
	}).Return(&glue.GetTableOutput{
		Table: &gluetypes.Table{
			StorageDescriptor: &gluetypes.StorageDescriptor{
				Columns: []gluetypes.Column{
					{Name: aws.String("id"), Type: aws.String("int")},
					{Name: aws.String("name"), Type: aws.String("string")},
				},
				Location: aws.String("s3://bucket/path"),
			},
			PartitionKeys: []gluetypes.Column{
				{Name: aws.String("dt"), Type: aws.String("string")},
			},
		},
	}, nil)

	schema, err := FetchTableSchema(ctx, api, "db", "tbl")
	assert.NoError(t, err)
	assert.Equal(t, "db", schema.Database)
	assert.Equal(t, "tbl", schema.Table)
	assert.Len(t, schema.Columns, 2)
	assert.Equal(t, "id", schema.Columns[0].Name)
	assert.Equal(t, PrimitiveType{Kind: Int}, schema.Columns[0].Type)
	assert.Equal(t, "dt", schema.PartitionKeys[0].Name)
	assert.Equal(t, "s3://bucket/path", schema.Location)

	api.AssertExpectations(t)
}

func TestCreateTable(t *testing.T) {
	api := new(mockGlueAPI)
	ctx := context.Background()

	in := CreateTableInput{
		Database: "db",
		Table:    "tbl",
		Columns: []Column{
			{Name: "id", Type: PrimitiveType{Kind: Int}},
		},
		Location: "s3://loc",
	}

	api.On("CreateTable", ctx, mock.MatchedBy(func(input *glue.CreateTableInput) bool {
		return *input.DatabaseName == "db" && *input.TableInput.Name == "tbl"
	})).Return(&glue.CreateTableOutput{}, nil)

	err := CreateTable(ctx, api, in)
	assert.NoError(t, err)

	api.AssertExpectations(t)
}
