package client

import (
	"context"
	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"github.com/sirupsen/logrus"
	"github.com/topfreegames/pusher/interfaces"
	"google.golang.org/api/option"
	"time"
)

type firebaseClientImpl struct {
	firebase *messaging.Client
	logger   *logrus.Logger
}

var _ interfaces.PushClient = &firebaseClientImpl{}

func NewFirebaseClient(jsonCredentials string, logger *logrus.Logger) (interfaces.PushClient, error) {
	ctx := context.Background()
	app, err := firebase.NewApp(ctx, nil, option.WithCredentialsJSON([]byte(jsonCredentials)))
	if err != nil {
		return nil, err
	}

	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, err
	}

	l := logger.WithFields(logrus.Fields{
		"source": "firebaseClient",
	})
	return &firebaseClientImpl{
		firebase: client,
		logger:   l.Logger,
	}, nil
}

func (f *firebaseClientImpl) SendPush(ctx context.Context, msg interfaces.Message) error {
	l := f.logger.WithFields(logrus.Fields{
		"method": "SendPush",
	})

	firebaseMsg := toFirebaseMessage(msg)
	res, err := f.firebase.Send(ctx, &firebaseMsg)
	if err != nil {
		l.WithError(err).Error("error sending message")
		return translateError(err)
	}

	l.Debugf("Successfully sent message: %s", res)

	return nil
}

func toFirebaseMessage(message interfaces.Message) messaging.Message {
	firebaseMessage := messaging.Message{
		Data: nil,
		Notification: &messaging.Notification{
			Title:    message.Notification.Title,
			Body:     message.Notification.Body,
			ImageURL: message.Notification.Icon,
		},
		Android: &messaging.AndroidConfig{
			CollapseKey: message.CollapseKey,
			Priority:    message.Priority,
			Notification: &messaging.AndroidNotification{
				Title:       message.Notification.Title,
				Body:        message.Notification.Body,
				Icon:        message.Notification.Icon,
				Color:       message.Notification.Color,
				Sound:       message.Notification.Sound,
				Tag:         message.Notification.Tag,
				ClickAction: message.Notification.ClickAction,
				BodyLocKey:  message.Notification.BodyLocKey,
				TitleLocKey: message.Notification.TitleLocKey,
			},
		},
		Token: message.To,
	}

	if message.TimeToLive != nil {
		secs := int(*message.TimeToLive)
		ttl := time.Duration(secs) * time.Second
		firebaseMessage.Android.TTL = &ttl
	}

	if message.Notification.BodyLocArgs != "" {
		firebaseMessage.Android.Notification.BodyLocArgs = []string{message.Notification.BodyLocArgs}
	}

	if message.Notification.TitleLocArgs != "" {
		firebaseMessage.Android.Notification.TitleLocArgs = []string{message.Notification.TitleLocArgs}
	}

	return firebaseMessage
}
