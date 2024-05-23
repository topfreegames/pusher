package handler

import "time"

type messageHandlerConfig struct {
	statusLogInterval time.Duration
}

func newDefaultMessageHandlerConfig() messageHandlerConfig {
	return messageHandlerConfig{
		statusLogInterval: 5 * time.Second,
	}
}
