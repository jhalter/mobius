package hotline

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlatFileInformationFork_UnmarshalBinary(t *testing.T) {
	type args struct {
		b []byte
	}
	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "when zero length comment size is omitted (Nostalgia client behavior)",
			args: args{
				b: []byte{
					0x41, 0x4d, 0x41, 0x43, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x09, 0x62, 0x65, 0x61, 0x72, 0x2e, 0x74, 0x69, 0x66, 0x66,
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when zero length comment size is included",
			args: args{
				b: []byte{
					0x41, 0x4d, 0x41, 0x43, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x09, 0x62, 0x65, 0x61, 0x72, 0x2e, 0x74, 0x69, 0x66, 0x66, 0x00, 0x00,
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ffif := &FlatFileInformationFork{}
			tt.wantErr(t, ffif.UnmarshalBinary(tt.args.b), fmt.Sprintf("Write(%v)", tt.args.b))
		})
	}
}

func TestFlatFileInformationFork_FriendlyType(t *testing.T) {
	tests := []struct {
		name          string
		typeSignature [4]byte
		want          []byte
	}{
		{
			name:          "known type TEXT",
			typeSignature: [4]byte{'T', 'E', 'X', 'T'},
			want:          []byte("Text File"),
		},
		{
			name:          "known type APPL",
			typeSignature: [4]byte{'A', 'P', 'P', 'L'},
			want:          []byte("Application Program"),
		},
		{
			name:          "known type SIT!",
			typeSignature: [4]byte{'S', 'I', 'T', '!'},
			want:          []byte("StuffIt Archive"),
		},
		{
			name:          "unknown type returns raw signature",
			typeSignature: [4]byte{'J', 'P', 'E', 'G'},
			want:          []byte("JPEG"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ffif := &FlatFileInformationFork{
				TypeSignature: tt.typeSignature,
			}
			assert.Equal(t, tt.want, ffif.FriendlyType())
		})
	}
}

func TestFlatFileInformationFork_FriendlyCreator(t *testing.T) {
	tests := []struct {
		name             string
		creatorSignature [4]byte
		want             []byte
	}{
		{
			name:             "known creator HTLC",
			creatorSignature: [4]byte{'H', 'T', 'L', 'C'},
			want:             []byte("Hotline"),
		},
		{
			name:             "unknown creator returns raw signature",
			creatorSignature: [4]byte{'o', 'g', 'l', 'e'},
			want:             []byte("ogle"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ffif := &FlatFileInformationFork{
				CreatorSignature: tt.creatorSignature,
			}
			assert.Equal(t, tt.want, ffif.FriendlyCreator())
		})
	}
}

func TestFlatFileInformationFork_SetComment(t *testing.T) {
	tests := []struct {
		name            string
		comment         []byte
		wantComment     []byte
		wantCommentSize [2]byte
	}{
		{
			name:            "sets a short comment",
			comment:         []byte("hello"),
			wantComment:     []byte("hello"),
			wantCommentSize: [2]byte{0x00, 0x05},
		},
		{
			name:            "sets an empty comment",
			comment:         []byte{},
			wantComment:     []byte{},
			wantCommentSize: [2]byte{0x00, 0x00},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ffif := &FlatFileInformationFork{}
			err := ffif.SetComment(tt.comment)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantComment, ffif.Comment)
			assert.Equal(t, tt.wantCommentSize, ffif.CommentSize)
		})
	}
}

func TestFlattenedFileObject_TransferSize(t *testing.T) {
	tests := []struct {
		name        string
		dataSize    uint32
		resForkSize uint32
		offset      int64
		wantNonZero bool
	}{
		{
			name:        "calculates transfer size with zero offset",
			dataSize:    100,
			resForkSize: 50,
			offset:      0,
			wantNonZero: true,
		},
		{
			name:        "calculates transfer size with non-zero offset",
			dataSize:    200,
			resForkSize: 100,
			offset:      50,
			wantNonZero: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ffo := &flattenedFileObject{}

			binary.BigEndian.PutUint32(ffo.FlatFileDataForkHeader.DataSize[:], tt.dataSize)
			binary.BigEndian.PutUint32(ffo.FlatFileResForkHeader.DataSize[:], tt.resForkSize)

			result := ffo.TransferSize(tt.offset)
			assert.Len(t, result, 4)

			size := binary.BigEndian.Uint32(result)
			assert.Greater(t, size, uint32(0))

			// With offset, the size should be smaller than without offset.
			if tt.offset > 0 {
				noOffsetResult := ffo.TransferSize(0)
				noOffsetSize := binary.BigEndian.Uint32(noOffsetResult)
				assert.Equal(t, noOffsetSize-uint32(tt.offset), size)
			}
		})
	}
}
