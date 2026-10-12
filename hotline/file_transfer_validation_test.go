package hotline

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileTransferProgressWithUnknownOrShortSize(t *testing.T) {
	for _, tc := range []struct {
		name                string
		size                []byte
		progress, formatted string
	}{
		{"absent", nil, "?", "?"},
		{"empty", []byte{}, "?", "?"},
		{"one byte", []byte{1}, "?", "?"},
		{"three bytes", []byte{0, 0, 1}, "?", "?"},
		{"zero", []byte{0, 0, 0, 0}, "?", "0K"},
		{"two bytes", []byte{4, 0}, "50", "1K"},
		{"four bytes", []byte{0, 0, 4, 0}, "50", "1K"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ft := &FileTransfer{TransferSize: tc.size, bytesSentCounter: &WriteCounter{Total: 512}}
			assert.Equal(t, tc.progress, ft.percentComplete())
			assert.Equal(t, tc.formatted, ft.formattedTransferSize())
		})
	}
}

func TestFileTransferProgressLargeSizes(t *testing.T) {
	for _, tc := range []struct {
		name                string
		size                []byte
		total               int64
		progress, formatted string
	}{
		{"2 GiB", []byte{0x80, 0, 0, 0}, 1 << 30, "50", "2048.0M"},
		{"3 GiB", []byte{0xc0, 0, 0, 0}, 3 << 29, "50", "3072.0M"},
		{"maximum uint32", []byte{0xff, 0xff, 0xff, 0xff}, 1<<32 - 1, "100", "4096.0M"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cc := &ClientConn{
				Server:                &Server{FileTransferMgr: NewMemFileTransferMgr()},
				ClientFileTransferMgr: NewClientFileTransferMgr(),
			}
			ft := cc.NewFileTransfer(FileUpload, "", []byte("large"), nil, tc.size)
			ft.bytesSentCounter.Total = tc.total
			assert.Equal(t, tc.size, ft.TransferSize)
			assert.Equal(t, tc.progress, ft.percentComplete())
			assert.Equal(t, tc.formatted, ft.formattedTransferSize())
		})
	}
}
