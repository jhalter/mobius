package mobius

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func fieldValidationClient(t *testing.T) *hotline.ClientConn {
	t.Helper()
	srv, err := hotline.NewServer(hotline.WithLogger(NewTestLogger()))
	require.NoError(t, err)
	srv.Config.FileRoot = t.TempDir()
	srv.AccountManager = &YAMLAccountManager{accountDir: t.TempDir(), accounts: map[string]hotline.Account{}}
	cc := srv.NewClientConn(nopReadWriteCloser{}, "127.0.0.1:1")
	cc.Logger = srv.Logger
	cc.Account = &hotline.Account{Login: "admin"}
	for i := 0; i < 64; i++ {
		cc.Account.Access.Set(i)
	}
	srv.ClientMgr.Add(cc)
	return cc
}

func accountSubfields(t *testing.T, fields ...hotline.Field) hotline.Field {
	t.Helper()
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, uint16(len(fields)))
	for _, f := range fields {
		data, err := io.ReadAll(&f)
		require.NoError(t, err)
		b = append(b, data...)
	}
	return hotline.NewField(hotline.FieldData, b)
}

func TestSetClientUserInfoShortOptions(t *testing.T) {
	for _, tc := range []struct {
		name       string
		data       []byte
		wantRefuse bool
	}{
		{"absent", nil, true}, {"empty", []byte{}, false}, {"one byte", []byte{1}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cc := fieldValidationClient(t)
			cc.SetFlag(hotline.UserFlagRefusePM, 1)
			cc.SetAutoReply([]byte("away"))
			req := hotline.NewTransaction(hotline.TranSetClientUserInfo, cc.ID,
				hotline.NewField(hotline.FieldUserName, []byte("name")), hotline.NewField(hotline.FieldUserIconID, []byte{0, 1}))
			if tc.data != nil {
				req.Fields = append(req.Fields, hotline.NewField(hotline.FieldOptions, tc.data))
			}
			replies := HandleSetClientUserInfo(cc, &req)
			require.Len(t, replies, 1)
			assert.Equal(t, hotline.TranNotifyChangeUser, replies[0].Type)
			assert.Equal(t, tc.wantRefuse, cc.IsFlagSet(hotline.UserFlagRefusePM))
			if tc.data == nil {
				assert.Equal(t, []byte("away"), cc.GetAutoReply())
			} else {
				assert.Empty(t, cc.GetAutoReply())
			}
		})
	}
}

func TestUpdateUserOptionalFields(t *testing.T) {
	login := hotline.NewField(hotline.FieldUserLogin, hotline.EncodeString([]byte("newuser")))
	optional := []hotline.Field{
		hotline.NewField(hotline.FieldUserName, []byte("New User")),
		hotline.NewField(hotline.FieldUserPassword, []byte("password")),
		hotline.NewField(hotline.FieldUserAccess, make([]byte, 8)),
	}
	for _, omit := range []int{-1, 0, 1, 2} {
		t.Run(fmt.Sprintf("omit optional field %d", omit), func(t *testing.T) {
			cc := fieldValidationClient(t)
			fields := []hotline.Field{login}
			if omit >= 0 {
				for i, f := range optional {
					if i != omit {
						fields = append(fields, f)
					}
				}
			}
			req := hotline.NewTransaction(hotline.TranUpdateUser, cc.ID, accountSubfields(t, fields...))
			replies := HandleUpdateUser(cc, &req)
			require.Len(t, replies, 1)
			require.Equal(t, [4]byte{}, replies[0].ErrorCode)
			account := cc.Server.AccountManager.Get("newuser")
			require.NotNil(t, account)
			assert.Equal(t, hotline.AccessBitmap{}, account.Access)
			if omit == -1 || omit == 0 {
				assert.Empty(t, account.Name)
			} else {
				assert.Equal(t, "New User", account.Name)
			}
			password := "password"
			if omit == -1 || omit == 1 {
				password = ""
			}
			assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(password)))
		})
	}
}

func TestUpdateUserPreservesPasswordSemanticsAndOmittedName(t *testing.T) {
	for _, tc := range []struct {
		name     string
		data     []byte
		password string
	}{
		{"omitted clears password", nil, ""}, {"sentinel keeps password", []byte{0}, "original"},
		{"new password", []byte("new"), "new"}, {"empty password", []byte{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cc := fieldValidationClient(t)
			var access hotline.AccessBitmap
			access.Set(hotline.AccessDownloadFile)
			require.NoError(t, cc.Server.AccountManager.Create(*hotline.NewAccount("existing", "Keep this name", "original", access)))
			fields := []hotline.Field{hotline.NewField(hotline.FieldUserLogin, hotline.EncodeString([]byte("existing")))}
			if tc.data != nil {
				fields = append(fields, hotline.NewField(hotline.FieldUserPassword, tc.data))
			}
			req := hotline.NewTransaction(hotline.TranUpdateUser, cc.ID, accountSubfields(t, fields...))
			replies := HandleUpdateUser(cc, &req)
			require.Len(t, replies, 1)
			require.Equal(t, [4]byte{}, replies[0].ErrorCode)
			account := cc.Server.AccountManager.Get("existing")
			require.NotNil(t, account)
			assert.Equal(t, "Keep this name", account.Name)
			assert.Equal(t, access, account.Access)
			assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(tc.password)))
		})
	}
}

func TestUpdateUserRejectsMissingLoginAndMalformedAccess(t *testing.T) {
	login := hotline.NewField(hotline.FieldUserLogin, hotline.EncodeString([]byte("existing")))
	for _, tc := range []struct {
		name   string
		fields []hotline.Field
	}{
		{"no subfields", nil},
		{"wrong delete field", []hotline.Field{hotline.NewField(hotline.FieldUserName, []byte("name"))}},
		{"empty delete login", []hotline.Field{hotline.NewField(hotline.FieldData, nil)}},
		{"missing rename login", []hotline.Field{hotline.NewField(hotline.FieldData, []byte("old")), hotline.NewField(hotline.FieldUserName, []byte("name"))}},
		{"empty login", []hotline.Field{hotline.NewField(hotline.FieldUserLogin, nil)}},
		{"short access", []hotline.Field{login, hotline.NewField(hotline.FieldUserAccess, []byte{0})}},
		{"long access", []hotline.Field{login, hotline.NewField(hotline.FieldUserAccess, make([]byte, 9))}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cc := fieldValidationClient(t)
			original := hotline.NewAccount("existing", "Original", "secret", hotline.AccessBitmap{})
			require.NoError(t, cc.Server.AccountManager.Create(*original))
			req := hotline.NewTransaction(hotline.TranUpdateUser, cc.ID, accountSubfields(t, tc.fields...))
			replies := HandleUpdateUser(cc, &req)
			require.Len(t, replies, 1)
			assert.NotEqual(t, [4]byte{}, replies[0].ErrorCode)
			assert.Equal(t, original, cc.Server.AccountManager.Get("existing"))
		})
	}
}

func TestUploadFolderValidatesItemCount(t *testing.T) {
	for _, tc := range []struct {
		name  string
		data  []byte
		want  int
		valid bool
	}{
		{"missing", nil, 0, false}, {"empty", []byte{}, 0, false}, {"one byte", []byte{1}, 0, false},
		{"three bytes", []byte{0, 0, 1}, 0, false}, {"two bytes", []byte{0, 5}, 5, true},
		{"four bytes", []byte{0, 0, 0, 5}, 5, true}, {"zero", []byte{0, 0}, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cc := fieldValidationClient(t)
			req := hotline.NewTransaction(hotline.TranUploadFldr, cc.ID,
				hotline.NewField(hotline.FieldFileName, []byte("folder")), hotline.NewField(hotline.FieldTransferSize, []byte{0, 42}))
			if tc.data != nil {
				req.Fields = append(req.Fields, hotline.NewField(hotline.FieldFolderItemCount, tc.data))
			}
			replies := HandleUploadFolder(cc, &req)
			require.Len(t, replies, 1)
			transfers := cc.ClientFileTransferMgr.Get(hotline.FolderUpload)
			if !tc.valid {
				assert.NotEqual(t, [4]byte{}, replies[0].ErrorCode)
				assert.Empty(t, transfers)
				return
			}
			require.Equal(t, [4]byte{}, replies[0].ErrorCode)
			require.Len(t, transfers, 1)
			assert.Equal(t, tc.want, transfers[0].ItemCount())
			assert.Equal(t, []byte{0, 0, 0, 42}, transfers[0].TransferSize)
		})
	}
}

func TestUploadsValidateTransferSize(t *testing.T) {
	for _, folder := range []bool{false, true} {
		for _, tc := range []struct {
			name  string
			data  []byte
			valid bool
		}{
			{"absent", nil, !folder}, {"empty", []byte{}, false}, {"one byte", []byte{1}, false},
			{"three bytes", []byte{0, 0, 1}, false}, {"two bytes", []byte{0, 42}, true}, {"four bytes", []byte{0, 0, 0, 42}, true},
		} {
			kind, transferType, handler := hotline.TranUploadFile, hotline.FileUpload, HandleUploadFile
			if folder {
				kind, transferType, handler = hotline.TranUploadFldr, hotline.FolderUpload, HandleUploadFolder
			}
			t.Run(fmt.Sprintf("folder=%t/%s", folder, tc.name), func(t *testing.T) {
				cc := fieldValidationClient(t)
				req := hotline.NewTransaction(kind, cc.ID, hotline.NewField(hotline.FieldFileName, []byte("incoming")), hotline.NewField(hotline.FieldFolderItemCount, []byte{0, 1}))
				if tc.data != nil {
					req.Fields = append(req.Fields, hotline.NewField(hotline.FieldTransferSize, tc.data))
				}
				replies := handler(cc, &req)
				require.Len(t, replies, 1)
				transfers := cc.ClientFileTransferMgr.Get(transferType)
				if !tc.valid {
					assert.NotEqual(t, [4]byte{}, replies[0].ErrorCode)
					assert.Empty(t, transfers)
					return
				}
				require.Equal(t, [4]byte{}, replies[0].ErrorCode)
				require.Len(t, transfers, 1)
				if tc.data == nil {
					assert.Nil(t, transfers[0].TransferSize)
				} else {
					assert.Equal(t, []byte{0, 0, 0, 42}, transfers[0].TransferSize)
				}
				infoReq := hotline.NewTransaction(hotline.TranGetClientInfoText, cc.ID, hotline.NewField(hotline.FieldUserID, cc.ID[:]))
				info := HandleGetClientInfoText(cc, &infoReq)
				require.Len(t, info, 1)
				assert.Equal(t, [4]byte{}, info[0].ErrorCode)
				assert.Contains(t, string(info[0].GetField(hotline.FieldData).Data), "incoming")
			})
		}
	}
}

func TestResumeUploadKeepsOmittedSizeUnknown(t *testing.T) {
	cc := fieldValidationClient(t)
	require.NoError(t, os.WriteFile(filepath.Join(cc.FileRoot(), "incoming"+hotline.IncompleteFileSuffix), []byte("partial"), 0600))
	req := hotline.NewTransaction(hotline.TranUploadFile, cc.ID,
		hotline.NewField(hotline.FieldFileName, []byte("incoming")), hotline.NewField(hotline.FieldFileTransferOptions, []byte{0, 2}))
	replies := HandleUploadFile(cc, &req)
	require.Len(t, replies, 1)
	require.Equal(t, [4]byte{}, replies[0].ErrorCode)
	assert.NotEmpty(t, replies[0].GetField(hotline.FieldFileResumeData).Data)
	transfers := cc.ClientFileTransferMgr.Get(hotline.FileUpload)
	require.Len(t, transfers, 1)
	assert.Nil(t, transfers[0].TransferSize)
	assert.Contains(t, transfers[0].String(), "?%")
}
