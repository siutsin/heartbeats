package controller

// Error messages
const (
	ErrFailedToCreateRequest  = "failed to create request"
	ErrFailedToMakeRequest    = "failed to make request after maximum attempts"
	ErrFailedToGetSecret      = "failed to get secret"
	ErrMissingRequiredKey     = "missing required key"
	ErrEndpointTimeout        = "endpoint timed out"
	ErrFailedToCheckEndpoint  = "failed to check endpoint health"
	ErrInvalidStatusCodeRange = "invalid status code range"
	ErrFailedToUpdateStatus   = "failed to update Heartbeat status"
	ErrEmptyEndpoint          = "endpoint URL is empty"
	ErrStatusCodeNotInRange   = "status code is not within expected ranges"
	ErrEndpointHealthy        = "endpoint is healthy"
	ErrEndpointNotSpecified   = "endpoint is not specified"
)
