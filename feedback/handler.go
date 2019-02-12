package feedback

type Handler interface {
	HandleMessage(msg *Message)
}
