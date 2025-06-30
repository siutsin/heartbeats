# Heartbeats Operator

A Kubernetes operator that monitors the health of HTTP endpoints by periodically sending requests and tracking their responses.

## Disclaimer

> [!WARNING]
> AI-GENERATED CODE AHEAD! Every single line of code in this repository is generated.

This entire repo is the result of me going "Hey, I wonder if I can vibe-code a Kubernetes operator in a weekend?"

Me over the weekend:

[![Vibe Coding](https://markdown-videos-api.jorgenkh.no/url?url=https%3A%2F%2Fyoutu.be%2F_2C2CNmK7dQ)](https://youtu.be/_2C2CNmK7dQ)

Buddy-coded with Cursor Pro, 307 premium model requests.

> [!WARNING]
> Use at your own risk. No guarantees, no warranties, just pure vibes and questionable life choices. 🤪

## Overview

The Heartbeats Operator provides a custom resource `Heartbeat` that allows you to monitor
the health of HTTP endpoints in your Kubernetes cluster. It periodically checks the endpoints
and updates their status based on HTTP response codes with comprehensive error handling and reporting.

## Features

- **Robust Health Monitoring**: Monitor HTTP endpoints with configurable intervals and retry logic
- **Flexible Status Code Ranges**: Define custom status code ranges for healthy/unhealthy states
- **Secure Configuration**: Store endpoint URLs securely in Kubernetes secrets
- **Real-time Status Updates**: Detailed health information with comprehensive error messages
- **Configurable Timeouts**: Adjustable timeout and retry settings for different network conditions
- **Prometheus Metrics**: Built-in metrics integration for monitoring and alerting
- **Comprehensive Error Handling**: Detailed error messages for troubleshooting
- **Report Endpoints**: Optional reporting to healthy/unhealthy endpoints for external monitoring systems
- **Production Ready**: Structured logging, proper error handling, and comprehensive test coverage

## Installation

### Prerequisites

- Kubernetes cluster (v1.16+)
- kubectl configured to access your cluster
- cert-manager installed (for webhook certificates)

### Using Helm

```bash
helm repo add heartbeats https://siutsin.github.io/heartbeats
helm install heartbeats heartbeats/heartbeats-operator
```

### Manual Installation

1. Install the CRDs:

    ```bash
    kubectl apply -f config/crd/bases/monitoring.siutsin.com_heartbeats.yaml
    ```

2. Deploy the operator:

    ```bash
    kubectl apply -f config/default/
    ```

## Usage

### Basic Example

1. Create a secret containing your endpoint URLs:

    ```yaml
    apiVersion: v1
    kind: Secret
    metadata:
      name: heartbeat-endpoints
    type: Opaque
    stringData:
      targetEndpoint: "https://api.example.com/health"
      targetEndpointMethod: "GET" # Optional, defaults to GET
      healthyEndpoint: "https://httpbin.org/status/200"
      healthyEndpointMethod: "POST" # Optional, defaults to GET
      unhealthyEndpoint: "https://httpbin.org/status/500"
      unhealthyEndpointMethod: "POST" # Optional, defaults to GET
    ```

2. Create a Heartbeat resource:

    ```yaml
    apiVersion: monitoring.siutsin.com/v1alpha1
    kind: Heartbeat
    metadata:
      name: api-health
    spec:
      endpointsSecret:
        name: heartbeat-endpoints
        targetEndpointKey: targetEndpoint
        targetEndpointMethodKey: targetEndpointMethod # Optional
        healthyEndpointKey: healthyEndpoint
        healthyEndpointMethodKey: healthyEndpointMethod # Optional
        unhealthyEndpointKey: unhealthyEndpoint
        unhealthyEndpointMethodKey: unhealthyEndpointMethod # Optional
      expectedStatusCodeRanges:
        - min: 200
          max: 299
      interval: 30s
    ```

### Advanced Example with Report Endpoints

```yaml
apiVersion: monitoring.siutsin.com/v1alpha1
kind: Heartbeat
metadata:
  name: api-health-with-reporting
spec:
  endpointsSecret:
    name: heartbeat-endpoints
    targetEndpointKey: targetEndpoint
    targetEndpointMethodKey: targetEndpointMethod # Optional
    healthyEndpointKey: healthyEndpoint
    healthyEndpointMethodKey: healthyEndpointMethod # Optional
    unhealthyEndpointKey: unhealthyEndpoint
    unhealthyEndpointMethodKey: unhealthyEndpointMethod # Optional
  expectedStatusCodeRanges:
    - min: 200
      max: 299
    - min: 404
      max: 404  # Accept 404 as healthy for certain endpoints
  interval: 30s
```

### Configuration Options

#### Heartbeat Spec

| Field                    | Type   | Description                                          | Required |
|--------------------------|--------|------------------------------------------------------|----------|
| endpointsSecret          | object | Reference to the secret containing endpoint URLs     | Yes      |
| expectedStatusCodeRanges | array  | Ranges of HTTP status codes considered healthy       | Yes      |
| interval                 | string | Time between health checks (e.g., "30s", "5m", "1h") | Yes      |

#### EndpointsSecret

| Field                      | Type   | Description                                                                 | Required |
|----------------------------|--------|-----------------------------------------------------------------------------|----------|
| name                       | string | Name of the secret                                                          | Yes      |
| namespace                  | string | Namespace of the secret (defaults to Heartbeat's namespace)                 | No       |
| targetEndpointKey          | string | Key containing the target endpoint URL                                      | Yes      |
| targetEndpointMethodKey    | string | Key containing the HTTP method for the target endpoint (e.g., GET, POST)    | No       |
| healthyEndpointKey         | string | Key containing the healthy endpoint URL for reporting                       | Yes      |
| healthyEndpointMethodKey   | string | Key containing the HTTP method for the healthy endpoint (e.g., GET, POST)   | No       |
| unhealthyEndpointKey       | string | Key containing the unhealthy endpoint URL for reporting                     | Yes      |
| unhealthyEndpointMethodKey | string | Key containing the HTTP method for the unhealthy endpoint (e.g., GET, POST) | No       |

#### StatusCodeRange

| Field | Type    | Description                            | Required |
|-------|---------|----------------------------------------|----------|
| min   | integer | Minimum status code in range (100-599) | Yes      |
| max   | integer | Maximum status code in range (100-599) | Yes      |

### Status Information

The Heartbeat resource's status includes:

- `healthy`: Boolean indicating if the endpoint is healthy
- `lastStatus`: Last HTTP status code received
- `message`: Human-readable status message with detailed error information
- `lastChecked`: Timestamp of the last health check
- `reportStatus`: Status of reporting to external endpoints ("Success" or "Failure")

### Error Messages

The operator provides detailed error messages for troubleshooting:

- `missing required key`: A required endpoint key is missing from the secret
- `endpoint is not specified`: An endpoint URL is empty or not provided
- `failed to check endpoint health`: Network or HTTP errors during health checks
- `endpoint timed out`: The endpoint took too long to respond
- `invalid status code range`: Status code range has invalid min/max values
- `status code is not within expected ranges`: Endpoint returned unexpected status code

## Development

### Development Prerequisites

- Go 1.24+
- Docker
- Kind (for local testing)
- kubectl

### Building and Testing

```bash
# Run unit tests
make test

# Run unit tests with race detection (for CI)
make test-ci

# Run e2e tests locally
make test-e2e LOCAL=true

# Run e2e tests with race detection (for CI)
make test-e2e-ci LOCAL=true

# Build the operator
make build

# Build Docker image
make docker-build
```

### Code Quality

```bash
# Format code
make fmt

# Run linter
make lint

# Run linter with fixes
make lint-fix

# Check markdown files
make lint-markdown
```

### Logging

The operator uses structured logging via the `logr` interface, integrated with controller-runtime's logger.
The `internal/logger` package is generic and only provides helpers for creating loggers with contextual information
(such as namespace and name).

**No business logic or business-specific logging helpers are present in the logger package.**

All business-specific logging (such as health check results, reconciliation events, etc.) is performed inline in the
controller and related business logic, using standard log levels and structured key-value pairs.

#### Example Usage

```go
log := logger.WithRequest(ctx, "heartbeat-reconciler", req.Namespace, req.Name)
log.V(1).Info("Starting reconciliation", "namespace", req.Namespace, "name", req.Name)
log.Error(err, "Failed to fetch resource", "namespace", req.Namespace, "name", req.Name)
```

#### Example Log Output

```json
{
  "time": "2025-06-27T21:58:58.917163+01:00",
  "level": "INFO",
  "msg": "Starting reconciliation",
  "namespace": "default",
  "name": "api-health"
}
```

#### Log Levels

- **INFO**: General operational information, health check results
- **ERROR**: Error conditions, failed health checks, configuration issues
- **DEBUG/V(1)**: Detailed debugging information, HTTP request details
- **V(2)**: Verbose debugging, internal state information

## Monitoring

The operator exposes Prometheus metrics at `/metrics`:

- `heartbeat_health_status`: Gauge indicating endpoint health
  (1 for healthy, 0 for unhealthy)
- `heartbeat_http_status_code`: Gauge showing the last HTTP status code
- `heartbeat_check_duration_seconds`: Histogram of health check durations

### Metrics Access

The metrics endpoint is secured and requires authentication. Access is controlled via RBAC:

```bash
# Get service account token
TOKEN=$(kubectl create token heartbeats-operator-controller-manager -n heartbeats-operator-system)

# Access metrics
curl -k -H "Authorization: Bearer $TOKEN" \
  https://heartbeats-operator-controller-manager-metrics-service.heartbeats-operator-system.svc.cluster.local:8443/metrics
```

## Troubleshooting

### Common Issues

1. **Endpoint Timeout**
   - Check if the endpoint is accessible from the cluster
   - Verify network policies allow the connection
   - Consider increasing the timeout duration
   - Check logs for detailed timeout information

2. **Invalid Status Code**
   - Ensure the endpoint returns expected status codes
   - Verify the status code ranges in the Heartbeat spec
   - Check the status message for specific error details

3. **Secret Not Found**
   - Confirm the secret exists in the correct namespace
   - Verify the secret keys match the Heartbeat configuration
   - Check for "missing required key" error messages

4. **Report Endpoint Failures**
   - Verify the report endpoints are accessible
   - Check the `reportStatus` field in the Heartbeat status
   - Review logs for report endpoint errors

### Debugging

```bash
# Check Heartbeat status
kubectl get heartbeat <name> -o yaml

# View operator logs
kubectl logs -n heartbeats-operator-system deployment/heartbeats-operator-controller-manager

# Check metrics
kubectl port-forward -n heartbeats-operator-system svc/heartbeats-operator-controller-manager-metrics-service 8443:8443
```

## Contributing

Contributions are welcome! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

### Development Guidelines

- Follow the existing code style and patterns
- Add comprehensive unit tests for new features
- Include e2e tests for integration scenarios
- Use structured logging with appropriate log levels
- Follow error handling patterns established in the codebase

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
