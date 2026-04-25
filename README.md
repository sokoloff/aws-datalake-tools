# Datalake Tools

High-performance utilities for AWS Data Lake ingestion and optimization, written in Go.

## Features

**CLI (`datalake`)**
* **Parquet Compaction**: Safely merges small Parquet files in S3 into optimized, larger files and updates AWS Glue tables.
* **DynamoDB Loader**: Parses DynamoDB JSON export dumps, infers schemas, and converts them to partitioned Parquet files in S3.

**Stream Processor (`stream-processor`)**
* **Firehose Transformation**: An AWS Lambda handler that transforms DynamoDB stream records (from Kinesis Firehose) into flat JSON for downstream ingestion.

## Usage

### `datalake compact`

Merges small Parquet files into larger, optimized ones. If a Glue database and table are provided, it ensures the compacted files adhere to the target table's schema.

```bash
datalake compact \
  --source s3://my-bucket/raw/ \
  --target s3://my-bucket/compacted/ \
  --database my_database \
  --table my_table \
  --target-size-mb 128 \
  --delete-source
```

### `datalake load`

Parses a DynamoDB S3 export (JSON format), infers the schema, and converts the data into partitioned Parquet files in S3.

```bash
datalake load \
  --input s3://my-bucket/dynamodb-export/AWSDynamoDB/0123456789-abcdef/ \
  --output s3://my-bucket/parquet-data/ \
  --database my_database \
  --table my_table \
  --partition auto
```

### `stream-processor`

This is an AWS Lambda handler designed to receive events from Amazon Kinesis Data Firehose (originating from a DynamoDB stream). It unpacks the `NewImage` from the DynamoDB stream record and flattens it into a standard JSON document.

Deploy the compiled `bootstrap` binary to AWS Lambda using the `provided.al2023` custom runtime. Configure a Kinesis Data Firehose delivery stream to use this Lambda function for data transformation.

## Testing

* `make test`: Runs fast, offline unit tests.
* `make test-integration`: Runs end-to-end tests against real emulated AWS environments using Moto and AWS Lambda RIE (requires Docker).
