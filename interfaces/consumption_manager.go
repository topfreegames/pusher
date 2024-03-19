package interfaces

type ConsumptionManager interface {
	Pause(topic string) error
	Resume(topic string) error
}
