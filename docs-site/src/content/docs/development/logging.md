---
title: Local Development Logging
description: Setting up structured logging for Hub and Broker development.
---

When developing the Hub or Runtime Broker locally, you often need to test the logging pipeline and verify log output before deploying to GCP. This guide covers how to configure structured logging for local development, including GCP Cloud Logging format testing.

## Quick Start

### Enable GCP-Format Logging Locally

Set the `SCION_LOG_GCP` environment variable to output logs in GCP Cloud Logging format:

```bash
# Run hub with GCP-formatted logs
SCION_LOG_GCP=true scion server start --enable-hub

# Run broker with GCP-formatted logs
SCION_LOG_GCP=true scion server start --enable-runtime-broker

# Run both with debug logging enabled
SCION_LOG_GCP=true SCION_LOG_LEVEL=debug scion server start --enable-hub --enable-runtime-broker
```

### Standard JSON Output

Without `SCION_LOG_GCP`, logs use standard JSON format:

```json
{
  "time": "2025-02-09T12:34:56Z",
  "level": "INFO",
  "msg": "Server started",
  "component": "scion-hub",
  "port": 9810
}
```

### GCP Cloud Logging Output

With `SCION_LOG_GCP=true`, logs use GCP's expected format:

```json
{
  "severity": "INFO",
  "message": "Server started",
  "timestamp": "2025-02-09T12:34:56Z",
  "logging.googleapis.com/labels": {
    "component": "scion-hub",
    "hostname": "dev-machine"
  },
  "logging.googleapis.com/sourceLocation": {
    "file": "/path/to/server.go",
    "line": "172",
    "function": "cmd.runServerStart"
  },
  "port": 9810
}
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SCION_LOG_GCP` | Enable GCP Cloud Logging format | `false` |
| `SCION_LOG_LEVEL` | Log level (`debug`, `info`, `warn`, `error`) | `info` |
| `K_SERVICE` | Auto-enables GCP logging (set by Cloud Run) | - |

## Sending Logs to GCP from Local Machine

For development, you can pipe logs directly to GCP Cloud Logging using the `gcloud` CLI.

### Prerequisites

1. Install [Google Cloud SDK](https://cloud.google.com/sdk/docs/install)
2. Authenticate: `gcloud auth login`
3. Set your project: `gcloud config set project YOUR_PROJECT_ID`

### Pipe Logs to Cloud Logging

```bash
# Create a custom log stream for development
SCION_LOG_GCP=true scion server start --enable-hub 2>&1 | \
  while read line; do
    echo "$line" | gcloud logging write scion-dev-hub --payload-type=json
  done
```

### Using a Named Pipe (Background Server)

For long-running development sessions:

```bash
# Create a named pipe
mkfifo /tmp/scion-logs

# Start log forwarder in background
cat /tmp/scion-logs | while read line; do
  echo "$line" | gcloud logging write scion-dev-hub --payload-type=json
done &

# Run server with logs to pipe
SCION_LOG_GCP=true scion server start --enable-hub > /tmp/scion-logs 2>&1
```

### View Logs in GCP Console

Navigate to **Logging > Logs Explorer** in the GCP Console and filter by:

```
logName="projects/YOUR_PROJECT/logs/scion-dev-hub"
```

## OpenTelemetry Export

For more advanced setups, you can export logs via OpenTelemetry to GCP:

```bash
# Enable OTel log bridging
export SCION_OTEL_LOG_ENABLED=true
export SCION_OTEL_ENDPOINT="monitoring.googleapis.com:443"
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/service-account.json"

scion server start --enable-hub
```

See the [Observability guide](/guides/observability) for full OTel configuration.

## Log Levels

Use `--debug` flag or `SCION_LOG_LEVEL=debug` for verbose output during development:

```bash
# Via flag
scion server start --enable-hub --debug

# Via environment
SCION_LOG_LEVEL=debug scion server start --enable-hub
```

Debug logging includes:
- Request/response details
- Internal state transitions
- Detailed error context

## Component Names

The log `component` field reflects the server mode:

| Mode | Component |
|------|-----------|
| Hub only | `scion-hub` |
| Broker only | `scion-broker` |
| Both | `scion-server` |

## Testing Log Output

To verify your logging configuration without sending to GCP:

```bash
# Pretty-print GCP-formatted logs with jq
SCION_LOG_GCP=true scion server start --enable-hub 2>&1 | jq .

# Filter for specific severity
SCION_LOG_GCP=true scion server start --enable-hub 2>&1 | \
  jq 'select(.severity == "ERROR")'

# Extract just messages
SCION_LOG_GCP=true scion server start --enable-hub 2>&1 | \
  jq -r '.message'
```

## Related Guides

- [Observability](/guides/observability) - Full telemetry pipeline setup
- [Metrics](/guides/metrics) - OpenTelemetry metrics configuration
- [Hub Server](/guides/hub-server) - Hub deployment and configuration
- [Runtime Broker](/guides/runtime-broker) - Broker setup
