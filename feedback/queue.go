package feedback

import "sync"

// KafkaMessage sent through the Channel
type KafkaMessage struct {
	Game     string
	Platform string
	// Topic    string
	Value []byte
}

// Queue interface for making new queues pluggable easily
type Queue interface {
	MessagesChannel() *chan *KafkaMessage
	ConsumeLoop() error
	StopConsuming()
	PendingMessagesWaitGroup() *sync.WaitGroup
}
