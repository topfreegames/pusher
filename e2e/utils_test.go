package e2e

import "time"

const (
	wait              = 1 * time.Second
	timeout           = 3 * time.Minute
	apnsTopicTemplate = "push-%s_apns-single"
	gcmTopicTemplate  = "push-%s_gcm-single"
)
