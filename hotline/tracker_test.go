package hotline

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"reflect"
	"testing"
)

func TestTrackerRegistration_Payload(t *testing.T) {
	type fields struct {
		Port        [2]byte
		UserCount   int
		PassID      [4]byte
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
				PassID:      [4]byte{0x00, 0x00, 0x00, 0x01},
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

			if got, _ := io.ReadAll(tr); !reflect.DeepEqual(got, tt.want) {
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
		{
			name: "when nameLen exceeds provided data",
			args: args{
				data: []byte{
					0x18, 0x05, 0x30, 0x63, // IP Addr
					0x15, 0x7c, // Port
					0x00, 0x02, // UserCount
					0x00, 0x00, // ??
					0xff,             // Name Len
					0x54, 0x68, 0x65, // Name
					0x03,             // Desc Len
					0x54, 0x54, 0x54, // Description
				},
				atEOF: false,
			},
			wantAdvance: 0,
			wantToken:   []byte(nil),
			wantErr:     assert.NoError,
		},
		{
			name: "when description len exceeds provided data",
			args: args{
				data: []byte{
					0x18, 0x05, 0x30, 0x63, // IP Addr
					0x15, 0x7c, // Port
					0x00, 0x02, // UserCount
					0x00, 0x00, // ??
					0x03,             // Name Len
					0x54, 0x68, 0x65, // Name
					0xff,             // Desc Len
					0x54, 0x54, 0x54, // Description
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

type mockConn struct {
	readBuffer  *bytes.Buffer
	writeBuffer *bytes.Buffer
	closed      bool
}

func (m *mockConn) Read(b []byte) (n int, err error) {
	return m.readBuffer.Read(b)
}

func (m *mockConn) Write(b []byte) (n int, err error) {
	return m.writeBuffer.Write(b)
}

func (m *mockConn) Close() error {
	m.closed = true
	return nil
}

func TestGetListing(t *testing.T) {
	tests := []struct {
		name       string
		mockConn   *mockConn
		wantErr    bool
		wantResult []ServerRecord
	}{
		{
			name: "Successful retrieval",
			mockConn: &mockConn{
				readBuffer: bytes.NewBuffer([]byte{
					// TrackerHeader
					0x48, 0x54, 0x52, 0x4B, // Protocol "HTRK"
					0x00, 0x01, // Version 1
					// ServerInfoHeader
					0x00, 0x01, // MsgType (1)
					0x00, 0x14, // MsgDataSize (20)
					0x00, 0x02, // SrvCount (2)
					0x00, 0x02, // SrvCountDup (2)
					// ServerRecord 1
					192, 168, 1, 1, // IP address
					0x1F, 0x90, // Port 8080
					0x00, 0x10, // NumUsers 16
					0x00, 0x00, // Unused
					0x04,               // NameSize
					'S', 'e', 'r', 'v', // Name
					0x0B,                                                  // DescriptionSize
					'M', 'y', ' ', 'S', 'e', 'r', 'v', 'e', 'r', ' ', '1', // Description
					// ServerRecord 2
					10, 0, 0, 1, // IP address
					0x1F, 0x91, // Port 8081
					0x00, 0x05, // NumUsers 5
					0x00, 0x00, // Unused
					0x04,               // NameSize
					'S', 'e', 'r', 'v', // Name
					0x0B,                                                  // DescriptionSize
					'M', 'y', ' ', 'S', 'e', 'r', 'v', 'e', 'r', ' ', '2', // Description
				}),
				writeBuffer: &bytes.Buffer{},
			},
			wantErr: false,
			wantResult: []ServerRecord{
				{
					IPAddr:          [4]byte{192, 168, 1, 1},
					Port:            [2]byte{0x1F, 0x90},
					NumUsers:        [2]byte{0x00, 0x10},
					Unused:          [2]byte{0x00, 0x00},
					NameSize:        4,
					Name:            []byte("Serv"),
					DescriptionSize: 11,
					Description:     []byte("My Server 1"),
				},
				{
					IPAddr:          [4]byte{10, 0, 0, 1},
					Port:            [2]byte{0x1F, 0x91},
					NumUsers:        [2]byte{0x00, 0x05},
					Unused:          [2]byte{0x00, 0x00},
					NameSize:        4,
					Name:            []byte("Serv"),
					DescriptionSize: 11,
					Description:     []byte("My Server 2"),
				},
			},
		},
		{
			name: "Write error",
			mockConn: &mockConn{
				readBuffer:  &bytes.Buffer{},
				writeBuffer: &bytes.Buffer{},
			},
			wantErr:    true,
			wantResult: nil,
		},
		{
			name: "Read error on TrackerHeader",
			mockConn: &mockConn{
				readBuffer: bytes.NewBuffer([]byte{
					// incomplete data to cause read error
					0x48,
				}),
				writeBuffer: &bytes.Buffer{},
			},
			wantErr:    true,
			wantResult: nil,
		},
		{
			name: "Read error on ServerInfoHeader",
			mockConn: &mockConn{
				readBuffer: bytes.NewBuffer([]byte{
					// TrackerHeader
					0x48, 0x54, 0x52, 0x4B, // Protocol "HTRK"
					0x00, 0x01, // Version 1
					// incomplete ServerInfoHeader
					0x00,
				}),
				writeBuffer: &bytes.Buffer{},
			},
			wantErr:    true,
			wantResult: nil,
		},
		{
			name: "Scanner error",
			mockConn: &mockConn{
				readBuffer: bytes.NewBuffer([]byte{
					// TrackerHeader
					0x48, 0x54, 0x52, 0x4B, // Protocol "HTRK"
					0x00, 0x01, // Version 1
					// ServerInfoHeader
					0x00, 0x01, // MsgType (1)
					0x00, 0x14, // MsgDataSize (20)
					0x00, 0x01, // SrvCount (1)
					0x00, 0x01, // SrvCountDup (1)
					// incomplete ServerRecord to cause scanner error
					192, 168, 1, 1,
				}),
				writeBuffer: &bytes.Buffer{},
			},
			wantErr:    true,
			wantResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetListing(tt.mockConn)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantResult, got)
			}
		})
	}
}
