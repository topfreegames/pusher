// Code generated by MockGen. DO NOT EDIT.
// Source: interfaces/statsd.go
//
// Generated by this command:
//
//	mockgen -source=interfaces/statsd.go -destination=mocks/interfaces/statsd.go
//

// Package mock_interfaces is a generated GoMock package.
package mock_interfaces

import (
	reflect "reflect"
	time "time"

	gomock "go.uber.org/mock/gomock"
)

// MockStatsDClient is a mock of StatsDClient interface.
type MockStatsDClient struct {
	ctrl     *gomock.Controller
	recorder *MockStatsDClientMockRecorder
}

// MockStatsDClientMockRecorder is the mock recorder for MockStatsDClient.
type MockStatsDClientMockRecorder struct {
	mock *MockStatsDClient
}

// NewMockStatsDClient creates a new mock instance.
func NewMockStatsDClient(ctrl *gomock.Controller) *MockStatsDClient {
	mock := &MockStatsDClient{ctrl: ctrl}
	mock.recorder = &MockStatsDClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockStatsDClient) EXPECT() *MockStatsDClientMockRecorder {
	return m.recorder
}

// Close mocks base method.
func (m *MockStatsDClient) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close.
func (mr *MockStatsDClientMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockStatsDClient)(nil).Close))
}

// Count mocks base method.
func (m *MockStatsDClient) Count(arg0 string, arg1 int64, arg2 []string, arg3 float64) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Count", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(error)
	return ret0
}

// Count indicates an expected call of Count.
func (mr *MockStatsDClientMockRecorder) Count(arg0, arg1, arg2, arg3 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Count", reflect.TypeOf((*MockStatsDClient)(nil).Count), arg0, arg1, arg2, arg3)
}

// Gauge mocks base method.
func (m *MockStatsDClient) Gauge(arg0 string, arg1 float64, arg2 []string, arg3 float64) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Gauge", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(error)
	return ret0
}

// Gauge indicates an expected call of Gauge.
func (mr *MockStatsDClientMockRecorder) Gauge(arg0, arg1, arg2, arg3 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Gauge", reflect.TypeOf((*MockStatsDClient)(nil).Gauge), arg0, arg1, arg2, arg3)
}

// Incr mocks base method.
func (m *MockStatsDClient) Incr(arg0 string, arg1 []string, arg2 float64) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Incr", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// Incr indicates an expected call of Incr.
func (mr *MockStatsDClientMockRecorder) Incr(arg0, arg1, arg2 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Incr", reflect.TypeOf((*MockStatsDClient)(nil).Incr), arg0, arg1, arg2)
}

// Timing mocks base method.
func (m *MockStatsDClient) Timing(arg0 string, arg1 time.Duration, arg2 []string, arg3 float64) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Timing", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(error)
	return ret0
}

// Timing indicates an expected call of Timing.
func (mr *MockStatsDClientMockRecorder) Timing(arg0, arg1, arg2, arg3 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Timing", reflect.TypeOf((*MockStatsDClient)(nil).Timing), arg0, arg1, arg2, arg3)
}
