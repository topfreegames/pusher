package feedback

import "sync"

// FeedbackMessage sent through the Channel
type FeedbackMessage struct {
	Game     string
	Platform string
	Value    []byte
}

// Queue interface for making new queues pluggable easily
type Queue interface {
	MessagesChannel() *chan *FeedbackMessage
	ConsumeLoop() error
	StopConsuming()
	Cleanup() error
	PendingMessagesWaitGroup() *sync.WaitGroup
}
