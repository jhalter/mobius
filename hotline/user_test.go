package hotline

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestReadUser(t *testing.T) {
	type args struct {
		b []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *User
		wantErr bool
	}{
		{
			name: "returns expected User struct",
			args: args{
				b: []byte{
					0x00, 0x01,
					0x07, 0xd0,
					0x00, 0x01,
					0x00, 0x03,
					0x61, 0x61, 0x61,
				},
			},
			want: &User{
				ID: []byte{
					0x00, 0x01,
				},
				Icon: []byte{
					0x07, 0xd0,
				},
				Flags: []byte{
					0x00, 0x01,
				},
				Name: "aaa",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadUser(tt.args.b)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadUser() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !assert.Equal(t, tt.want, got) {
				t.Errorf("ReadUser() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecodeUserString(t *testing.T) {
	type args struct {
		encodedString []byte
	}
	tests := []struct {
		name              string
		args              args
		wantDecodedString string
	}{
		{
			name: "decodes bytes to guest",
			args: args{
				encodedString: []byte{
					0x98, 0x8a, 0x9a, 0x8c, 0x8b,
				},
			},
			wantDecodedString: "guest",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotDecodedString := decodeString(tt.args.encodedString); gotDecodedString != tt.wantDecodedString {
				t.Errorf("decodeString() = %v, want %v", gotDecodedString, tt.wantDecodedString)
			}
		})
	}
}

func TestNegatedUserString(t *testing.T) {
	type args struct {
		encodedString []byte
	}
	tests := []struct {
		name string
		args args
		want []byte
	}{
		{
			name: "encodes bytes to expected string",
			args: args{
				encodedString: []byte("guest"),
			},
			want: []byte{0x98, 0x8a, 0x9a, 0x8c, 0x8b},
		},
		{
			name: "encodes bytes with numerals to expected string",
			args: args{
				encodedString: []byte("foo1"),
			},
			want: []byte{0x99, 0x90, 0x90, 0xce},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := encodeString(tt.args.encodedString); !bytes.Equal(got, tt.want) {
				t.Errorf("NegatedUserString() = %x, want %x", got, tt.want)
			}
		})
	}
}
