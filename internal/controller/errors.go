package controller

// Error messages used throughout the controller for consistent error reporting.
// These constants provide standardised error messages for various failure scenarios.
const (
	// ErrFailedToCreateRequest indicates that an HTTP request could not be created.
	// This typically occurs when the endpoint URL is malformed or invalid.
	ErrFailedToCreateRequest = "failed to create request"

	// ErrFailedToMakeRequest indicates that an HTTP request failed after maximum retry attempts.
	// This includes network errors, connection refused, DNS resolution failures, etc.
	ErrFailedToMakeRequest = "failed to make request after maximum attempts"

	// ErrFailedToGetSecret indicates that a Kubernetes secret could not be retrieved.
	// This occurs when the secret doesn't exist or the controller lacks permissions.
	ErrFailedToGetSecret = "failed to get secret"

	// ErrMissingRequiredKey indicates that a required key is missing from a Kubernetes secret.
	// This happens when the secret exists but doesn't contain the expected endpoint keys.
	ErrMissingRequiredKey = "missing required key"

	// ErrEndpointTimeout indicates that an endpoint request timed out.
	// This occurs when the endpoint takes longer than the configured timeout to respond.
	ErrEndpointTimeout = "endpoint timed out"

	// ErrFailedToCheckEndpoint indicates that the health check process failed.
	// This is a general error for any failure during endpoint health checking.
	ErrFailedToCheckEndpoint = "failed to check endpoint health"

	// ErrInvalidStatusCodeRange indicates that a status code range has invalid min/max values.
	// This occurs when the minimum value is greater than the maximum value.
	ErrInvalidStatusCodeRange = "invalid status code range"

	// ErrFailedToUpdateStatus indicates that the Heartbeat status could not be updated.
	// This occurs when there are permission issues or the resource has been modified.
	ErrFailedToUpdateStatus = "failed to update Heartbeat status"

	// ErrEmptyEndpoint indicates that an endpoint URL is empty or not specified.
	// This occurs when the secret contains an empty string for an endpoint key.
	ErrEmptyEndpoint = "endpoint URL is empty"

	// ErrStatusCodeNotInRange indicates that the endpoint returned a status code outside expected ranges.
	// This occurs when the endpoint is healthy but returns an unexpected status code.
	ErrStatusCodeNotInRange = "status code is not within expected ranges"

	// ErrEndpointHealthy indicates that the endpoint is healthy and responding correctly.
	// This is a success message indicating the endpoint passed all health checks.
	ErrEndpointHealthy = "endpoint is healthy"

	// ErrEndpointNotSpecified indicates that an endpoint URL is not specified.
	// This occurs when the endpoint key is missing from the secret.
	ErrEndpointNotSpecified = "endpoint is not specified"
)
