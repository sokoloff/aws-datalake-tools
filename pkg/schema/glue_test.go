package schema

import (
	"context"
	"fmt"
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

func (m *mockGlueAPI) CreatePartition(ctx context.Context, params *glue.CreatePartitionInput, optFns ...func(*glue.Options)) (*glue.CreatePartitionOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*glue.CreatePartitionOutput), args.Error(1)
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

func TestCreateTable_Replace(t *testing.T) {
	api := new(mockGlueAPI)
	ctx := context.Background()

	in := CreateTableInput{
		Database: "db",
		Table:    "tbl",
		Replace:  true,
	}

	api.On("DeleteTable", ctx, &glue.DeleteTableInput{
		DatabaseName: aws.String("db"),
		Name:         aws.String("tbl"),
	}).Return(&glue.DeleteTableOutput{}, nil)

	api.On("CreateTable", ctx, mock.Anything).Return(&glue.CreateTableOutput{}, nil)

	err := CreateTable(ctx, api, in)
	assert.NoError(t, err)
}

func TestFetchTableSchema_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("not found", func(t *testing.T) {
		api := new(mockGlueAPI)
		api.On("GetTable", ctx, mock.Anything).Return(&glue.GetTableOutput{Table: nil}, nil)
		_, err := FetchTableSchema(ctx, api, "db", "tbl")
		assert.Error(t, err)
	})

	t.Run("api error", func(t *testing.T) {
		api := new(mockGlueAPI)
		api.On("GetTable", ctx, mock.Anything).Return((*glue.GetTableOutput)(nil), fmt.Errorf("api error"))
		_, err := FetchTableSchema(ctx, api, "db", "tbl")
		assert.Error(t, err)
	})

	t.Run("no storage descriptor", func(t *testing.T) {
		api := new(mockGlueAPI)
		api.On("GetTable", ctx, mock.Anything).Return(&glue.GetTableOutput{
			Table: &gluetypes.Table{StorageDescriptor: nil},
		}, nil)
		_, err := FetchTableSchema(ctx, api, "db", "tbl")
		assert.Error(t, err)
	})

	t.Run("malformed type", func(t *testing.T) {
		api := new(mockGlueAPI)
		api.On("GetTable", ctx, mock.Anything).Return(&glue.GetTableOutput{
			Table: &gluetypes.Table{
				StorageDescriptor: &gluetypes.StorageDescriptor{
					Columns: []gluetypes.Column{{Name: aws.String("c"), Type: aws.String("invalid<")}},
				},
			},
		}, nil)
		_, err := FetchTableSchema(ctx, api, "db", "tbl")
		assert.Error(t, err)
	})
}

func TestUpdateTableSchema(t *testing.T) {
	api := new(mockGlueAPI)
	ctx := context.Background()

	api.On("GetTable", ctx, mock.Anything).Return(&glue.GetTableOutput{
		Table: &gluetypes.Table{
			Name: aws.String("tbl"),
			StorageDescriptor: &gluetypes.StorageDescriptor{
				Columns: []gluetypes.Column{},
			},
		},
	}, nil)

	api.On("UpdateTable", ctx, mock.Anything).Return(&glue.UpdateTableOutput{}, nil)

	err := UpdateTableSchema(ctx, api, "db", "tbl", []Column{{Name: "id", Type: PrimitiveType{Kind: Int}}})
	assert.NoError(t, err)

	t.Run("fetch fail", func(t *testing.T) {
		api := new(mockGlueAPI)
		api.On("GetTable", ctx, mock.Anything).Return((*glue.GetTableOutput)(nil), fmt.Errorf("err"))
		err := UpdateTableSchema(ctx, api, "db", "tbl", nil)
		assert.Error(t, err)
	})

	t.Run("update fail", func(t *testing.T) {
		api := new(mockGlueAPI)
		api.On("GetTable", ctx, mock.Anything).Return(&glue.GetTableOutput{
			Table: &gluetypes.Table{Name: aws.String("t"), StorageDescriptor: &gluetypes.StorageDescriptor{}},
		}, nil)
		api.On("UpdateTable", ctx, mock.Anything).Return((*glue.UpdateTableOutput)(nil), fmt.Errorf("err"))
		err := UpdateTableSchema(ctx, api, "db", "tbl", nil)
		assert.Error(t, err)
	})
}
