// Code generated by mockery v1.0.0. DO NOT EDIT.

package mocks

import (
	context "context"

	chrome_branch "go.skia.org/infra/go/chrome_branch"

	mock "github.com/stretchr/testify/mock"
)

// Client is an autogenerated mock type for the Client type
type Client struct {
	mock.Mock
}

// Get provides a mock function with given fields: _a0
func (_m *Client) Get(_a0 context.Context) (*chrome_branch.Branches, error) {
	ret := _m.Called(_a0)

	var r0 *chrome_branch.Branches
	if rf, ok := ret.Get(0).(func(context.Context) *chrome_branch.Branches); ok {
		r0 = rf(_a0)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*chrome_branch.Branches)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(_a0)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}