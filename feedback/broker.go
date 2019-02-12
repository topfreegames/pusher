package feedback

import (
	"encoding/json"
	"fmt"

	log "github.com/sirupsen/logrus"
)

// Message is a struct that will decode a apns or gcm feedback message
type Message struct {
	From             string                 `json:"from"`
	MessageID        string                 `json:"message_id"`
	MessageType      string                 `json:"message_type"`
	Error            string                 `json:"error"`
	ErrorDescription string                 `json:"error_description"`
	DeviceToken      string                 `json:"DeviceToken"`
	ID               string                 `json:"id"`
	Err              map[string]interface{} `json:"Err"`
	Metadata         map[string]interface{} `json:"metadata"`
}

type Broker struct {
	Logger              *log.Logger
	IncomeChannel       *chan []byte
	InvalidTokenHandler Handler
	run                 bool
}

func NewBroker(logger *log.Logger, ichan *chan []byte,
	invalidTokenHandler Handler,
) *Broker {
	b := &Broker{
		Logger:              logger,
		IncomeChannel:       ichan,
		InvalidTokenHandler: invalidTokenHandler,
	}

	return b
}

func (b *Broker) Start() {
	l := b.Logger.WithField(
		"operation", "Broker.Start",
	)
	b.run = true
	// go h.flushFeedbacks()
	for b.run == true {
		select {
		case msg := <-*b.IncomeChannel:
			fmt.Println("INCOME CHANNEL HAS: ", msg)
			var message Message
			err := json.Unmarshal(msg, &message)
			if err != nil {
				l.WithError(err).Error("error unmarshelling kafka message")
			}
			b.routeMessage(&message)
		}
	}
}

func (b *Broker) routeMessage(msg *Message) {
	fmt.Println("GOT MESSAGE", msg)

	switch msg.Error {
	case "DEVICE_UNREGISTERED", "BAD_REGISTRATION",
		"BadDeviceToken", "Unregistered", "TopicDisallowed", "DeviceTokenNotForTopic":
		b.InvalidTokenHandler.HandleMessage(msg)
	}
}
