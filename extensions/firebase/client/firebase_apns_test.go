package client

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/topfreegames/pusher/interfaces"
)

func TestToFirebaseMessage_IOSBuildsAPNSOnly(t *testing.T) {
	msg := interfaces.Message{
		To:       "ios-token",
		Platform: "ios",
		Notification: &interfaces.Notification{
			Title: "hello",
			Body:  "world",
			Sound: "default",
		},
	}

	out := toFirebaseMessage(msg)

	require.NotNil(t, out.APNS)
	require.NotNil(t, out.APNS.Payload)
	require.NotNil(t, out.APNS.Payload.Aps)
	require.NotNil(t, out.APNS.Payload.Aps.Alert)
	assert.Equal(t, "hello", out.APNS.Payload.Aps.Alert.Title)
	assert.Equal(t, "world", out.APNS.Payload.Aps.Alert.Body)
	assert.Equal(t, "default", out.APNS.Payload.Aps.Sound)
	assert.Nil(t, out.Android, "iOS message must not populate Android config")
}

func TestToFirebaseMessage_GCMBuildsAndroidOnly(t *testing.T) {
	msg := interfaces.Message{
		To:       "android-token",
		Platform: "gcm",
		Notification: &interfaces.Notification{
			Title: "hello",
			Body:  "world",
		},
	}

	out := toFirebaseMessage(msg)

	require.NotNil(t, out.Android)
	require.NotNil(t, out.Android.Notification)
	assert.Equal(t, "hello", out.Android.Notification.Title)
	assert.Nil(t, out.APNS, "Android message must not populate APNS config")
}

func TestToFirebaseMessage_EmptyPlatformDefaultsToAndroid(t *testing.T) {
	msg := interfaces.Message{
		To:       "android-token",
		Platform: "",
		Notification: &interfaces.Notification{
			Title: "hello",
			Body:  "world",
		},
	}

	out := toFirebaseMessage(msg)

	require.NotNil(t, out.Android)
	assert.Nil(t, out.APNS)
}

func TestBuildIOSMessage_BadgeStringToInt(t *testing.T) {
	msg := interfaces.Message{
		To:       "ios-token",
		Platform: "ios",
		Notification: &interfaces.Notification{
			Title: "t",
			Body:  "b",
			Badge: "42",
		},
	}

	out := toFirebaseMessage(msg)

	require.NotNil(t, out.APNS.Payload.Aps.Badge)
	assert.Equal(t, 42, *out.APNS.Payload.Aps.Badge)
}

func TestBuildIOSMessage_BadgeInvalidStringIsOmitted(t *testing.T) {
	msg := interfaces.Message{
		To:       "ios-token",
		Platform: "ios",
		Notification: &interfaces.Notification{
			Title: "t",
			Body:  "b",
			Badge: "not-a-number",
		},
	}

	out := toFirebaseMessage(msg)

	assert.Nil(t, out.APNS.Payload.Aps.Badge)
}

func TestBuildIOSMessage_BadgeEmptyIsOmitted(t *testing.T) {
	msg := interfaces.Message{
		To:       "ios-token",
		Platform: "ios",
		Notification: &interfaces.Notification{
			Title: "t",
			Body:  "b",
		},
	}

	out := toFirebaseMessage(msg)

	assert.Nil(t, out.APNS.Payload.Aps.Badge)
}

func TestBuildIOSMessage_SilentPush(t *testing.T) {
	msg := interfaces.Message{
		To:               "ios-token",
		Platform:         "ios",
		ContentAvailable: true,
		Data:             interfaces.Data{"k": "v"},
	}

	out := toFirebaseMessage(msg)

	require.NotNil(t, out.APNS)
	require.NotNil(t, out.APNS.Payload)
	require.NotNil(t, out.APNS.Payload.Aps)
	assert.True(t, out.APNS.Payload.Aps.ContentAvailable)
	assert.Nil(t, out.APNS.Payload.Aps.Alert, "silent push must not include an alert")
	assert.Nil(t, out.Notification, "silent push must not include a top-level Notification")
	assert.Equal(t, map[string]string{"k": "v"}, out.Data)
}

func TestBuildIOSMessage_CollapseKeyAndTTLProduceHeaders(t *testing.T) {
	ttl := uint(60)
	msg := interfaces.Message{
		To:          "ios-token",
		Platform:    "ios",
		CollapseKey: "collapse-1",
		Priority:    "10",
		TimeToLive:  &ttl,
		Notification: &interfaces.Notification{
			Title: "t",
			Body:  "b",
		},
	}

	before := time.Now().Unix()
	out := toFirebaseMessage(msg)
	after := time.Now().Unix()

	require.NotNil(t, out.APNS)
	headers := out.APNS.Headers
	assert.Equal(t, "collapse-1", headers["apns-collapse-id"])
	assert.Equal(t, "10", headers["apns-priority"])

	expStr, ok := headers["apns-expiration"]
	require.True(t, ok, "apns-expiration header must be set when TTL provided")
	exp, err := strconv.ParseInt(expStr, 10, 64)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, exp, before+int64(ttl))
	assert.LessOrEqual(t, exp, after+int64(ttl))
}

func TestBuildIOSMessage_OmitsHeadersWhenNotProvided(t *testing.T) {
	msg := interfaces.Message{
		To:       "ios-token",
		Platform: "ios",
		Notification: &interfaces.Notification{
			Title: "t",
			Body:  "b",
		},
	}

	out := toFirebaseMessage(msg)

	require.NotNil(t, out.APNS)
	_, hasCollapse := out.APNS.Headers["apns-collapse-id"]
	_, hasPriority := out.APNS.Headers["apns-priority"]
	_, hasExpiration := out.APNS.Headers["apns-expiration"]
	assert.False(t, hasCollapse)
	assert.False(t, hasPriority)
	assert.False(t, hasExpiration)
}

func TestBuildIOSMessage_LocKeysAndArgs(t *testing.T) {
	msg := interfaces.Message{
		To:       "ios-token",
		Platform: "ios",
		Notification: &interfaces.Notification{
			BodyLocKey:   "body.key",
			BodyLocArgs:  "arg1",
			TitleLocKey:  "title.key",
			TitleLocArgs: "title-arg",
		},
	}

	out := toFirebaseMessage(msg)

	require.NotNil(t, out.APNS.Payload.Aps.Alert)
	alert := out.APNS.Payload.Aps.Alert
	assert.Equal(t, "body.key", alert.LocKey)
	assert.Equal(t, []string{"arg1"}, alert.LocArgs)
	assert.Equal(t, "title.key", alert.TitleLocKey)
	assert.Equal(t, []string{"title-arg"}, alert.TitleLocArgs)
}

