package feedback

import "errors"

// Errors
var (
	ErrAPNSUnmarshal        = errors.New("error unmarshalling apns message")
	ErrGCMUnmarshal         = errors.New("error unmarshalling gcm message")
	ErrInvalidTokenChanFull = errors.New("invalid token out channel full")
)

const (
	APNSPlatform = "apns"
	GCMPlatform  = "gcm"
)
