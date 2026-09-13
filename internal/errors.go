package heartbeats

// Error message constants for consistent status reporting.
const (
	// ErrFailedToCreateRequest indicates that an HTTP request could not be created.
	ErrFailedToCreateRequest = "failed to create request"

	// ErrFailedToMakeRequest indicates that an HTTP request failed after maximum retry attempts.
	ErrFailedToMakeRequest = "failed to make request after maximum attempts"

	// ErrFailedToGetSecret indicates that a Kubernetes secret could not be retrieved.
	ErrFailedToGetSecret = "failed to get secret"

	// ErrMissingRequiredKey indicates that a required key is missing from a Kubernetes secret.
	ErrMissingRequiredKey = "missing required key"

	// ErrEndpointTimeout indicates that an endpoint request timed out.
	ErrEndpointTimeout = "endpoint timed out"

	// ErrFailedToCheckEndpoint indicates that the health check process failed.
	ErrFailedToCheckEndpoint = "failed to check endpoint health"

	// ErrInvalidStatusCodeRange indicates that a status code range has invalid min/max values.
	ErrInvalidStatusCodeRange = "invalid status code range"

	// ErrFailedToUpdateStatus indicates that the Heartbeat status could not be updated.
	ErrFailedToUpdateStatus = "failed to update Heartbeat status"

	// ErrStatusCodeNotInRange indicates that the endpoint returned a status code outside expected ranges.
	ErrStatusCodeNotInRange = "status code is not within expected ranges"

	// ErrEndpointHealthy indicates that the endpoint is healthy and responding correctly.
	ErrEndpointHealthy = "endpoint is healthy"

	// ErrEndpointNotSpecified indicates that an endpoint URL is not specified.
	ErrEndpointNotSpecified = "endpoint is not specified"
)
