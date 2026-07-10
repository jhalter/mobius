package mobius

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/jhalter/mobius/hotline"
)

// HandleSetUser sets the information for a specific user in the server's list of allowed users.
//
// Fields used in the request:
//   - 105 User login     Required
//   - 106 User password  Optional
//   - 102 User name      Required
//   - 110 User access    Required - User access privileges bitmap
//
// Fields used in the reply: None
func HandleSetUser(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessModifyUser) {
		return cc.NewErrReply(t, ErrMsgNotAllowedModifyAccounts)
	}

	login := t.GetField(hotline.FieldUserLogin).DecodeObfuscatedString()
	userName := string(t.GetField(hotline.FieldUserName).Data)

	newAccessLvl := t.GetField(hotline.FieldUserAccess).Data

	account := cc.Server.AccountManager.Get(login)
	if account == nil {
		return cc.NewErrReply(t, ErrMsgAccountNotFound)
	}
	account.Name = userName
	copy(account.Access[:], newAccessLvl)

	// If the password field is cleared in the Hotline edit user UI, the SetUser transaction does
	// not include FieldUserPassword
	if t.GetField(hotline.FieldUserPassword).Data == nil {
		account.Password = hotline.HashAndSalt([]byte(""))
	}

	if !bytes.Equal([]byte{0}, t.GetField(hotline.FieldUserPassword).Data) {
		account.Password = hotline.HashAndSalt(t.GetField(hotline.FieldUserPassword).Data)
	}

	err := cc.Server.AccountManager.Update(*account, account.Login)
	if err != nil {
		cc.Logger.Error("Error updating account", "err", err)
		return cc.NewErrReply(t, ErrMsgUpdateAccount)
	}

	// Notify connected clients logged in as the user of the new access level
	for _, c := range cc.Server.ClientMgr.List() {
		if c.GetAccount().Login == login {
			newT := hotline.NewTransaction(hotline.TranUserAccess, c.ID, hotline.NewField(hotline.FieldUserAccess, newAccessLvl))
			res = append(res, newT)

			c.SetAccountAccess(account.Access)

			if c.Authorize(hotline.AccessDisconUser) {
				c.SetFlag(hotline.UserFlagAdmin, 1)
			} else {
				c.SetFlag(hotline.UserFlagAdmin, 0)
			}

			cc.SendAll(
				hotline.TranNotifyChangeUser,
				hotline.NewField(hotline.FieldUserID, c.ID[:]),
				hotline.NewField(hotline.FieldUserFlags, c.FlagBytes()),
				hotline.NewField(hotline.FieldUserName, c.GetUserName()),
				hotline.NewField(hotline.FieldUserIconID, c.GetIcon()),
			)
		}
	}

	return append(res, cc.NewReply(t))
}

// HandleGetUser requests the information for a specific user from the server's list of allowed users.
//
// Fields used in the request:
//   - 105 User login  Required
//
// Fields used in the reply:
//   - 102 User name      Account display name
//   - 105 User login     Account login (encoded, each character negated)
//   - 106 User password  Account password
//   - 110 User access    User access privileges bitmap
func HandleGetUser(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessOpenUser) {
		return cc.NewErrReply(t, ErrMsgNotAllowedViewAccounts)
	}

	account := cc.Server.AccountManager.Get(string(t.GetField(hotline.FieldUserLogin).Data))
	if account == nil {
		return cc.NewErrReply(t, ErrMsgAccountNotExist)
	}

	return append(res, cc.NewReply(t,
		hotline.NewField(hotline.FieldUserName, []byte(account.Name)),
		hotline.NewField(hotline.FieldUserLogin, hotline.EncodeString(t.GetField(hotline.FieldUserLogin).Data)),
		hotline.NewField(hotline.FieldUserPassword, []byte(account.Password)),
		hotline.NewField(hotline.FieldUserAccess, account.Access[:]),
	))
}

// HandleListUsers returns a list of all user accounts on the server.
// This is a server-specific transaction not in the original Hotline protocol.
//
// Fields used in the request: None
//
// Fields used in the reply:
//   - 101 Data  Repeated - Serialized account data for each user
func HandleListUsers(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessOpenUser) {
		return cc.NewErrReply(t, ErrMsgNotAllowedViewAccounts)
	}

	var userFields []hotline.Field
	for _, acc := range cc.Server.AccountManager.List() {
		b, err := io.ReadAll(&acc)
		if err != nil {
			cc.Logger.Error("Error reading account", "account", acc.Login, "err", err)
			continue
		}

		userFields = append(userFields, hotline.NewField(hotline.FieldData, b))
	}

	return append(res, cc.NewReply(t, userFields...))
}

// HandleUpdateUser is used by the v1.5+ multi-user editor to perform account editing for multiple users at a time.
// An update can be a mix of these actions:
// * Create user
// * Delete user
// * Modify user (including renaming the account login)
//
// The Transaction sent by the client includes one data field per user that was modified.  This data field in turn
// contains another data field encoded in its payload with a varying number of sub fields depending on which action is
// performed.  This seems to be the only place in the Hotline protocol where a data field contains another data field.
// HandleUpdateUser processes batch user account operations from the v1.5+ multi-user editor.
// This handler supports creating, deleting, and modifying multiple user accounts in a single transaction.
//
// Fields used in the request:
// * 101	Data				Repeated - Each contains encoded sub-fields for one user operation
//
// Sub-fields for user operations:
// * 101	Data				Optional - Original login name (for rename operations)
// * 105	User Login			Required - Login name (new name for renames)
// * 102	User Name			Optional - Display name (for create/modify)
// * 106	User Password		Optional - Password (for create/modify)
// * 110	User Access			Optional - Access permissions (for create/modify)
//
// Fields used in the reply:
// None
func HandleUpdateUser(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	for _, field := range t.Fields {
		var subFields []hotline.Field

		// The first two bytes are the sub-field count; reject a malformed block
		// that is too short to contain it rather than slicing out of range.
		if len(field.Data) < 2 {
			cc.Logger.Error("update user: malformed sub-field block", "len", len(field.Data))
			return cc.NewErrReply(t, ErrMsgUpdateAccount)
		}

		// Create a new scanner for parsing incoming bytes into transaction tokens
		scanner := bufio.NewScanner(bytes.NewReader(field.Data[2:]))
		scanner.Split(hotline.FieldScanner)

		for i := 0; i < int(binary.BigEndian.Uint16(field.Data[0:2])); i++ {
			scanner.Scan()

			var field hotline.Field
			if _, err := field.Write(scanner.Bytes()); err != nil {
				cc.Logger.Error("update user: parse sub-field", "err", err)
				return cc.NewErrReply(t, ErrMsgUpdateAccount)
			}
			subFields = append(subFields, field)
		}

		// If there's only one subfield, that indicates this is a delete operation for the login in FieldData
		if len(subFields) == 1 {
			if !cc.Authorize(hotline.AccessDeleteUser) {
				return cc.NewErrReply(t, ErrMsgNotAllowedDeleteAccounts)
			}

			login := string(hotline.EncodeString(hotline.GetField(hotline.FieldData, &subFields).Data))

			cc.Logger.Info("DeleteUser", "login", login)

			if err := cc.Server.AccountManager.Delete(login); err != nil {
				cc.Logger.Error("Error deleting account", "err", err)
				return cc.NewErrReply(t, ErrMsgDeleteAccount)
			}

			for _, client := range cc.Server.ClientMgr.List() {
				if client.GetAccount().Login == login {
					//					"You are logged in with an account which was deleted."

					res = append(res,
						hotline.NewTransaction(hotline.TranServerMsg, [2]byte{},
							hotline.NewField(hotline.FieldData, []byte(ErrMsgAccountDeleted)),
							hotline.NewField(hotline.FieldChatOptions, []byte{0}),
						),
					)

					go func(c *hotline.ClientConn) {
						time.Sleep(3 * time.Second)
						c.Disconnect()
					}(client)
				}
			}

			continue
		}

		// login of the account to update
		var accountToUpdate, loginToRename string

		// If FieldData is included, this is a rename operation where FieldData contains the login of the existing
		// account and FieldUserLogin contains the new login.
		if hotline.GetField(hotline.FieldData, &subFields) != nil {
			loginToRename = string(hotline.EncodeString(hotline.GetField(hotline.FieldData, &subFields).Data))
		}
		userLogin := string(hotline.EncodeString(hotline.GetField(hotline.FieldUserLogin, &subFields).Data))
		if loginToRename != "" {
			accountToUpdate = loginToRename
		} else {
			accountToUpdate = userLogin
		}

		// Check if accountToUpdate has an existing account.  If so, we know we are updating an existing user.
		if acc := cc.Server.AccountManager.Get(accountToUpdate); acc != nil {
			if loginToRename != "" {
				cc.Logger.Info("RenameUser", "prevLogin", accountToUpdate, "newLogin", userLogin)
			} else {
				cc.Logger.Info("UpdateUser", "login", accountToUpdate)
			}

			// Account exists, so this is an update action.
			if !cc.Authorize(hotline.AccessModifyUser) {
				return cc.NewErrReply(t, ErrMsgNotAllowedModifyAccounts)
			}

			// This part is a bit tricky. There are three possibilities:
			// 1) The transaction is intended to update the password.
			//	  In this case, FieldUserPassword is sent with the new password.
			// 2) The transaction is intended to remove the password.
			//    In this case, FieldUserPassword is not sent.
			// 3) The transaction updates the users access bits, but not the password.
			//    In this case, FieldUserPassword is sent with zero as the only byte.
			if hotline.GetField(hotline.FieldUserPassword, &subFields) != nil {
				newPass := hotline.GetField(hotline.FieldUserPassword, &subFields).Data
				if !bytes.Equal([]byte{0}, newPass) {
					acc.Password = hotline.HashAndSalt(newPass)
				}
			} else {
				acc.Password = hotline.HashAndSalt([]byte(""))
			}

			if hotline.GetField(hotline.FieldUserAccess, &subFields) != nil {
				copy(acc.Access[:], hotline.GetField(hotline.FieldUserAccess, &subFields).Data)
			}

			acc.Name = string(hotline.GetField(hotline.FieldUserName, &subFields).Data)

			err := cc.Server.AccountManager.Update(*acc, string(hotline.EncodeString(hotline.GetField(hotline.FieldUserLogin, &subFields).Data)))

			if err != nil {
				cc.Logger.Error("update user: update account", "err", err)
				return cc.NewErrReply(t, ErrMsgUpdateAccount)
			}
		} else {
			if !cc.Authorize(hotline.AccessCreateUser) {
				return cc.NewErrReply(t, ErrMsgNotAllowedCreateAccounts)
			}

			cc.Logger.Info("CreateUser", "login", userLogin)

			var newAccess hotline.AccessBitmap
			copy(newAccess[:], hotline.GetField(hotline.FieldUserAccess, &subFields).Data)

			// Prevent account from creating new account with greater permission
			for i := 0; i < 64; i++ {
				if newAccess.IsSet(i) {
					if !cc.Authorize(i) {
						return cc.NewErrReply(t, "Cannot create account with more access than yourself.")
					}
				}
			}

			account := hotline.NewAccount(
				userLogin,
				string(hotline.GetField(hotline.FieldUserName, &subFields).Data),
				string(hotline.GetField(hotline.FieldUserPassword, &subFields).Data),
				newAccess,
			)

			err := cc.Server.AccountManager.Create(*account)
			if err != nil {
				return cc.NewErrReply(t, ErrMsgAccountExists)
			}
		}
	}

	return append(res, cc.NewReply(t))
}

// HandleNewUser adds a new user to the server's list of allowed users.
//
// Fields used in the request:
//   - 105 User login     Required
//   - 106 User password  Required
//   - 102 User name      Required
//   - 110 User access    Required - User access privileges bitmap
//
// Fields used in the reply: None
func HandleNewUser(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessCreateUser) {
		return cc.NewErrReply(t, ErrMsgNotAllowedCreateAccounts)
	}

	login := t.GetField(hotline.FieldUserLogin).DecodeObfuscatedString()

	// If the account already exists, reply with an error.
	if account := cc.Server.AccountManager.Get(login); account != nil {
		return cc.NewErrReply(t, fmt.Sprintf(ErrMsgAccountExistsTemplate, login))
	}

	var newAccess hotline.AccessBitmap
	copy(newAccess[:], t.GetField(hotline.FieldUserAccess).Data)

	// Prevent account from creating new account with greater permission
	for i := 0; i < 64; i++ {
		if newAccess.IsSet(i) {
			if !cc.Authorize(i) {
				return cc.NewErrReply(t, ErrMsgAccountMoreAccess)
			}
		}
	}

	account := hotline.NewAccount(login, string(t.GetField(hotline.FieldUserName).Data), string(t.GetField(hotline.FieldUserPassword).Data), newAccess)

	err := cc.Server.AccountManager.Create(*account)
	if err != nil {
		return cc.NewErrReply(t, ErrMsgAccountExists)
	}

	return append(res, cc.NewReply(t))
}

// HandleDeleteUser deletes the specified user from the server's list of allowed users.
//
// Fields used in the request:
//   - 105 User login  Required
//
// Fields used in the reply: None
func HandleDeleteUser(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessDeleteUser) {
		return cc.NewErrReply(t, ErrMsgNotAllowedDeleteAccounts)
	}

	login := t.GetField(hotline.FieldUserLogin).DecodeObfuscatedString()

	if err := cc.Server.AccountManager.Delete(login); err != nil {
		cc.Logger.Error("Error deleting account", "err", err)
		return cc.NewErrReply(t, ErrMsgDeleteAccount)
	}

	for _, client := range cc.Server.ClientMgr.List() {
		if client.GetAccount().Login == login {
			res = append(res,
				hotline.NewTransaction(hotline.TranServerMsg, client.ID,
					hotline.NewField(hotline.FieldData, []byte(ErrMsgAccountDeleted)),
					hotline.NewField(hotline.FieldChatOptions, []byte{2}),
				),
			)

			go func(c *hotline.ClientConn) {
				time.Sleep(2 * time.Second)
				c.Disconnect()
			}(client)
		}
	}

	return append(res, cc.NewReply(t))
}
