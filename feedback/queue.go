package feedback

import "sync"

// Queue interface for making new queues pluggable easily
type Queue interface {
	MessagesChannel() *chan []byte
	ConsumeLoop() error
	StopConsuming()
	PendingMessagesWaitGroup() *sync.WaitGroup
}
