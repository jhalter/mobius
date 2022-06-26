package hotline

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_accessBitmap_IsSet(t *testing.T) {
	type args struct {
		i int
	}
	tests := []struct {
		name string
		bits accessBitmap
		args args
		want bool
	}{
		{
			name: "returns true when bit is set",
			bits: func() (access accessBitmap) {
				access.Set(22)
				return access
			}(),
			args: args{i: 22},
			want: true,
		},
		{
			name: "returns false when bit is unset",
			bits: accessBitmap{},
			args: args{i: 22},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, tt.bits.IsSet(tt.args.i), "IsSet(%v)", tt.args.i)
		})
	}
}
