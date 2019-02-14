package feedback

import (
	"encoding/json"

	gcm "github.com/topfreegames/go-gcm"
	"github.com/topfreegames/pusher/structs"

	"github.com/sideshow/apns2"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
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

	Reason string
}

// Broker receives kafka messages in its InChan, unmarshal them according to the
// platform and routes them to the correct out channel after examining their content
type Broker struct {
	Logger              *log.Logger
	Config              *viper.Viper
	InChan              *chan *KafkaMessage
	InvalidTokenOutChan chan *InvalidToken

	run         bool
	stopChannel chan struct{}
}

// NewBroker creates a new Broker instance
func NewBroker(
	logger *log.Logger, cfg *viper.Viper,
	inChan *chan *KafkaMessage,
) *Broker {
	b := &Broker{
		Logger:              logger,
		Config:              cfg,
		InChan:              inChan,
		InvalidTokenOutChan: make(chan *InvalidToken, 100),
		stopChannel:         make(chan struct{}),
	}

	// TODO Setup default values and read them from config file
	// input and output sizes
	return b
}

// Start starts a routine to process the Broker in channel
func (b *Broker) Start() {
	b.run = true
	go b.processMessages()
}

// Stop stops all routines from processing the in channel and close all output channels
func (b *Broker) Stop() {
	b.run = false
	close(b.stopChannel)
	close(b.InvalidTokenOutChan)
}

func (b *Broker) processMessages() {
	l := b.Logger.WithField(
		"operation", "Broker.Start",
	)

	for b.run == true {
		select {
		case msg := <-*b.InChan:
			switch msg.Platform {
			case APNSPlatform:
				var res structs.ResponseWithMetadata
				err := json.Unmarshal(msg.Value, &res)
				if err != nil {
					l.WithError(err).Error(ErrAPNSUnmarshal.Error())
				}
				b.routeAPNSMessage(&res, msg.Game)

			case GCMPlatform:
				var res gcm.CCSMessage
				err := json.Unmarshal(msg.Value, &res)
				if err != nil {
					l.WithError(err).Error(ErrGCMUnmarshal.Error())
				}
				b.routeGCMMessage(&res, msg.Game)
			}

		case <-b.stopChannel:
			break
		}

	}

	l.Info("stop processing Broker's in channel")
}

func (b *Broker) routeAPNSMessage(msg *structs.ResponseWithMetadata, game string) {
	switch msg.Reason {
	case apns2.ReasonBadDeviceToken, apns2.ReasonUnregistered, apns2.ReasonTopicDisallowed, apns2.ReasonDeviceTokenNotForTopic:
		tk := &InvalidToken{
			Token:    msg.DeviceToken,
			Game:     game,
			Platform: APNSPlatform,
		}

		select {
		case b.InvalidTokenOutChan <- tk:
		default:
			b.Logger.Error(ErrInvalidTokenChanFull.Error())
		}
	}
}

func (b *Broker) routeGCMMessage(msg *gcm.CCSMessage, game string) {
	switch msg.Error {
	case "DEVICE_UNREGISTERED", "BAD_REGISTRATION":
		tk := &InvalidToken{
			Token:    msg.From,
			Game:     game,
			Platform: GCMPlatform,
		}

		select {
		case b.InvalidTokenOutChan <- tk:
		default:
			b.Logger.Error(ErrInvalidTokenChanFull.Error())
		}
	}
}
