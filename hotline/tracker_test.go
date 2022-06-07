package hotline

import (
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
			if got := tr.Payload(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Payload() = %v, want %v", got, tt.want)
			}
		})
	}
}
