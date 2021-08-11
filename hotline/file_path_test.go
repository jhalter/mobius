package hotline

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFilePath_UnmarshalBinary(t *testing.T) {
	type fields struct {
		ItemCount []byte
		Items     []FilePathItem
	}
	type args struct {
		b []byte
	}
	tests := []struct {
		name    string
		args    args
		want    FilePath
		wantErr bool
	}{
		{
			name: "unmarshals bytes into struct",
			args: args{b: []byte{
				0x00, 0x02,
				0x00, 0x00,
				0x0f,
				0x46, 0x69, 0x72, 0x73, 0x74, 0x20, 0x4c, 0x65, 0x76, 0x65, 0x6c, 0x20, 0x44, 0x69, 0x72,
				0x00, 0x00,
				0x08,
				0x41, 0x20, 0x53, 0x75, 0x62, 0x44, 0x69, 0x72,
			}},
			want: FilePath{
				ItemCount: []byte{0x00, 0x02},
				Items: []FilePathItem{
					{
						Len:  0x0f,
						Name: []byte("First Level Dir"),
					},
					{
						Len:  0x08,
						Name: []byte("A SubDir"),
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fp FilePath
			if err := fp.UnmarshalBinary(tt.args.b); (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !assert.Equal(t, tt.want, fp) {
				t.Errorf("Read() got = %v, want %v", fp, tt.want)
			}
		})
	}
}
