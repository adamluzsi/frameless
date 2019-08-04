// Code generated by MockGen. DO NOT EDIT.
// Source: SQLRows.go

// Package iterators_test is a generated GoMock package.
package iterators_test

import (
	frameless "github.com/adamluzsi/frameless"
	iterators "github.com/adamluzsi/frameless/iterators"
	gomock "github.com/golang/mock/gomock"
	reflect "reflect"
)

// MockSQLRowScanner is a mock of SQLRowScanner interface
type MockSQLRowScanner struct {
	ctrl     *gomock.Controller
	recorder *MockSQLRowScannerMockRecorder
}

// MockSQLRowScannerMockRecorder is the mock recorder for MockSQLRowScanner
type MockSQLRowScannerMockRecorder struct {
	mock *MockSQLRowScanner
}

// NewMockSQLRowScanner creates a new mock instance
func NewMockSQLRowScanner(ctrl *gomock.Controller) *MockSQLRowScanner {
	mock := &MockSQLRowScanner{ctrl: ctrl}
	mock.recorder = &MockSQLRowScannerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockSQLRowScanner) EXPECT() *MockSQLRowScannerMockRecorder {
	return m.recorder
}

// Scan mocks base method
func (m *MockSQLRowScanner) Scan(arg0 ...interface{}) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{}
	for _, a := range arg0 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Scan", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// Scan indicates an expected call of Scan
func (mr *MockSQLRowScannerMockRecorder) Scan(arg0 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Scan", reflect.TypeOf((*MockSQLRowScanner)(nil).Scan), arg0...)
}

// MockSQLRowMapper is a mock of SQLRowMapper interface
type MockSQLRowMapper struct {
	ctrl     *gomock.Controller
	recorder *MockSQLRowMapperMockRecorder
}

// MockSQLRowMapperMockRecorder is the mock recorder for MockSQLRowMapper
type MockSQLRowMapperMockRecorder struct {
	mock *MockSQLRowMapper
}

// NewMockSQLRowMapper creates a new mock instance
func NewMockSQLRowMapper(ctrl *gomock.Controller) *MockSQLRowMapper {
	mock := &MockSQLRowMapper{ctrl: ctrl}
	mock.recorder = &MockSQLRowMapperMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockSQLRowMapper) EXPECT() *MockSQLRowMapperMockRecorder {
	return m.recorder
}

// Map mocks base method
func (m *MockSQLRowMapper) Map(s iterators.SQLRowScanner, e frameless.Entity) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Map", s, e)
	ret0, _ := ret[0].(error)
	return ret0
}

// Map indicates an expected call of Map
func (mr *MockSQLRowMapperMockRecorder) Map(s, e interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Map", reflect.TypeOf((*MockSQLRowMapper)(nil).Map), s, e)
}

// MockSQLRows is a mock of SQLRows interface
type MockSQLRows struct {
	ctrl     *gomock.Controller
	recorder *MockSQLRowsMockRecorder
}

// MockSQLRowsMockRecorder is the mock recorder for MockSQLRows
type MockSQLRowsMockRecorder struct {
	mock *MockSQLRows
}

// NewMockSQLRows creates a new mock instance
func NewMockSQLRows(ctrl *gomock.Controller) *MockSQLRows {
	mock := &MockSQLRows{ctrl: ctrl}
	mock.recorder = &MockSQLRowsMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockSQLRows) EXPECT() *MockSQLRowsMockRecorder {
	return m.recorder
}

// Close mocks base method
func (m *MockSQLRows) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close
func (mr *MockSQLRowsMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockSQLRows)(nil).Close))
}

// Next mocks base method
func (m *MockSQLRows) Next() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Next")
	ret0, _ := ret[0].(bool)
	return ret0
}

// Next indicates an expected call of Next
func (mr *MockSQLRowsMockRecorder) Next() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Next", reflect.TypeOf((*MockSQLRows)(nil).Next))
}

// Err mocks base method
func (m *MockSQLRows) Err() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Err")
	ret0, _ := ret[0].(error)
	return ret0
}

// Err indicates an expected call of Err
func (mr *MockSQLRowsMockRecorder) Err() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Err", reflect.TypeOf((*MockSQLRows)(nil).Err))
}

// Scan mocks base method
func (m *MockSQLRows) Scan(dest ...interface{}) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{}
	for _, a := range dest {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Scan", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// Scan indicates an expected call of Scan
func (mr *MockSQLRowsMockRecorder) Scan(dest ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Scan", reflect.TypeOf((*MockSQLRows)(nil).Scan), dest...)
}
