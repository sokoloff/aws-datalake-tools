# aws-datalake-tools

Tools for AWS Data Lake:
- **datalake**: CLI utility for compacting Parquet files and loading DDB exports.
- **stream-processor**: Lambda handler for processing Kinesis Firehose events.

## Testing

Unit tests run offline without external dependencies:
```bash
make test
```

### Integration Tests

Integration tests run against real emulated AWS environments using [Moto](https://github.com/getmoto/moto) and the AWS Lambda Runtime Interface Emulator (RIE).

**Prerequisites:**
- Docker must be installed and running.
- The first run will pull Moto (~80MB) and RIE (~180MB) images, which takes a few seconds.

To run integration tests:
```bash
make test-integration
```

To run both unit and integration tests:
```bash
make test-all
```
