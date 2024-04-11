package interfaces

import "github.com/topfreegames/pusher/interfaces"

type MockConsumptionManager struct {
}

func (m *MockConsumptionManager) Pause(topic string) error {
	return nil
}

func (m *MockConsumptionManager) Resume(topic string) error {
	return nil
}

func NewMockConsumptionManager() *MockConsumptionManager {
	return &MockConsumptionManager{}
}

var _ interfaces.ConsumptionManager = &MockConsumptionManager{}
