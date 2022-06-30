package hotline

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

func TestTrackerRegistration_Payload(t *testing.T) {
	type fields struct {
		Port        [2]byte
		UserCount   int
		PassID      []byte
		Name        string
		Description string
	}
	tests := []struct {
		name   string
		fields fields
		want   []byte
	}{
		{
			name: "returns expected payload bytes",
			fields: fields{
				Port:        [2]byte{0x00, 0x10},
				UserCount:   2,
				PassID:      []byte{0x00, 0x00, 0x00, 0x01},
				Name:        "Test Serv",
				Description: "Fooz",
			},
			want: []byte{
				0x00, 0x01,
				0x00, 0x10,
				0x00, 0x02,
				0x00, 0x00,
				0x00, 0x00, 0x00, 0x01,
				0x09,
				0x54, 0x65, 0x73, 0x74, 0x20, 0x53, 0x65, 0x72, 0x76,
				0x04,
				0x46, 0x6f, 0x6f, 0x7a,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &TrackerRegistration{
				Port:        tt.fields.Port,
				UserCount:   tt.fields.UserCount,
				PassID:      tt.fields.PassID,
				Name:        tt.fields.Name,
				Description: tt.fields.Description,
			}
			if got := tr.Read(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Read() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_serverScanner(t *testing.T) {
	type args struct {
		data  []byte
		atEOF bool
	}
	tests := []struct {
		name        string
		args        args
		wantAdvance int
		wantToken   []byte
		wantErr     assert.ErrorAssertionFunc
	}{
		{
			name: "when a full server entry is provided",
			args: args{
				data: []byte{
					0x18, 0x05, 0x30, 0x63, // IP Addr
					0x15, 0x7c, // Port
					0x00, 0x02, // UserCount
					0x00, 0x00, // ??
					0x03,             // Name Len
					0x54, 0x68, 0x65, // Name
					0x03,             // Desc Len
					0x54, 0x54, 0x54, // Description
				},
				atEOF: false,
			},
			wantAdvance: 18,
			wantToken: []byte{
				0x18, 0x05, 0x30, 0x63, // IP Addr
				0x15, 0x7c, // Port
				0x00, 0x02, // UserCount
				0x00, 0x00, // ??
				0x03,             // Name Len
				0x54, 0x68, 0x65, // Name
				0x03,             // Desc Len
				0x54, 0x54, 0x54, // Description
			},
			wantErr: assert.NoError,
		},
		{
			name: "when extra bytes are provided",
			args: args{
				data: []byte{
					0x18, 0x05, 0x30, 0x63, // IP Addr
					0x15, 0x7c, // Port
					0x00, 0x02, // UserCount
					0x00, 0x00, // ??
					0x03,             // Name Len
					0x54, 0x68, 0x65, // Name
					0x03,             // Desc Len
					0x54, 0x54, 0x54, // Description
					0x54, 0x54, 0x54, 0x54, 0x54, 0x54, 0x54, 0x54, 0x54,
				},
				atEOF: false,
			},
			wantAdvance: 18,
			wantToken: []byte{
				0x18, 0x05, 0x30, 0x63, // IP Addr
				0x15, 0x7c, // Port
				0x00, 0x02, // UserCount
				0x00, 0x00, // ??
				0x03,             // Name Len
				0x54, 0x68, 0x65, // Name
				0x03,             // Desc Len
				0x54, 0x54, 0x54, // Description
			},
			wantErr: assert.NoError,
		},
		{
			name: "when insufficient bytes are provided",
			args: args{
				data: []byte{
					0, 0,
				},
				atEOF: false,
			},
			wantAdvance: 0,
			wantToken:   []byte(nil),
			wantErr:     assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAdvance, gotToken, err := serverScanner(tt.args.data, tt.args.atEOF)
			if !tt.wantErr(t, err, fmt.Sprintf("serverScanner(%v, %v)", tt.args.data, tt.args.atEOF)) {
				return
			}
			assert.Equalf(t, tt.wantAdvance, gotAdvance, "serverScanner(%v, %v)", tt.args.data, tt.args.atEOF)
			assert.Equalf(t, tt.wantToken, gotToken, "serverScanner(%v, %v)", tt.args.data, tt.args.atEOF)
		})
	}
}
