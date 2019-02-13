package feedback

import "fmt"

type InvalidToken struct {
	Token    string
	Game     string
	Platform string
}

type InvalidTokenHandler struct {
}

func NewInvalidTokenHandler() *InvalidTokenHandler {
	return &InvalidTokenHandler{}

	// Start Handler

}

func (i *InvalidTokenHandler) HandleMessage(msg *Message) {
	fmt.Println("InvalidTOkenHandler got message: ", msg)
}
