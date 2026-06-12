package mobius

import (
	"cmp"
	"encoding/binary"
	"io"
	"log/slog"
	"os"
	"slices"
	"testing"

	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

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

type nopReadWriteCloser struct{}

func (nopReadWriteCloser) Read([]byte) (int, error) { return 0, io.EOF }

func (nopReadWriteCloser) Write(p []byte) (int, error) { return len(p), nil }

func (nopReadWriteCloser) Close() error { return nil }

func NewTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, nil))
}

var tranSortFunc = func(a, b hotline.Transaction) int {
	return cmp.Compare(
		binary.BigEndian.Uint16(a.ClientID[:]),
		binary.BigEndian.Uint16(b.ClientID[:]),
	)
}

// TranAssertEqual compares equality of transactions slices after stripping out the random transaction Type
func TranAssertEqual(t *testing.T, tran1, tran2 []hotline.Transaction) bool {
	var newT1 []hotline.Transaction
	var newT2 []hotline.Transaction

	for _, trans := range tran1 {
		trans.ID = [4]byte{0, 0, 0, 0}
		var fs []hotline.Field
		for _, field := range trans.Fields {
			if field.Type == hotline.FieldRefNum { // FieldRefNum
				continue
			}
			if field.Type == hotline.FieldChatID { // FieldChatID
				continue
			}

			fs = append(fs, field)
		}
		trans.Fields = fs
		newT1 = append(newT1, trans)
	}

	for _, trans := range tran2 {
		trans.ID = [4]byte{0, 0, 0, 0}
		var fs []hotline.Field
		for _, field := range trans.Fields {
			if field.Type == hotline.FieldRefNum { // FieldRefNum
				continue
			}
			if field.Type == hotline.FieldChatID { // FieldChatID
				continue
			}

			fs = append(fs, field)
		}
		trans.Fields = fs
		newT2 = append(newT2, trans)
	}

	slices.SortFunc(newT1, tranSortFunc)
	slices.SortFunc(newT2, tranSortFunc)

	return assert.Equal(t, newT1, newT2)
}
