package e2e

import "time"

const (
	wait              = 5 * time.Second
	timeout           = 2 * time.Minute
	apnsTopicTemplate = "push-%s_apns-single"
	gcmTopicTemplate  = "push-%s_gcm-single"
)
