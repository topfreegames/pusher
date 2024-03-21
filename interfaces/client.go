package interfaces

import "context"

type PushClient interface {
	SendPush(ctx context.Context, msg Message) error
}

type Message struct {
	To                       string        `json:"to,omitempty"`
	MessageID                string        `json:"message_id"`
	MessageType              string        `json:"message_type,omitempty"`
	CollapseKey              string        `json:"collapse_key,omitempty"`
	Priority                 string        `json:"priority,omitempty"`
	ContentAvailable         bool          `json:"content_available,omitempty"`
	TimeToLive               *uint         `json:"time_to_live,omitempty"`
	DeliveryReceiptRequested bool          `json:"delivery_receipt_requested,omitempty"`
	DryRun                   bool          `json:"dry_run,omitempty"`
	Data                     Data          `json:"data,omitempty"`
	Notification             *Notification `json:"notification,omitempty"`
}

// Data defines the custom payload of a message.
type Data map[string]interface{}

// Notification defines the notification payload of a GCM message.
// NOTE: contains keys for both Android and iOS notifications.
type Notification struct {
	// Common fields.
	Title        string `json:"title,omitempty"`
	Body         string `json:"body,omitempty"`
	Sound        string `json:"sound,omitempty"`
	ClickAction  string `json:"click_action,omitempty"`
	BodyLocKey   string `json:"body_loc_key,omitempty"`
	BodyLocArgs  string `json:"body_loc_args,omitempty"`
	TitleLocKey  string `json:"title_loc_key,omitempty"`
	TitleLocArgs string `json:"title_loc_args,omitempty"`

	// Android-only fields.
	Icon  string `json:"icon,omitempty"`
	Tag   string `json:"tag,omitempty"`
	Color string `json:"color,omitempty"`

	// iOS-only fields
	Badge string `json:"badge,omitempty"`
}
