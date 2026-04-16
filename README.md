# SARIF-SQL

A Go CLI tool for managing GitHub Code Scanning Multi-Repository Variant Analysis (MRVA) workflows and transforming SARIF (Static Analysis Results Interchange Format) files into a SQLite database for efficient data processing and analytics.

## Installation

### Prerequisites

- Go 1.25.2 or later
- GitHub Personal Access Token or GitHub App credentials

### Build from Source

```bash
git clone https://github.com/ghas-projects/sarif-sql.git
cd sarif-sql
make build
```

The binary will be created at `dist/sarif-sql`.

## Usage

All commands require the global flags `--analysis-id` and `--controller-repo`. The `analysis` subcommands also require authentication via `--token` or `--app-id`/`--private-key`.

### 1. Start an Analysis

Initialize the local directory structure for a new MRVA analysis:

```bash
./dist/sarif-sql analysis start \
  --analysis-id 12345 \
  --controller-repo org/repo \
  --token $GITHUB_TOKEN
```

Or with GitHub App authentication:

```bash
./dist/sarif-sql analysis start \
  --analysis-id 12345 \
  --controller-repo org/repo \
  --app-id 123456 \
  --private-key "$GITHUB_APP_PRIVATE_KEY"
```

Creates a directory at `analyses/{analysis-id}-{owner}-{repo}/`.

### 2. Download Analysis Artifacts

Download SARIF artifacts from a completed analysis. The repository list is fetched automatically from the MRVA summary API:

```bash
./dist/sarif-sql analysis download \
  --analysis-id 12345 \
  --controller-repo org/repo \
  --directory ./analyses/12345-org-repo \
  --token $GITHUB_TOKEN
```

**Options:**
- `--directory`: Local directory to store downloaded SARIF files (required). Created in the first step.

Downloads are performed concurrently using an adaptive worker pool. Each artifact ZIP is extracted to a `.sarif` file automatically.

Generates a status report at `reports/{analysis-id}-{owner}-{repo}-status-report.md`.

### 3. Transform SARIF to SQLite

Convert the downloaded SARIF files into a SQLite database for analytics and reporting:

```bash
./dist/sarif-sql transform \
  --analysis-id 12345 \
  --controller-repo org/repo \
  --sarif-directory ./analyses/12345-org-repo \
  --output ./output
```

**Options:**
- `--sarif-directory`: Directory containing SARIF files (required)
- `--analysis-id`: Analysis ID for tracking (required)
- `--controller-repo`: Controller repository in owner/name format (required)
- `--output`: Output directory for SQLite database (default: `./output`)

**Output:**
- `mrva-analysis.db` - SQLite database containing:
  - `analysis` - Analysis run metadata
  - `repository` - Repository information and scan status
  - `rule` - CodeQL rule definitions
  - `alert` - Security findings/alerts with code snippets, locations, and code-flow data

### Analysis Summary (optional)

Fetch and generate a summary report for an MRVA analysis at any point:

```bash
./dist/sarif-sql analysis summary \
  --analysis-id 12345 \
  --controller-repo org/repo \
  --token $GITHUB_TOKEN
```

Generates a summary report at `reports/summary/{analysis-id}-{owner}-{repo}-summary-report.md`.

## Authentication

### GitHub Personal Access Token

```bash
export GITHUB_TOKEN="ghp_xxxxxxxxxxxx"
./dist/sarif-sql analysis download \
  --analysis-id 12345 \
  --controller-repo org/repo \
  --directory ./analyses/12345-org-repo \
  --token $GITHUB_TOKEN
```

**Required Scopes:**
- `repo` - Full control of private repositories
- `security_events` - Read and write security events

### GitHub App

```bash
export GITHUB_APP_PRIVATE_KEY="-----BEGIN RSA PRIVATE KEY-----
...
-----END RSA PRIVATE KEY-----"

./dist/sarif-sql analysis download \
  --analysis-id 12345 \
  --controller-repo org/repo \
  --directory ./analyses/12345-org-repo \
  --app-id 123456 \
  --private-key "$GITHUB_APP_PRIVATE_KEY"
```

**Required Permissions:**
- Code scanning alerts: Read and write
- Contents: Read-only

## Project Structure

```
sarif-sql/
├── cmd/                    # CLI commands
│   ├── sarif-protobuf.go  # Root command & global flags
│   ├── analysis/          # MRVA analysis commands (start, download, summary)
│   └── transform/         # SARIF → SQLite transformation command
├── internal/
│   ├── auth/              # Authentication (PAT & GitHub App)
│   ├── github/            # GitHub API client (MRVA status, summary, artifact download)
│   ├── models/            # Data models (SARIF, API responses, SQL types)
│   ├── parser/            # Repository file parsers (TOML, JSON)
│   ├── service/           # Business logic
│   │   ├── analysis_service.go  # MRVA lifecycle operations
│   │   ├── transform_service.go # SARIF → SQLite conversion
│   │   └── report.go            # Markdown report generation
│   └── store/             # Data persistence
│       └── sqlite.go      # SQLite schema, bulk writes, transactions
├── util/                  # Utilities
│   ├── logger.go          # Structured JSON logging
│   └── workers.go         # Adaptive worker pool sizing
├── main.go
├── Makefile
└── go.mod
```

## Logging

All operations log to `logs/sarif-sql-YYYYMMDD-HHMMSS.json` in structured JSON format:

```json
{"time":"2026-02-06T14:07:49Z","level":"INFO","msg":"transformation completed successfully","db":"./output/mrva-analysis.db","total_repositories":10,"total_rules":42,"total_alerts":3230}
```

## Error Handling

- **Cancellation**: Press Ctrl+C to gracefully stop long-running operations
- **Partial Failures**: Failed downloads are logged but don't stop the entire process
- **Validation**: Input validation with helpful error messages
- **Timeouts**: HTTP operations have appropriate timeouts

## Examples

### Complete MRVA Workflow

```bash
# 1. Start analysis - creates local directory structure
./dist/sarif-sql analysis start \
  --analysis-id 12345 \
  --controller-repo org/controller \
  --token $GITHUB_TOKEN

# 2. Wait for analysis to complete (check GitHub UI or use summary command)

# 3. Download SARIF artifacts
./dist/sarif-sql analysis download \
  --analysis-id 12345 \
  --controller-repo org/controller \
  --directory ./analyses/12345-org-controller \
  --token $GITHUB_TOKEN

# 4. Transform to SQLite
./dist/sarif-sql transform \
  --sarif-directory ./analyses/12345-org-controller \
  --analysis-id 12345 \
  --controller-repo org/controller \
  --output ./output

# 5. Query results with any SQLite client
sqlite3 ./output/mrva-analysis.db "SELECT COUNT(*) FROM alert"
```

## Pipeline

`sarif-sql` is one component of the end-to-end MRVA reporting pipeline. The full flow is:

1. **sarif-sql** (this repo) — Download SARIF artifacts and transform them into a normalized SQLite database.
2. [**mrva-prep**](https://github.com/ghas-projects/mrva-prep) — Add query-optimized indexes, extract dashboard metrics, and compress the database.
3. [**mrva-reports**](https://github.com/advanced-security/mrva-reports) — Render the database as an interactive single-page dashboard in the browser.

A GitHub Actions workflow chains all three steps into a single automated run. See the [MRVA Documentation](https://github.com/advanced-security/mrva-documentation) for the full architecture and user guide.

## License

Licensed under the [MIT license](LICENSE).
