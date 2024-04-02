package handler

// FeedbackResponse struct is sent to feedback reporters
// in order to keep the format expected by it
type FeedbackResponse struct {
	From             string `json:"from,omitempty"`
	Error            string `json:"error,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
}
