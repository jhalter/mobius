package mobius

import (
	"fmt"
	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewYAMLAccountManager(t *testing.T) {
	type args struct {
		accountDir string
	}
	tests := []struct {
		name    string
		args    args
		want    *YAMLAccountManager
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "loads accounts from a directory",
			args: args{
				accountDir: "test/config/Users",
			},
			want: &YAMLAccountManager{
				accountDir: "test/config/Users",
				accounts: map[string]hotline.Account{
					"admin": {
						Name:     "admin",
						Login:    "admin",
						Password: "$2a$04$2itGEYx8C1N5bsfRSoC9JuonS3I4YfnyVPZHLSwp7kEInRX0yoB.a",
						Access:   hotline.AccessBitmap{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x00, 0x00},
					},
					"Test User Name": {
						Name:     "test-user",
						Login:    "Test User Name",
						Password: "$2a$04$9P/jgLn1fR9TjSoWL.rKxuN6g.1TSpf2o6Hw.aaRuBwrWIJNwsKkS",
						Access:   hotline.AccessBitmap{0x7d, 0xf0, 0x0c, 0xef, 0xab, 0x80, 0x00, 0x00},
					},
					"guest": {
						Name:     "guest",
						Login:    "guest",
						Password: "$2a$04$6Yq/TIlgjSD.FbARwtYs9ODnkHawonu1TJ5W2jJKfhnHwBIQTk./y",
						Access:   hotline.AccessBitmap{0x7d, 0xf0, 0x0c, 0xef, 0xab, 0x80, 0x00, 0x00},
					},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewYAMLAccountManager(tt.args.accountDir)
			if !tt.wantErr(t, err, fmt.Sprintf("NewYAMLAccountManager(%v)", tt.args.accountDir)) {
				return
			}

			assert.Equal(t,
				&hotline.Account{
					Name:     "admin",
					Login:    "admin",
					Password: "$2a$04$2itGEYx8C1N5bsfRSoC9JuonS3I4YfnyVPZHLSwp7kEInRX0yoB.a",
					Access:   hotline.AccessBitmap{0xff, 0xff, 0xef, 0xff, 0xff, 0x80, 0x00, 0x00},
				},
				got.Get("admin"),
			)

			assert.Equal(t,
				&hotline.Account{
					Login:    "guest",
					Name:     "guest",
					Password: "$2a$04$6Yq/TIlgjSD.FbARwtYs9ODnkHawonu1TJ5W2jJKfhnHwBIQTk./y",
					Access:   hotline.AccessBitmap{0x60, 0x70, 0x0c, 0x20, 0x03, 0x80, 0x00, 0x00},
				},
				got.Get("guest"),
			)

			assert.Equal(t,
				&hotline.Account{
					Login:    "test-user",
					Name:     "Test User Name",
					Password: "$2a$04$9P/jgLn1fR9TjSoWL.rKxuN6g.1TSpf2o6Hw.aaRuBwrWIJNwsKkS",
					Access:   hotline.AccessBitmap{0x7d, 0xf0, 0x0c, 0xef, 0xab, 0x80, 0x00, 0x00},
				},
				got.Get("test-user"),
			)
		})
	}
}
