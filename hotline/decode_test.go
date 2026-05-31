package hotline

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientIDFromBytes(t *testing.T) {
	tests := []struct {
		name   string
		b      []byte
		want   ClientID
		wantOk bool
	}{
		{name: "valid 2 bytes", b: []byte{0x00, 0x05}, want: ClientID{0x00, 0x05}, wantOk: true},
		{name: "nil", b: nil, wantOk: false},
		{name: "too short", b: []byte{0x01}, wantOk: false},
		{name: "too long", b: []byte{0x01, 0x02, 0x03}, wantOk: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ClientIDFromBytes(tt.b)
			assert.Equal(t, tt.wantOk, ok)
			if tt.wantOk {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestChatIDFromBytes(t *testing.T) {
	tests := []struct {
		name   string
		b      []byte
		want   ChatID
		wantOk bool
	}{
		{name: "valid 4 bytes", b: []byte{0x00, 0x00, 0x00, 0x09}, want: ChatID{0x00, 0x00, 0x00, 0x09}, wantOk: true},
		{name: "nil", b: nil, wantOk: false},
		{name: "too short", b: []byte{0x01, 0x02, 0x03}, wantOk: false},
		{name: "too long", b: []byte{0x01, 0x02, 0x03, 0x04, 0x05}, wantOk: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ChatIDFromBytes(tt.b)
			assert.Equal(t, tt.wantOk, ok)
			if tt.wantOk {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestFileResumeData_UnmarshalBinary(t *testing.T) {
	// A well-formed buffer: 42-byte header with ForkCount=1, plus one 16-byte fork.
	valid := make([]byte, resumeDataHeaderLen+forkInfoLen)
	valid[41] = 1 // ForkCount low byte

	t.Run("valid single-fork buffer parses", func(t *testing.T) {
		var frd FileResumeData
		require.NoError(t, frd.UnmarshalBinary(valid))
		assert.Len(t, frd.ForkInfoList, 1)
	})

	t.Run("buffer shorter than header returns error, no panic", func(t *testing.T) {
		var frd FileResumeData
		assert.Error(t, frd.UnmarshalBinary(make([]byte, 10)))
	})

	t.Run("fork count overrunning the buffer returns error, no panic", func(t *testing.T) {
		b := make([]byte, resumeDataHeaderLen) // header only, no fork bytes
		b[41] = 3                              // claims 3 forks that aren't present
		var frd FileResumeData
		assert.Error(t, frd.UnmarshalBinary(b))
	})

	t.Run("zero fork count yields an empty list", func(t *testing.T) {
		b := make([]byte, resumeDataHeaderLen)
		var frd FileResumeData
		require.NoError(t, frd.UnmarshalBinary(b))
		assert.Empty(t, frd.ForkInfoList)
	})
}
