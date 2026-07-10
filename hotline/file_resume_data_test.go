package hotline

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// UnmarshalBinary is covered in decode_test.go. This file covers the constructors, the marshal
// path, encode↔decode symmetry, and ForkType.String().

func TestForkType_String(t *testing.T) {
	assert.Equal(t, "DATA", ForkTypeDATA.String())
	assert.Equal(t, "INFO", ForkTypeINFO.String())
	assert.Equal(t, "MACR", ForkTypeMACR.String())
}

func TestNewForkInfoList(t *testing.T) {
	fil := NewForkInfoList([]byte{0x00, 0x00, 0x10, 0x00})
	assert.Equal(t, ForkTypeDATA, fil.Fork)
	assert.Equal(t, [4]byte{0x00, 0x00, 0x10, 0x00}, fil.DataSize)
	assert.Equal(t, uint32(0x1000), binary.BigEndian.Uint32(fil.DataSize[:]))
}

func TestNewFileResumeData(t *testing.T) {
	frd := NewFileResumeData([]ForkInfoList{*NewForkInfoList([]byte{0, 0, 0, 5})})

	assert.Equal(t, FormatRFLT, frd.Format)
	assert.Equal(t, [2]byte{0, 1}, frd.Version)
	assert.Equal(t, [2]byte{0, 1}, frd.ForkCount, "ForkCount low byte tracks the list length")
	require.Len(t, frd.ForkInfoList, 1)
}

func TestFileResumeData_BinaryMarshal(t *testing.T) {
	frd := NewFileResumeData([]ForkInfoList{
		*NewForkInfoList([]byte{0, 0, 0, 5}),
		*NewForkInfoList([]byte{0, 0, 0, 9}),
	})

	b, err := frd.BinaryMarshal()
	require.NoError(t, err)
	assert.Len(t, b, resumeDataHeaderLen+2*forkInfoLen)
	assert.Equal(t, FormatRFLT[:], b[0:4])
	assert.Equal(t, byte(2), b[41], "ForkCount low byte")
}

func TestFileResumeData_RoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		forks []ForkInfoList
	}{
		{
			name:  "two forks",
			forks: []ForkInfoList{*NewForkInfoList([]byte{0, 0, 0, 5}), *NewForkInfoList([]byte{0, 0, 1, 0})},
		},
		{
			name: "three forks",
			forks: []ForkInfoList{
				*NewForkInfoList([]byte{0, 0, 0, 5}),
				*NewForkInfoList([]byte{0, 0, 1, 0}),
				*NewForkInfoList([]byte{0, 0, 0, 1}),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := NewFileResumeData(tt.forks)

			b, err := original.BinaryMarshal()
			require.NoError(t, err)

			var decoded FileResumeData
			require.NoError(t, decoded.UnmarshalBinary(b))

			// Format/Version/ForkCount and every fork survive the round trip. BinaryMarshal writes
			// with LittleEndian and UnmarshalBinary reads with BigEndian, but all fields are
			// [n]byte arrays, so the encoding is endian-neutral — this test pins that.
			assert.Equal(t, original.Format, decoded.Format)
			assert.Equal(t, original.Version, decoded.Version)
			assert.Equal(t, original.ForkCount, decoded.ForkCount)
			assert.Equal(t, original.ForkInfoList, decoded.ForkInfoList)
		})
	}
}
