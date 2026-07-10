package mobius

import (
	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/mock"
)

// MockAccountManager provides a test double implementation of AccountManager using testify/mock.
type MockAccountManager struct {
	mock.Mock
}

var _ hotline.AccountManager = (*MockAccountManager)(nil)

// Create mocks the Create method for testing.
func (m *MockAccountManager) Create(account hotline.Account) error {
	args := m.Called(account)

	return args.Error(0)
}

// Update mocks the Update method for testing.
func (m *MockAccountManager) Update(account hotline.Account, newLogin string) error {
	args := m.Called(account, newLogin)

	return args.Error(0)
}

// Get mocks the Get method for testing.
func (m *MockAccountManager) Get(login string) *hotline.Account {
	args := m.Called(login)

	return args.Get(0).(*hotline.Account)
}

// List mocks the List method for testing.
func (m *MockAccountManager) List() []hotline.Account {
	args := m.Called()

	return args.Get(0).([]hotline.Account)
}

// Delete mocks the Delete method for testing.
func (m *MockAccountManager) Delete(login string) error {
	args := m.Called(login)

	return args.Error(0)
}
