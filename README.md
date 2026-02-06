# SARIF-Avro

A high-performance Go CLI tool for managing GitHub Code Scanning Multi-Repository Variant Analysis (MRVA) workflows and transforming SARIF (Static Analysis Results Interchange Format) files into Avro format for efficient data processing and analytics.

## Features

- 🚀 **High Performance**: Concurrent processing with optimized JSON parsing and HTTP connection pooling
- 🔄 **SARIF to Avro Transformation**: Convert Code Scanning results to Avro for data warehousing
- 📊 **MRVA Lifecycle Management**: Start, download, and summarize multi-repository analyses
- 🛡️ **Robust Error Handling**: Graceful cancellation support and comprehensive logging
- 🔐 **Flexible Authentication**: Support for both GitHub App and Personal Access Token authentication

## Installation

### Prerequisites

- Go 1.25.2 or later
- GitHub Personal Access Token or GitHub App credentials

### Build from Source

```bash
git clone https://github.com/ghas-projects/sarif-avro.git
cd sarif-avro
go build
```

The binary will be created as `sarif-avro` in the current directory.

## Usage

### Transform SARIF to Avro

Convert SARIF files to Avro format for analytics and reporting:

```bash
./sarif-avro transform \
  --sarif-directory ./analyses/12345-org-repo \
  --analysis-id 12345 \
  --controller-repo org/repo \
  --output ./avro-output
```

**Options:**
- `--sarif-directory`: Directory containing SARIF files (required)
- `--analysis-id`: Analysis ID for tracking (required)
- `--controller-repo`: Controller repository in owner/name format (required)
- `--output`: Output directory for Avro files (default: `./avro-output`)

**Output Files:**
- `run.avro` - Analysis run metadata
- `repository.avro` - Repository information
- `rule.avro` - Security rule definitions
- `alert.avro` - Security findings/alerts

### MRVA Analysis Management

#### Start an Analysis

Initialize directory structure for a new MRVA analysis:

```bash
./sarif-avro analysis start \
  --analysis-id 12345 \
  --controller-repo org/repo \
  --repos-file repos.toml \
  --token $GITHUB_TOKEN
```

Or with GitHub App authentication:

```bash
./sarif-avro analysis start \
  --analysis-id 12345 \
  --controller-repo org/repo \
  --repos "org/repo1,org/repo2,org/repo3" \
  --app-id 123456 \
  --private-key "$GITHUB_APP_PRIVATE_KEY"
```

#### Download Analysis Artifacts

Download SARIF artifacts from completed analyses:

```bash
./sarif-avro analysis download \
  --analysis-id 12345 \
  --controller-repo org/repo \
  --directory ./analyses/12345-org-repo \
  --repos-file repos.toml \
  --token $GITHUB_TOKEN
```

Generates a status report at `reports/{analysis-id}-{repo}-status-report.md`

#### Get Analysis Summary

Fetch and generate summary report for an MRVA analysis:

```bash
./sarif-avro analysis summary \
  --analysis-id 12345 \
  --controller-repo org/repo \
  --repos-file repos.toml \
  --token $GITHUB_TOKEN
```

Generates a summary report at `reports/summary/{analysis-id}-{repo}-summary-report.md`

### Repository List Formats

#### TOML Format (`repos.toml`)

```toml
[[repositories]]
full_name = "owner/repo1"

[[repositories]]
full_name = "owner/repo2"

[[repositories]]
full_name = "owner/repo3"
```

#### JSON Format (`repos.json`)

```json
[
  {"full_name": "owner/repo1"},
  {"full_name": "owner/repo2"},
  {"full_name": "owner/repo3"}
]
```

#### Command Line

```bash
--repos "owner/repo1,owner/repo2,owner/repo3"
```

## Authentication

### GitHub Personal Access Token

```bash
export GITHUB_TOKEN="ghp_xxxxxxxxxxxx"
./sarif-avro analysis download --token $GITHUB_TOKEN ...
```

**Required Scopes:**
- `repo` - Full control of private repositories
- `security_events` - Read and write security events

### GitHub App

```bash
export GITHUB_APP_PRIVATE_KEY="-----BEGIN RSA PRIVATE KEY-----
...
-----END RSA PRIVATE KEY-----"

./sarif-avro analysis download \
  --app-id 123456 \
  --private-key "$GITHUB_APP_PRIVATE_KEY" \
  ...
```

**Required Permissions:**
- Code scanning alerts: Read and write
- Contents: Read-only

## Architecture

### Key Design Features

1. **Structured SARIF Parsing**: Uses strongly-typed Go structs instead of generic maps for 40-60% faster JSON unmarshaling

2. **HTTP Client Reuse**: Single HTTP client per service with connection pooling for improved performance

3. **Concurrent Processing**: Worker pool pattern with optimal goroutine count based on CPU cores and task count

4. **Dynamic Memory Pre-allocation**: String builders pre-allocate based on expected output size to reduce GC pressure

5. **Graceful Cancellation**: Context-based cancellation propagates through all operations for clean shutdown

### Project Structure

```
sarif-avro/
├── cmd/                    # CLI commands
│   ├── analysis/          # MRVA analysis commands
│   └── transform/         # SARIF transformation commands
├── internal/
│   ├── auth/             # Authentication (PAT & GitHub App)
│   ├── github/           # GitHub API client
│   ├── models/           # Data models (SARIF, Avro, API)
│   ├── parser/           # Repository file parsers
│   └── service/          # Business logic
│       ├── analysis_service.go  # MRVA operations
│       ├── transform_service.go # SARIF→Avro conversion
│       └── report.go            # Markdown report generation
├── schema/               # Avro schemas (.avsc)
├── util/                 # Utilities (logging, workers)
└── main.go
```

## Performance Optimizations

This tool is optimized for processing large-scale MRVA analyses:

- **Concurrent Processing**: Processes multiple SARIF files simultaneously
- **Connection Pooling**: Reuses HTTP connections across API calls
- **Efficient Memory Use**: Pre-allocated buffers and structured parsing
- **Batch Operations**: Reduces lock contention with batch writes

**Benchmarks** (10 SARIF files, ~16MB total):
- Parsing: ~60-70ms
- Transformation: ~100ms total
- Report generation: ~20-30ms

## Logging

All operations log to `logs/sarif-avro-YYYYMMDD-HHMMSS.json` in structured JSON format:

```json
{"time":"2026-02-06T14:07:49Z","level":"INFO","msg":"transformation completed","total_alerts":3230,"total_repositories":10}
```

## Error Handling

- **Cancellation**: Press Ctrl+C to gracefully stop long-running operations
- **Partial Failures**: Failed downloads are logged but don't stop the entire process
- **Validation**: Input validation with helpful error messages
- **Timeouts**: HTTP operations have appropriate timeouts

## Examples

### Complete MRVA Workflow

```bash
# 1. Start analysis
./sarif-avro analysis start \
  --analysis-id 12345 \
  --controller-repo org/controller \
  --repos-file repos.toml \
  --token $GITHUB_TOKEN

# 2. Wait for analysis to complete (check GitHub UI or use summary command)

# 3. Download SARIF artifacts
./sarif-avro analysis download \
  --analysis-id 12345 \
  --controller-repo org/controller \
  --directory ./analyses/12345-org-controller \
  --repos-file repos.toml \
  --token $GITHUB_TOKEN

# 4. Transform to Avro
./sarif-avro transform \
  --sarif-directory ./analyses/12345-org-controller \
  --analysis-id 12345 \
  --controller-repo org/controller \
  --output ./avro-output

# 5. Generate summary report
./sarif-avro analysis summary \
  --analysis-id 12345 \
  --controller-repo org/controller \
  --repos-file repos.toml \
  --token $GITHUB_TOKEN
```

## Contributing

### Development Setup

```bash
go get -u
go build -v
```

### Running Tests

```bash
go test ./...
```

### Code Style

- Follow standard Go conventions
- Use `gofmt` for formatting
- Add structured logging for important operations
- Include context parameters for cancellable operations

## License

See LICENSE file for details.

## Support

For issues and questions:
- Open an issue on GitHub
- Check existing issues for solutions
- Review logs in `logs/` directory for debugging

## Acknowledgments

- Built with [Cobra](https://github.com/spf13/cobra) for CLI framework
- Uses [Hamba Avro](https://github.com/hamba/avro) for Avro encoding
- Designed for GitHub Advanced Security workflows
