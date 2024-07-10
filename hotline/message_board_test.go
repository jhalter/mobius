package hotline

import "github.com/stretchr/testify/mock"

type mockReadWriteSeeker struct {
	mock.Mock
}

func (m *mockReadWriteSeeker) Read(p []byte) (int, error) {
	args := m.Called(p)

	return args.Int(0), args.Error(1)
}

func (m *mockReadWriteSeeker) Write(p []byte) (int, error) {
	args := m.Called(p)

	return args.Int(0), args.Error(1)
}

func (m *mockReadWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	args := m.Called(offset, whence)

	return args.Get(0).(int64), args.Error(1)
}
