package awsclient

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Config struct {
	Region  string
	Profile string
}

type Clients struct {
	S3        *s3.Client
	Glue      *glue.Client
	DynamoDB  *dynamodb.Client
	AWSConfig aws.Config
}

// New creates AWS service clients from the given configuration.
func New(ctx context.Context, cfg Config) (*Clients, error) {
	var opts []func(*config.LoadOptions) error

	if cfg.Region != "" {
		opts = append(opts, config.WithRegion(cfg.Region))
	}
	if cfg.Profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(cfg.Profile))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}

	return &Clients{
		S3:        s3.NewFromConfig(awsCfg),
		Glue:      glue.NewFromConfig(awsCfg),
		DynamoDB:  dynamodb.NewFromConfig(awsCfg),
		AWSConfig: awsCfg,
	}, nil
}
