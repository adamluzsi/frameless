// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/adamluzsi/frameless/resources/specs (interfaces: Resource)

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	frameless "github.com/adamluzsi/frameless"
	gomock "github.com/golang/mock/gomock"
	reflect "reflect"
)

// MockResource is a mock of Resource interface
type MockResource struct {
	ctrl     *gomock.Controller
	recorder *MockResourceMockRecorder
}

// MockResourceMockRecorder is the mock recorder for MockResource
type MockResourceMockRecorder struct {
	mock *MockResource
}

// NewMockResource creates a new mock instance
func NewMockResource(ctrl *gomock.Controller) *MockResource {
	mock := &MockResource{ctrl: ctrl}
	mock.recorder = &MockResourceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockResource) EXPECT() *MockResourceMockRecorder {
	return m.recorder
}

// Delete mocks base method
func (m *MockResource) Delete(arg0 context.Context, arg1 interface{}) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Delete", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// Delete indicates an expected call of Delete
func (mr *MockResourceMockRecorder) Delete(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Delete", reflect.TypeOf((*MockResource)(nil).Delete), arg0, arg1)
}

// DeleteByID mocks base method
func (m *MockResource) DeleteByID(arg0 context.Context, arg1 interface{}, arg2 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteByID", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteByID indicates an expected call of DeleteByID
func (mr *MockResourceMockRecorder) DeleteByID(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteByID", reflect.TypeOf((*MockResource)(nil).DeleteByID), arg0, arg1, arg2)
}

// FindAll mocks base method
func (m *MockResource) FindAll(arg0 context.Context, arg1 interface{}) frameless.Iterator {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FindAll", arg0, arg1)
	ret0, _ := ret[0].(frameless.Iterator)
	return ret0
}

// FindAll indicates an expected call of FindAll
func (mr *MockResourceMockRecorder) FindAll(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindAll", reflect.TypeOf((*MockResource)(nil).FindAll), arg0, arg1)
}

// FindByID mocks base method
func (m *MockResource) FindByID(arg0 context.Context, arg1 interface{}, arg2 string) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FindByID", arg0, arg1, arg2)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// FindByID indicates an expected call of FindByID
func (mr *MockResourceMockRecorder) FindByID(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindByID", reflect.TypeOf((*MockResource)(nil).FindByID), arg0, arg1, arg2)
}

// Save mocks base method
func (m *MockResource) Save(arg0 context.Context, arg1 interface{}) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Save", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// Save indicates an expected call of Save
func (mr *MockResourceMockRecorder) Save(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Save", reflect.TypeOf((*MockResource)(nil).Save), arg0, arg1)
}

// Truncate mocks base method
func (m *MockResource) Truncate(arg0 context.Context, arg1 interface{}) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Truncate", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// Truncate indicates an expected call of Truncate
func (mr *MockResourceMockRecorder) Truncate(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Truncate", reflect.TypeOf((*MockResource)(nil).Truncate), arg0, arg1)
}

// Update mocks base method
func (m *MockResource) Update(arg0 context.Context, arg1 interface{}) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Update", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// Update indicates an expected call of Update
func (mr *MockResourceMockRecorder) Update(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Update", reflect.TypeOf((*MockResource)(nil).Update), arg0, arg1)
}
