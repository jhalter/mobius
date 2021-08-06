package hotline

import "testing"

func TestTransfer_Read(t *testing.T) {
	type fields struct {
		Protocol        [4]byte
		ReferenceNumber [4]byte
		DataSize        [4]byte
		RSVD            [4]byte
	}
	type args struct {
		b []byte
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    int
		wantErr bool
	}{
		{
			name: "when b is a valid transfer",
			fields: fields{
				Protocol:        [4]byte{},
				ReferenceNumber: [4]byte{},
				DataSize:        [4]byte{},
				RSVD:            [4]byte{},
			},
			args: args{
				b: []byte{
					0x48, 0x54, 0x58, 0x46,
					0x00, 0x00, 0x00, 0x01,
					0x00, 0x00, 0x00, 0x02,
					0x00, 0x00, 0x00, 0x00,
				},
			},
			want:    16,
			wantErr: false,
		},
		{
			name: "when b contains invalid transfer protocol",
			fields: fields{
				Protocol:        [4]byte{},
				ReferenceNumber: [4]byte{},
				DataSize:        [4]byte{},
				RSVD:            [4]byte{},
			},
			args: args{
				b: []byte{
					0x11, 0x11, 0x11, 0x11,
					0x00, 0x00, 0x00, 0x01,
					0x00, 0x00, 0x00, 0x02,
					0x00, 0x00, 0x00, 0x00,
				},
			},
			want:    0,
			wantErr: true,
		},
		{
			name: "when b does not contain expected len of bytes",
			fields: fields{
				Protocol:        [4]byte{},
				ReferenceNumber: [4]byte{},
				DataSize:        [4]byte{},
				RSVD:            [4]byte{},
			},
			args: args{
				b: []byte{
					0x48, 0x54, 0x58, 0x46,
					0x00, 0x00, 0x00, 0x01,
					0x00, 0x00, 0x00, 0x02,
					0x00, 0x00, 0x00,
				},
			},
			want:    0,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tf := &transfer{
				Protocol:        tt.fields.Protocol,
				ReferenceNumber: tt.fields.ReferenceNumber,
				DataSize:        tt.fields.DataSize,
				RSVD:            tt.fields.RSVD,
			}
			got, err := tf.Write(tt.args.b)
			if (err != nil) != tt.wantErr {
				t.Errorf("Read() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Read() got = %v, want %v", got, tt.want)
			}
		})
	}
}
