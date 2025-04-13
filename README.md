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
and updates their status based on HTTP response codes.

## Features

- Monitor HTTP endpoints with configurable intervals
- Define custom status code ranges for healthy/unhealthy states
- Store endpoint URLs securely in Kubernetes secrets
- Real-time status updates with detailed health information
- Configurable timeout and retry settings
- Prometheus metrics integration

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
      healthyEndpoint: "https://httpbin.org/status/200"
      unhealthyEndpoint: "https://httpbin.org/status/500"
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
        healthyEndpointKey: healthyEndpoint
        unhealthyEndpointKey: unhealthyEndpoint
      expectedStatusCodeRanges:
        - min: 200
          max: 299
      interval: 30s
    ```

### Configuration Options

#### Heartbeat Spec

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| endpointsSecret | object | Reference to the secret containing endpoint URLs | Yes |
| expectedStatusCodeRanges | array | Ranges of HTTP status codes considered healthy | Yes |
| interval | string | Time between health checks (e.g., "30s", "5m", "1h") | Yes |

#### EndpointsSecret

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| name | string | Name of the secret | Yes |
| namespace | string | Namespace of the secret (defaults to Heartbeat's namespace) | No |
| targetEndpointKey | string | Key containing the target endpoint URL | Yes |
| healthyEndpointKey | string | Key containing the healthy endpoint URL | Yes |
| unhealthyEndpointKey | string | Key containing the unhealthy endpoint URL | Yes |

#### StatusCodeRange

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| min | integer | Minimum status code in range (100-599) | Yes |
| max | integer | Maximum status code in range (100-599) | Yes |

### Status Information

The Heartbeat resource's status includes:

- `healthy`: Boolean indicating if the endpoint is healthy
- `lastStatus`: Last HTTP status code received
- `message`: Human-readable status message
- `lastChecked`: Timestamp of the last health check

## Monitoring

The operator exposes Prometheus metrics at `/metrics`:

- `heartbeat_health_status`: Gauge indicating endpoint health
  (1 for healthy, 0 for unhealthy)
- `heartbeat_http_status_code`: Gauge showing the last HTTP status code
- `heartbeat_check_duration_seconds`: Histogram of health check durations

## Troubleshooting

Common issues and solutions:

1. **Endpoint Timeout**
   - Check if the endpoint is accessible from the cluster
   - Verify network policies allow the connection
   - Consider increasing the timeout duration
2. **Invalid Status Code**
   - Ensure the endpoint returns expected status codes
   - Verify the status code ranges in the Heartbeat spec
3. **Secret Not Found**
   - Confirm the secret exists in the correct namespace
   - Verify the secret keys match the Heartbeat configuration

## Contributing

Contributions are welcome! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
