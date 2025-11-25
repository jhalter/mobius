package hotline

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				0x00,
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

			if got, _ := io.ReadAll(tr); !assert.Equal(t, tt.want, got) {
				t.Errorf("Read() = %v, want %v", got, tt.want)
			}
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
					0x00, 0x02, // BatchSize (2)
					// ServerRecord 1
					192, 168, 1, 1, // IP address
					0x1F, 0x90, // Port 8080
					0x00, 0x10, // NumUsers 16
					0x00, 0x00, // TLSPort
					0x04,               // NameSize
					'S', 'e', 'r', 'v', // Name
					0x0B,                                                  // DescriptionSize
					'M', 'y', ' ', 'S', 'e', 'r', 'v', 'e', 'r', ' ', '1', // Description
					// ServerRecord 2
					10, 0, 0, 1, // IP address
					0x1F, 0x91, // Port 8081
					0x00, 0x05, // NumUsers 5
					0x00, 0x00, // TLSPort
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
					TLSPort:         [2]byte{0x00, 0x00},
					NameSize:        4,
					Name:            []byte("Serv"),
					DescriptionSize: 11,
					Description:     []byte("My Server 1"),
				},
				{
					IPAddr:          [4]byte{10, 0, 0, 1},
					Port:            [2]byte{0x1F, 0x91},
					NumUsers:        [2]byte{0x00, 0x05},
					TLSPort:         [2]byte{0x00, 0x00},
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
					0x00, 0x01, // BatchSize (1)
					// incomplete ServerRecord to cause scanner error
					192, 168, 1, 1,
				}),
				writeBuffer: &bytes.Buffer{},
			},
			wantErr:    true,
			wantResult: nil,
		},
		{
			name: "Multiple batches with ServerInfoHeaders",
			mockConn: &mockConn{
				readBuffer: bytes.NewBuffer([]byte{
					// TrackerHeader
					0x48, 0x54, 0x52, 0x4B, // Protocol "HTRK"
					0x00, 0x01, // Version 1
					// First ServerInfoHeader
					0x00, 0x01, // MsgType (1)
					0x00, 0x14, // MsgDataSize (20)
					0x00, 0x03, // SrvCount (3 total)
					0x00, 0x02, // BatchSize (2 in first batch)
					// ServerRecord 1
					192, 168, 1, 1, // IP address
					0x1F, 0x90, // Port 8080
					0x00, 0x0A, // NumUsers 10
					0x00, 0x00, // TLSPort
					0x07,                              // NameSize
					'S', 'e', 'r', 'v', 'e', 'r', '1', // Name
					0x0C,                                                       // DescriptionSize
					'F', 'i', 'r', 's', 't', ' ', 'b', 'a', 't', 'c', 'h', '1', // Description
					// ServerRecord 2
					192, 168, 1, 2, // IP address
					0x1F, 0x91, // Port 8081
					0x00, 0x14, // NumUsers 20
					0x00, 0x00, // TLSPort
					0x07,                              // NameSize
					'S', 'e', 'r', 'v', 'e', 'r', '2', // Name
					0x0C,                                                       // DescriptionSize
					'F', 'i', 'r', 's', 't', ' ', 'b', 'a', 't', 'c', 'h', '2', // Description
					// Second ServerInfoHeader (next batch)
					0x00, 0x01, // MsgType (1)
					0x00, 0x0A, // MsgDataSize (10)
					0x00, 0x03, // SrvCount (3 total - same)
					0x00, 0x01, // BatchSize (1 in second batch)
					// ServerRecord 3
					192, 168, 1, 3, // IP address
					0x1F, 0x92, // Port 8082
					0x00, 0x1E, // NumUsers 30
					0x00, 0x00, // TLSPort
					0x07,                              // NameSize
					'S', 'e', 'r', 'v', 'e', 'r', '3', // Name
					0x0D,                                                            // DescriptionSize
					'S', 'e', 'c', 'o', 'n', 'd', ' ', 'b', 'a', 't', 'c', 'h', '1', // Description
				}),
				writeBuffer: &bytes.Buffer{},
			},
			wantErr: false,
			wantResult: []ServerRecord{
				{
					IPAddr:          [4]byte{192, 168, 1, 1},
					Port:            [2]byte{0x1F, 0x90},
					NumUsers:        [2]byte{0x00, 0x0A},
					TLSPort:         [2]byte{0x00, 0x00},
					NameSize:        7,
					Name:            []byte("Server1"),
					DescriptionSize: 12,
					Description:     []byte("First batch1"),
				},
				{
					IPAddr:          [4]byte{192, 168, 1, 2},
					Port:            [2]byte{0x1F, 0x91},
					NumUsers:        [2]byte{0x00, 0x14},
					TLSPort:         [2]byte{0x00, 0x00},
					NameSize:        7,
					Name:            []byte("Server2"),
					DescriptionSize: 12,
					Description:     []byte("First batch2"),
				},
				{
					IPAddr:          [4]byte{192, 168, 1, 3},
					Port:            [2]byte{0x1F, 0x92},
					NumUsers:        [2]byte{0x00, 0x1E},
					TLSPort:         [2]byte{0x00, 0x00},
					NameSize:        7,
					Name:            []byte("Server3"),
					DescriptionSize: 13,
					Description:     []byte("Second batch1"),
				},
			},
		},
		{
			name: "Three batches",
			mockConn: &mockConn{
				readBuffer: bytes.NewBuffer([]byte{
					// TrackerHeader
					0x48, 0x54, 0x52, 0x4B, // Protocol "HTRK"
					0x00, 0x01, // Version 1
					// First ServerInfoHeader
					0x00, 0x01, // MsgType (1)
					0x00, 0x0A, // MsgDataSize
					0x00, 0x04, // SrvCount (4 total)
					0x00, 0x02, // BatchSize (2 in first batch)
					// ServerRecord 1
					192, 168, 1, 1, // IP
					0x15, 0x7c, // Port 5500
					0x00, 0x01, // NumUsers 1
					0x00, 0x00, // TLSPort
					0x01, // NameSize
					'A',  // Name
					0x01, // DescriptionSize
					'1',  // Description
					// ServerRecord 2
					192, 168, 1, 2, // IP
					0x15, 0x7c, // Port 5500
					0x00, 0x02, // NumUsers 2
					0x00, 0x00, // TLSPort
					0x01, // NameSize
					'B',  // Name
					0x01, // DescriptionSize
					'2',  // Description
					// Second ServerInfoHeader
					0x00, 0x01, // MsgType (1)
					0x00, 0x0A, // MsgDataSize
					0x00, 0x04, // SrvCount (4 total)
					0x00, 0x01, // BatchSize (1 in second batch)
					// ServerRecord 3
					192, 168, 1, 3, // IP
					0x15, 0x7c, // Port 5500
					0x00, 0x03, // NumUsers 3
					0x00, 0x00, // TLSPort
					0x01, // NameSize
					'C',  // Name
					0x01, // DescriptionSize
					'3',  // Description
					// Third ServerInfoHeader
					0x00, 0x01, // MsgType (1)
					0x00, 0x0A, // MsgDataSize
					0x00, 0x04, // SrvCount (4 total)
					0x00, 0x01, // BatchSize (1 in third batch)
					// ServerRecord 4
					192, 168, 1, 4, // IP
					0x15, 0x7c, // Port 5500
					0x00, 0x04, // NumUsers 4
					0x00, 0x00, // TLSPort
					0x01, // NameSize
					'D',  // Name
					0x01, // DescriptionSize
					'4',  // Description
				}),
				writeBuffer: &bytes.Buffer{},
			},
			wantErr: false,
			wantResult: []ServerRecord{
				{
					IPAddr:          [4]byte{192, 168, 1, 1},
					Port:            [2]byte{0x15, 0x7c},
					NumUsers:        [2]byte{0x00, 0x01},
					TLSPort:         [2]byte{0x00, 0x00},
					NameSize:        1,
					Name:            []byte("A"),
					DescriptionSize: 1,
					Description:     []byte("1"),
				},
				{
					IPAddr:          [4]byte{192, 168, 1, 2},
					Port:            [2]byte{0x15, 0x7c},
					NumUsers:        [2]byte{0x00, 0x02},
					TLSPort:         [2]byte{0x00, 0x00},
					NameSize:        1,
					Name:            []byte("B"),
					DescriptionSize: 1,
					Description:     []byte("2"),
				},
				{
					IPAddr:          [4]byte{192, 168, 1, 3},
					Port:            [2]byte{0x15, 0x7c},
					NumUsers:        [2]byte{0x00, 0x03},
					TLSPort:         [2]byte{0x00, 0x00},
					NameSize:        1,
					Name:            []byte("C"),
					DescriptionSize: 1,
					Description:     []byte("3"),
				},
				{
					IPAddr:          [4]byte{192, 168, 1, 4},
					Port:            [2]byte{0x15, 0x7c},
					NumUsers:        [2]byte{0x00, 0x04},
					TLSPort:         [2]byte{0x00, 0x00},
					NameSize:        1,
					Name:            []byte("D"),
					DescriptionSize: 1,
					Description:     []byte("4"),
				},
			},
		},
		{
			name: "Error reading second ServerInfoHeader",
			mockConn: &mockConn{
				readBuffer: bytes.NewBuffer([]byte{
					// TrackerHeader
					0x48, 0x54, 0x52, 0x4B, // Protocol "HTRK"
					0x00, 0x01, // Version 1
					// First ServerInfoHeader
					0x00, 0x01, // MsgType (1)
					0x00, 0x0A, // MsgDataSize
					0x00, 0x02, // SrvCount (2 total)
					0x00, 0x01, // BatchSize (1 in first batch)
					// ServerRecord 1
					192, 168, 1, 1, // IP
					0x15, 0x7c, // Port 5500
					0x00, 0x01, // NumUsers 1
					0x00, 0x00, // TLSPort
					0x01, // NameSize
					'A',  // Name
					0x01, // DescriptionSize
					'1',  // Description
					// Incomplete second ServerInfoHeader
					0x00, 0x01, // MsgType only
				}),
				writeBuffer: &bytes.Buffer{},
			},
			wantErr:    true,
			wantResult: nil,
		},
		{
			name: "Empty server list",
			mockConn: &mockConn{
				readBuffer: bytes.NewBuffer([]byte{
					// TrackerHeader
					0x48, 0x54, 0x52, 0x4B, // Protocol "HTRK"
					0x00, 0x01, // Version 1
					// ServerInfoHeader with 0 servers
					0x00, 0x01, // MsgType (1)
					0x00, 0x00, // MsgDataSize (0)
					0x00, 0x00, // SrvCount (0)
					0x00, 0x00, // BatchSize (0)
				}),
				writeBuffer: &bytes.Buffer{},
			},
			wantErr:    false,
			wantResult: []ServerRecord{},
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
