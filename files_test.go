package hotline

import (
	"bytes"
	"os"
	"reflect"
	"testing"
)

func TestEncodeFilePath(t *testing.T) {
	var tests = []struct {
		filePath string
		want     []byte
	}{
		{
			filePath: "kitten1.jpg",
			want: []byte{
				0x00, 0x01, // length of next path section (1)
				0x00,       // leading path separator
				0x00, 0x0b, // length of next path section (11)
				0x6b, 0x69, 0x74, 0x74, 0x65, 0x6e, 0x31, 0x2e, 0x6a, 0x70, 0x67, // kitten1.jpg
			},
		},
	}

	for _, test := range tests {
		got := EncodeFilePath(test.filePath)
		if bytes.Compare(got, test.want) != 0 {
			t.Errorf("field mismatch:  want: %#v got: %#v", test.want, got)
		}
	}
}

func TestCalcTotalSize(t *testing.T) {
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)

	err := os.Chdir("test/config/Files")
	if err != nil {
		panic(err)
	}

	type args struct {
		filePath string
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "Foo",
			args: args{
				filePath: "test",
			},
			want: []byte{0x00, 0x00, 0x18, 0x00},
			wantErr: false,
		},
		// TODO: Add more test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalcTotalSize(tt.args.filePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("CalcTotalSize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CalcTotalSize() got = %v, want %v", got, tt.want)
			}
		})
	}
}