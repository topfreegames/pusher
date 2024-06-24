package structs

import "github.com/sideshow/apns2"

type ApnsNotification struct {
	apns2.Notification
	Metadata     map[string]interface{}
	SendAttempts int
}
