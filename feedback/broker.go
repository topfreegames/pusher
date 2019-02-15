package feedback

import (
	"encoding/json"
	"fmt"
	"sync"

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
	InChan              *chan *FeedbackMessage
	pendingMessagesWG   *sync.WaitGroup
	InvalidTokenOutChan chan *InvalidToken

	run         bool
	stopChannel chan struct{}
}

// NewBroker creates a new Broker instance
func NewBroker(
	logger *log.Logger, cfg *viper.Viper,
	inChan *chan *FeedbackMessage,
	pendingMessagesWG *sync.WaitGroup,
) (*Broker, error) {
	b := &Broker{
		Logger:            logger,
		Config:            cfg,
		InChan:            inChan,
		pendingMessagesWG: pendingMessagesWG,
		stopChannel:       make(chan struct{}),
	}

	b.configure()
	fmt.Println("BROKER OUT CHAN SIZE", cap(b.InvalidTokenOutChan))

	return b, nil
}

func (b *Broker) loadConfigurationDefaults() {
	b.Config.SetDefault("feedbackListeners.broker.invalidTokenChan.size", 1000)
}

func (b *Broker) configure() {
	b.loadConfigurationDefaults()

	b.InvalidTokenOutChan = make(chan *InvalidToken, b.Config.GetInt("feedbackListeners.broker.invalidTokenChan.size"))
}

// Start starts a routine to process the Broker in channel
func (b *Broker) Start() {
	l := b.Logger.WithField(
		"operation", "start",
	)
	l.Info("starting broker")
	fmt.Println("broker started")
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
		"operation", "processMessages",
	)

	for b.run == true {
		select {
		case msg, ok := <-*b.InChan:
			if ok {
				fmt.Println("BROKER GOT MESSAGE IN IN CHANNEL", msg)
				switch msg.Platform {
				case APNSPlatform:
					var res structs.ResponseWithMetadata
					err := json.Unmarshal(msg.Value, &res)
					if err != nil {
						l.WithError(err).Error(ErrAPNSUnmarshal.Error())
					}
					b.routeAPNSMessage(&res, msg.Game)
					b.confirmMessage()

				case GCMPlatform:
					var res gcm.CCSMessage
					err := json.Unmarshal(msg.Value, &res)
					if err != nil {
						l.WithError(err).Error(ErrGCMUnmarshal.Error())
					}
					fmt.Println("GOT GCM MESSAGE!!!!", res)
					b.routeGCMMessage(&res, msg.Game)
					b.confirmMessage()
				}
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

func (b *Broker) confirmMessage() {
	if b.pendingMessagesWG != nil {
		b.pendingMessagesWG.Done()
	}
}
