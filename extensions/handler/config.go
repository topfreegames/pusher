package handler

import "time"

type messageHandlerConfig struct {
	statusLogInterval          time.Duration
	concurrentResponseHandlers int
}

func newDefaultMessageHandlerConfig() messageHandlerConfig {
	return messageHandlerConfig{
		statusLogInterval:          5 * time.Second,
		concurrentResponseHandlers: 1,
	}
}
