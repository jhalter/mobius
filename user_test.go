package hotline

import (
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
			if gotDecodedString := DecodeUserString(tt.args.encodedString); gotDecodedString != tt.wantDecodedString {
				t.Errorf("DecodeUserString() = %v, want %v", gotDecodedString, tt.wantDecodedString)
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
		want string
	}{
		{
			name: "encodes bytes to string",
			args: args{
				encodedString: []byte("guest"),
			},
			want: string([]byte{0x98, 0x8a, 0x9a, 0x8c, 0x8b}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NegatedUserString(tt.args.encodedString); got != tt.want {
				t.Errorf("NegatedUserString() = %v, want %v", got, tt.want)
			}
		})
	}
}
