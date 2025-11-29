package hotline

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func Test_accessBitmap_IsSet(t *testing.T) {
	type args struct {
		i int
	}
	tests := []struct {
		name string
		bits AccessBitmap
		args args
		want bool
	}{
		{
			name: "returns true when bit is set",
			bits: func() (access AccessBitmap) {
				access.Set(22)
				return access
			}(),
			args: args{i: 22},
			want: true,
		},
		{
			name: "returns false when bit is unset",
			bits: AccessBitmap{},
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

func Test_accessBitmap_UnmarshalYAML(t *testing.T) {
	// Test direct method call for explicit coverage
	t.Run("direct method call", func(t *testing.T) {
		var bits AccessBitmap
		err := bits.UnmarshalYAML(func(v interface{}) error {
			switch ptr := v.(type) {
			case *interface{}:
				*ptr = map[string]interface{}{
					"DownloadFile": true,
					"UploadFile":   true,
				}
			}
			return nil
		})
		require.NoError(t, err)
		assert.True(t, bits.IsSet(AccessDownloadFile))
		assert.True(t, bits.IsSet(AccessUploadFile))
	})

	// Original YAML unmarshaling tests
	tests := []struct {
		name     string
		yamlData string
		expected AccessBitmap
		wantErr  bool
	}{
		{
			name:     "unmarshal array format (legacy)",
			yamlData: "access: [96, 112, 12, 32, 3, 128, 0, 0]",
			expected: AccessBitmap{96, 112, 12, 32, 3, 128, 0, 0},
			wantErr:  false,
		},
		{
			name: "unmarshal map format with true values",
			yamlData: `access:
  DownloadFile: true
  UploadFile: true
  DeleteFile: true`,
			expected: func() AccessBitmap {
				var bits AccessBitmap
				bits.Set(AccessDownloadFile)
				bits.Set(AccessUploadFile)
				bits.Set(AccessDeleteFile)
				return bits
			}(),
			wantErr: false,
		},
		{
			name: "unmarshal map format with false values",
			yamlData: `access:
  DownloadFile: false
  UploadFile: false
  DeleteFile: false`,
			expected: AccessBitmap{},
			wantErr:  false,
		},
		{
			name: "unmarshal map format with mixed values",
			yamlData: `access:
  DownloadFile: true
  UploadFile: false
  DeleteFile: true
  CreateFolder: true`,
			expected: func() AccessBitmap {
				var bits AccessBitmap
				bits.Set(AccessDownloadFile)
				bits.Set(AccessDeleteFile)
				bits.Set(AccessCreateFolder)
				return bits
			}(),
			wantErr: false,
		},
		{
			name: "unmarshal map format with all permissions",
			yamlData: `access:
  DeleteFile: true
  UploadFile: true
  DownloadFile: true
  RenameFile: true
  MoveFile: true
  CreateFolder: true
  DeleteFolder: true
  RenameFolder: true
  MoveFolder: true
  ReadChat: true
  SendChat: true
  OpenChat: true
  CloseChat: true
  ShowInList: true
  CreateUser: true
  DeleteUser: true
  OpenUser: true
  ModifyUser: true
  ChangeOwnPass: true
  NewsReadArt: true
  NewsPostArt: true
  DisconnectUser: true
  CannotBeDisconnected: true
  GetClientInfo: true
  UploadAnywhere: true
  AnyName: true
  NoAgreement: true
  SetFileComment: true
  SetFolderComment: true
  ViewDropBoxes: true
  MakeAlias: true
  Broadcast: true
  NewsDeleteArt: true
  NewsCreateCat: true
  NewsDeleteCat: true
  NewsCreateFldr: true
  NewsDeleteFldr: true
  SendPrivMsg: true
  UploadFolder: true
  DownloadFolder: true`,
			expected: func() AccessBitmap {
				var bits AccessBitmap
				bits.Set(AccessDeleteFile)
				bits.Set(AccessUploadFile)
				bits.Set(AccessDownloadFile)
				bits.Set(AccessRenameFile)
				bits.Set(AccessMoveFile)
				bits.Set(AccessCreateFolder)
				bits.Set(AccessDeleteFolder)
				bits.Set(AccessRenameFolder)
				bits.Set(AccessMoveFolder)
				bits.Set(AccessReadChat)
				bits.Set(AccessSendChat)
				bits.Set(AccessOpenChat)
				bits.Set(AccessCloseChat)
				bits.Set(AccessShowInList)
				bits.Set(AccessCreateUser)
				bits.Set(AccessDeleteUser)
				bits.Set(AccessOpenUser)
				bits.Set(AccessModifyUser)
				bits.Set(AccessChangeOwnPass)
				bits.Set(AccessNewsReadArt)
				bits.Set(AccessNewsPostArt)
				bits.Set(AccessDisconUser)
				bits.Set(AccessCannotBeDiscon)
				bits.Set(AccessGetClientInfo)
				bits.Set(AccessUploadAnywhere)
				bits.Set(AccessAnyName)
				bits.Set(AccessNoAgreement)
				bits.Set(AccessSetFileComment)
				bits.Set(AccessSetFolderComment)
				bits.Set(AccessViewDropBoxes)
				bits.Set(AccessMakeAlias)
				bits.Set(AccessBroadcast)
				bits.Set(AccessNewsDeleteArt)
				bits.Set(AccessNewsCreateCat)
				bits.Set(AccessNewsDeleteCat)
				bits.Set(AccessNewsCreateFldr)
				bits.Set(AccessNewsDeleteFldr)
				bits.Set(AccessSendPrivMsg)
				bits.Set(AccessUploadFolder)
				bits.Set(AccessDownloadFolder)
				return bits
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data struct {
				Access AccessBitmap `yaml:"access"`
			}

			err := yaml.Unmarshal([]byte(tt.yamlData), &data)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, data.Access)
		})
	}
}

func Test_accessBitmap_MarshalYAML(t *testing.T) {
	tests := []struct {
		name     string
		bits     AccessBitmap
		expected string
	}{
		{
			name: "empty access bitmap",
			bits: AccessBitmap{},
			expected: `    DownloadFile: false
    DownloadFolder: false
    UploadFile: false
    UploadFolder: false
    DeleteFile: false
    RenameFile: false
    MoveFile: false
    CreateFolder: false
    DeleteFolder: false
    RenameFolder: false
    MoveFolder: false
    ReadChat: false
    SendChat: false
    OpenChat: false
    CloseChat: false
    ShowInList: false
    CreateUser: false
    DeleteUser: false
    OpenUser: false
    ModifyUser: false
    ChangeOwnPass: false
    NewsReadArt: false
    NewsPostArt: false
    DisconnectUser: false
    CannotBeDisconnected: false
    GetClientInfo: false
    UploadAnywhere: false
    AnyName: false
    NoAgreement: false
    SetFileComment: false
    SetFolderComment: false
    ViewDropBoxes: false
    MakeAlias: false
    Broadcast: false
    NewsDeleteArt: false
    NewsCreateCat: false
    NewsDeleteCat: false
    NewsCreateFldr: false
    NewsDeleteFldr: false
    SendPrivMsg: false
`,
		},
		{
			name: "access bitmap with some permissions",
			bits: func() AccessBitmap {
				var bits AccessBitmap
				bits.Set(AccessDownloadFile)
				bits.Set(AccessUploadFile)
				bits.Set(AccessCreateFolder)
				bits.Set(AccessReadChat)
				return bits
			}(),
			expected: `    DownloadFile: true
    DownloadFolder: false
    UploadFile: true
    UploadFolder: false
    DeleteFile: false
    RenameFile: false
    MoveFile: false
    CreateFolder: true
    DeleteFolder: false
    RenameFolder: false
    MoveFolder: false
    ReadChat: true
    SendChat: false
    OpenChat: false
    CloseChat: false
    ShowInList: false
    CreateUser: false
    DeleteUser: false
    OpenUser: false
    ModifyUser: false
    ChangeOwnPass: false
    NewsReadArt: false
    NewsPostArt: false
    DisconnectUser: false
    CannotBeDisconnected: false
    GetClientInfo: false
    UploadAnywhere: false
    AnyName: false
    NoAgreement: false
    SetFileComment: false
    SetFolderComment: false
    ViewDropBoxes: false
    MakeAlias: false
    Broadcast: false
    NewsDeleteArt: false
    NewsCreateCat: false
    NewsDeleteCat: false
    NewsCreateFldr: false
    NewsDeleteFldr: false
    SendPrivMsg: false
`,
		},
		{
			name: "access bitmap with all permissions",
			bits: func() AccessBitmap {
				var bits AccessBitmap
				bits.Set(AccessDeleteFile)
				bits.Set(AccessUploadFile)
				bits.Set(AccessDownloadFile)
				bits.Set(AccessRenameFile)
				bits.Set(AccessMoveFile)
				bits.Set(AccessCreateFolder)
				bits.Set(AccessDeleteFolder)
				bits.Set(AccessRenameFolder)
				bits.Set(AccessMoveFolder)
				bits.Set(AccessReadChat)
				bits.Set(AccessSendChat)
				bits.Set(AccessOpenChat)
				bits.Set(AccessCloseChat)
				bits.Set(AccessShowInList)
				bits.Set(AccessCreateUser)
				bits.Set(AccessDeleteUser)
				bits.Set(AccessOpenUser)
				bits.Set(AccessModifyUser)
				bits.Set(AccessChangeOwnPass)
				bits.Set(AccessNewsReadArt)
				bits.Set(AccessNewsPostArt)
				bits.Set(AccessDisconUser)
				bits.Set(AccessCannotBeDiscon)
				bits.Set(AccessGetClientInfo)
				bits.Set(AccessUploadAnywhere)
				bits.Set(AccessAnyName)
				bits.Set(AccessNoAgreement)
				bits.Set(AccessSetFileComment)
				bits.Set(AccessSetFolderComment)
				bits.Set(AccessViewDropBoxes)
				bits.Set(AccessMakeAlias)
				bits.Set(AccessBroadcast)
				bits.Set(AccessNewsDeleteArt)
				bits.Set(AccessNewsCreateCat)
				bits.Set(AccessNewsDeleteCat)
				bits.Set(AccessNewsCreateFldr)
				bits.Set(AccessNewsDeleteFldr)
				bits.Set(AccessSendPrivMsg)
				bits.Set(AccessUploadFolder)
				bits.Set(AccessDownloadFolder)
				return bits
			}(),
			expected: `    DownloadFile: true
    DownloadFolder: true
    UploadFile: true
    UploadFolder: true
    DeleteFile: true
    RenameFile: true
    MoveFile: true
    CreateFolder: true
    DeleteFolder: true
    RenameFolder: true
    MoveFolder: true
    ReadChat: true
    SendChat: true
    OpenChat: true
    CloseChat: true
    ShowInList: true
    CreateUser: true
    DeleteUser: true
    OpenUser: true
    ModifyUser: true
    ChangeOwnPass: true
    NewsReadArt: true
    NewsPostArt: true
    DisconnectUser: true
    CannotBeDisconnected: true
    GetClientInfo: true
    UploadAnywhere: true
    AnyName: true
    NoAgreement: true
    SetFileComment: true
    SetFolderComment: true
    ViewDropBoxes: true
    MakeAlias: true
    Broadcast: true
    NewsDeleteArt: true
    NewsCreateCat: true
    NewsDeleteCat: true
    NewsCreateFldr: true
    NewsDeleteFldr: true
    SendPrivMsg: true
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test via YAML marshaling
			yamlData, err := yaml.Marshal(struct {
				Access AccessBitmap `yaml:"access"`
			}{Access: tt.bits})

			require.NoError(t, err)

			// Extract just the access portion
			lines := strings.Split(string(yamlData), "\n")
			var accessLines []string
			inAccess := false
			for _, line := range lines {
				if strings.HasPrefix(line, "access:") {
					inAccess = true
					continue
				}
				if inAccess && strings.HasPrefix(line, "    ") {
					accessLines = append(accessLines, line)
				}
			}

			actualAccess := strings.Join(accessLines, "\n") + "\n"
			assert.Equal(t, tt.expected, actualAccess)
		})
	}
}
