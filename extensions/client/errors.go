package client

import (
	"errors"
	"firebase.google.com/go/v4/messaging"
	pushererrors "github.com/topfreegames/pusher/errors"
)

// Firebase errors docs can be found here: https://firebase.google.com/docs/cloud-messaging/send-message#admin
var (
	ErrUnspecified         = errors.New("unspecified error")
	ErrInvalidArgument     = errors.New("invalid argument")
	ErrUnregisteredDevice  = errors.New("unregistered device")
	ErrSenderIDMismatch    = errors.New("sender id mismatch")
	ErrQuotaExceeded       = errors.New("quota exceeded")
	ErrUnavailable         = errors.New("unavailable")
	ErrInternalServerError = errors.New("internal server error")
	ErrThirdParyAuthError  = errors.New("third party authentication error")
)

// TranslateError translates a Firebase error into a pusher error.
func translateError(err error) *pushererrors.PushError {
	switch {
	case messaging.IsInvalidArgument(err):
		return pushererrors.NewPushError("invalid_argument", err.Error())
	case messaging.IsUnregistered(err):
		return pushererrors.NewPushError("unregistered_device", err.Error())
	case messaging.IsSenderIDMismatch(err):
		return pushererrors.NewPushError("sender_id_mismatch", err.Error())
	case messaging.IsQuotaExceeded(err):
		return pushererrors.NewPushError("quota_exceeded", err.Error())
	case messaging.IsUnavailable(err):
		return pushererrors.NewPushError("unavailable", err.Error())
	case messaging.IsInternal(err):
		return pushererrors.NewPushError("firebase_internal_error", err.Error())
	case messaging.IsThirdPartyAuthError(err):
		return pushererrors.NewPushError("third_party_auth_error", err.Error())
	default:
		return pushererrors.NewPushError("unknown", err.Error())
	}

	return nil
}
