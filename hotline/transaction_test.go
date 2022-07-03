package hotline

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestReadFields(t *testing.T) {
	type args struct {
		paramCount []byte
		buf        []byte
	}
	tests := []struct {
		name    string
		args    args
		want    []Field
		wantErr bool
	}{
		{
			name: "valid field data",
			args: args{
				paramCount: []byte{0x00, 0x02},
				buf: []byte{
					0x00, 0x65, // ID: fieldData
					0x00, 0x04, // Size: 2 bytes
					0x01, 0x02, 0x03, 0x04, // Data
					0x00, 0x66, // ID: fieldUserName
					0x00, 0x02, // Size: 2 bytes
					0x00, 0x01, // Data
				},
			},
			want: []Field{
				{
					ID:        []byte{0x00, 0x65},
					FieldSize: []byte{0x00, 0x04},
					Data:      []byte{0x01, 0x02, 0x03, 0x04},
				},
				{
					ID:        []byte{0x00, 0x66},
					FieldSize: []byte{0x00, 0x02},
					Data:      []byte{0x00, 0x01},
				},
			},
			wantErr: false,
		},
		{
			name: "empty bytes",
			args: args{
				paramCount: []byte{0x00, 0x00},
				buf:        []byte{},
			},
			want:    []Field(nil),
			wantErr: false,
		},
		{
			name: "when field size does not match data length",
			args: args{
				paramCount: []byte{0x00, 0x01},
				buf: []byte{
					0x00, 0x65, // ID: fieldData
					0x00, 0x04, // Size: 4 bytes
					0x01, 0x02, 0x03, // Data
				},
			},
			want:    []Field{},
			wantErr: true,
		},
		{
			name: "when field size of second field does not match data length",
			args: args{
				paramCount: []byte{0x00, 0x01},
				buf: []byte{
					0x00, 0x65, // ID: fieldData
					0x00, 0x02, // Size: 2 bytes
					0x01, 0x02, // Data
					0x00, 0x65, // ID: fieldData
					0x00, 0x04, // Size: 4 bytes
					0x01, 0x02, 0x03, // Data
				},
			},
			want:    []Field{},
			wantErr: true,
		},
		{
			name: "when field data has extra bytes",
			args: args{
				paramCount: []byte{0x00, 0x01},
				buf: []byte{
					0x00, 0x65, // ID: fieldData
					0x00, 0x02, // Size: 2 bytes
					0x01, 0x02, 0x03, // Data
				},
			},
			want:    []Field{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadFields(tt.args.paramCount, tt.args.buf)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadFields() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !assert.Equal(t, tt.want, got) {
				t.Errorf("ReadFields() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_transactionScanner(t *testing.T) {
	type args struct {
		data []byte
		in1  bool
	}
	tests := []struct {
		name        string
		args        args
		wantAdvance int
		wantToken   []byte
		wantErr     assert.ErrorAssertionFunc
	}{
		{
			name: "when too few bytes are provided to read the transaction size",
			args: args{
				data: []byte{},
				in1:  false,
			},
			wantAdvance: 0,
			wantToken:   []byte(nil),
			wantErr:     assert.NoError,
		},
		{
			name: "when too few bytes are provided to read the full payload",
			args: args{
				data: []byte{
					0,
					1,
					0, 0,
					0, 00, 00, 04,
					00, 00, 00, 00,
					00, 00, 00, 10,
					00, 00, 00, 10,
				},
				in1: false,
			},
			wantAdvance: 0,
			wantToken:   []byte(nil),
			wantErr:     assert.NoError,
		},
		{
			name: "when a full transaction is provided",
			args: args{
				data: []byte{
					0,
					1,
					0, 0,
					0, 00, 00, 0x04,
					00, 00, 00, 0x00,
					00, 00, 00, 0x10,
					00, 00, 00, 0x10,
					00, 02,
					00, 0x6c, // 108 - fieldTransferSize
					00, 02,
					0x63, 0x3b,
					00, 0x6b, // 107 = fieldRefNum
					00, 0x04,
					00, 0x02, 0x93, 0x47,
				},
				in1: false,
			},
			wantAdvance: 36,
			wantToken: []byte{
				0,
				1,
				0, 0,
				0, 00, 00, 0x04,
				00, 00, 00, 0x00,
				00, 00, 00, 0x10,
				00, 00, 00, 0x10,
				00, 02,
				00, 0x6c, // 108 - fieldTransferSize
				00, 02,
				0x63, 0x3b,
				00, 0x6b, // 107 = fieldRefNum
				00, 0x04,
				00, 0x02, 0x93, 0x47,
			},
			wantErr: assert.NoError,
		},
		{
			name: "when a full transaction plus extra bytes are provided",
			args: args{
				data: []byte{
					0,
					1,
					0, 0,
					0, 00, 00, 0x04,
					00, 00, 00, 0x00,
					00, 00, 00, 0x10,
					00, 00, 00, 0x10,
					00, 02,
					00, 0x6c, // 108 - fieldTransferSize
					00, 02,
					0x63, 0x3b,
					00, 0x6b, // 107 = fieldRefNum
					00, 0x04,
					00, 0x02, 0x93, 0x47,
					1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
				},
				in1: false,
			},
			wantAdvance: 36,
			wantToken: []byte{
				0,
				1,
				0, 0,
				0, 00, 00, 0x04,
				00, 00, 00, 0x00,
				00, 00, 00, 0x10,
				00, 00, 00, 0x10,
				00, 02,
				00, 0x6c, // 108 - fieldTransferSize
				00, 02,
				0x63, 0x3b,
				00, 0x6b, // 107 = fieldRefNum
				00, 0x04,
				00, 0x02, 0x93, 0x47,
			},
			wantErr: assert.NoError,
		},
		{
			name: "when two full transactions are provided",
			args: args{
				data: []byte{
					0,
					1,
					0, 0,
					0, 00, 00, 0x04,
					00, 00, 00, 0x00,
					00, 00, 00, 0x10,
					00, 00, 00, 0x10,
					00, 02,
					00, 0x6c, // 108 - fieldTransferSize
					00, 02,
					0x63, 0x3b,
					00, 0x6b, // 107 = fieldRefNum
					00, 0x04,
					00, 0x02, 0x93, 0x47,
					0,
					1,
					0, 0,
					0, 00, 00, 0x04,
					00, 00, 00, 0x00,
					00, 00, 00, 0x10,
					00, 00, 00, 0x10,
					00, 02,
					00, 0x6c, // 108 - fieldTransferSize
					00, 02,
					0x63, 0x3b,
					00, 0x6b, // 107 = fieldRefNum
					00, 0x04,
					00, 0x02, 0x93, 0x47,
				},
				in1: false,
			},
			wantAdvance: 36,
			wantToken: []byte{
				0,
				1,
				0, 0,
				0, 00, 00, 0x04,
				00, 00, 00, 0x00,
				00, 00, 00, 0x10,
				00, 00, 00, 0x10,
				00, 02,
				00, 0x6c, // 108 - fieldTransferSize
				00, 02,
				0x63, 0x3b,
				00, 0x6b, // 107 = fieldRefNum
				00, 0x04,
				00, 0x02, 0x93, 0x47,
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAdvance, gotToken, err := transactionScanner(tt.args.data, tt.args.in1)
			if !tt.wantErr(t, err, fmt.Sprintf("transactionScanner(%v, %v)", tt.args.data, tt.args.in1)) {
				return
			}
			assert.Equalf(t, tt.wantAdvance, gotAdvance, "transactionScanner(%v, %v)", tt.args.data, tt.args.in1)
			assert.Equalf(t, tt.wantToken, gotToken, "transactionScanner(%v, %v)", tt.args.data, tt.args.in1)
		})
	}
}

func TestTransaction_Write(t1 *testing.T) {
	sampleTransaction := &Transaction{
		Flags:      byte(0),
		IsReply:    byte(0),
		Type:       []byte{0x000, 0x93},
		ID:         []byte{0x000, 0x00, 0x00, 0x01},
		ErrorCode:  []byte{0x000, 0x00, 0x00, 0x00},
		TotalSize:  []byte{0x000, 0x00, 0x00, 0x08},
		DataSize:   []byte{0x000, 0x00, 0x00, 0x08},
		ParamCount: []byte{0x00, 0x01},
		Fields: []Field{
			{
				ID:        []byte{0x00, 0x01},
				FieldSize: []byte{0x00, 0x02},
				Data:      []byte{0xff, 0xff},
			},
		},
	}

	type fields struct {
		clientID   *[]byte
		Flags      byte
		IsReply    byte
		Type       []byte
		ID         []byte
		ErrorCode  []byte
		TotalSize  []byte
		DataSize   []byte
		ParamCount []byte
		Fields     []Field
	}
	type args struct {
		p []byte
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantN   int
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name:   "when buf contains all bytes for a single transaction",
			fields: fields{},
			args: args{
				p: func() []byte {
					b, _ := sampleTransaction.MarshalBinary()
					return b
				}(),
			},
			wantN:   28,
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &Transaction{
				clientID:   tt.fields.clientID,
				Flags:      tt.fields.Flags,
				IsReply:    tt.fields.IsReply,
				Type:       tt.fields.Type,
				ID:         tt.fields.ID,
				ErrorCode:  tt.fields.ErrorCode,
				TotalSize:  tt.fields.TotalSize,
				DataSize:   tt.fields.DataSize,
				ParamCount: tt.fields.ParamCount,
				Fields:     tt.fields.Fields,
			}
			gotN, err := t.Write(tt.args.p)
			if !tt.wantErr(t1, err, fmt.Sprintf("Write(%v)", tt.args.p)) {
				return
			}
			assert.Equalf(t1, tt.wantN, gotN, "Write(%v)", tt.args.p)
		})
	}
}
