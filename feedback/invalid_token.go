package feedback

import "fmt"

type InvalidTokenHandler struct {
}

func NewInvalidTokenHandler() *InvalidTokenHandler {
	return &InvalidTokenHandler{}
}

func (i *InvalidTokenHandler) HandleMessage(msg *Message) {
	fmt.Println("InvalidTOkenHandler got message: ", msg)
}
