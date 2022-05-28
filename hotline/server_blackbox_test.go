package hotline

import (
	"bytes"
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"net"
	"os"
	"testing"
)

type testCase struct {
	name        string       // test case description
	account     Account      // Account struct for a user that will test transaction will execute under
	request     *Transaction // transaction that will be sent by the client to the server
	setup       func()       // Optional test-specific setup required for the scenario
	teardown    func()       // Optional test-specific teardown for the scenario
	mockHandler map[int]*mockClientHandler
}

func (tt *testCase) Setup(srv *Server) error {
	if err := srv.NewUser(tt.account.Login, tt.account.Name, string(negateString([]byte(tt.account.Password))), *tt.account.Access); err != nil {
		return err
	}

	if tt.setup != nil {
		tt.setup()
	}

	return nil
}

func (tt *testCase) Teardown(srv *Server) error {
	if err := srv.DeleteUser(tt.account.Login); err != nil {
		return err
	}

	if tt.teardown != nil {
		tt.teardown()
	}

	return nil
}

func NewTestLogger() *zap.SugaredLogger {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "timestamp"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderCfg),
		zapcore.Lock(os.Stdout),
		zap.DebugLevel,
	)

	cores := []zapcore.Core{core}
	l := zap.New(zapcore.NewTee(cores...))
	defer func() { _ = l.Sync() }()
	return l.Sugar()
}

func StartTestServer() (*Server, context.Context, context.CancelFunc) {
	ctx, cancelRoot := context.WithCancel(context.Background())

	FS = &OSFileStore{}

	srv, err := NewServer("test/config/", "localhost", 0, NewTestLogger())
	if err != nil {
		panic(err)
	}

	go func() {
		err := srv.ListenAndServe(ctx, cancelRoot)
		if err != nil {
			panic(err)
		}
	}()

	return srv, ctx, cancelRoot
}

func TestHandshake(t *testing.T) {
	mfs := &MockFileStore{}
	fh, _ := os.Open("./test/config/Agreement.txt")
	mfs.On("Open", "/test/config/Agreement.txt").Return(fh, nil)
	fh, _ = os.Open("./test/config/config.yaml")
	mfs.On("Open", "/test/config/config.yaml").Return(fh, nil)
	FS = mfs
	spew.Dump(mfs)

	srv, _, cancelFunc := StartTestServer()
	defer cancelFunc()

	port := srv.APIListener.Addr().(*net.TCPAddr).Port

	conn, err := net.Dial("tcp", fmt.Sprintf(":%v", port))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	conn.Write([]byte{0x54, 0x52, 0x54, 0x50, 0x00, 0x01, 0x00, 0x00})

	replyBuf := make([]byte, 8)
	_, _ = conn.Read(replyBuf)

	want := []byte{84, 82, 84, 80, 0, 0, 0, 0}
	if bytes.Compare(replyBuf, want) != 0 {
		t.Errorf("%q, want %q", replyBuf, want)
	}

}

// func TestLogin(t *testing.T) {
//
//	tests := []struct {
//		name   string
//		client *Client
//	}{
//		{
//			name:   "when login is successful",
//			client: NewClient("guest", NewTestLogger()),
//		},
//	}
//	for _, test := range tests {
//		t.Run(test.name, func(t *testing.T) {
//
//		})
//	}
// }

func TestNewUser(t *testing.T) {
	srv, _, _ := StartTestServer()

	tests := []testCase{
		// {
		//	name: "a valid new account",
		//	mockHandler: func() mockClientHandler {
		//		mh := mockClientHandler{}
		//		mh.On("Handle", mock.AnythingOfType("*hotline.Client"), mock.MatchedBy(func(t *Transaction) bool {
		//			println("@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@")
		//			spew.Dump(t.Type)
		//			spew.Dump(bytes.Equal(t.Type, []byte{0x01, 0x5e}))
		//			//if !bytes.Equal(t.GetField(fieldError).Data, []byte("You are not allowed to create new accounts.")) {
		//			//	return false
		//			//}
		//			return bytes.Equal(t.Type, []byte{0x01, 0x5e},
		//			)
		//		})).Return(
		//			[]Transaction{}, nil,
		//		)
		//
		//		clientHandlers[tranNewUser] = mh
		//		return mh
		//	}(),
		//	client: func() *Client {
		//		c := NewClient("testUser", NewTestLogger())
		//		return c
		//	}(),
		//	teardown: func() {
		//		_ = srv.DeleteUser("testUser")
		//	},
		//	account: Account{
		//		Login:    "test",
		//		Name:     "unnamed",
		//		Password: "test",
		//		Access:   &[]byte{255, 255, 255, 255, 255, 255, 255, 255},
		//	},
		//	request: NewTransaction(
		//		tranNewUser, nil,
		//		NewField(fieldUserLogin, []byte(NegatedUserString([]byte("testUser")))),
		//		NewField(fieldUserName, []byte("testUserName")),
		//		NewField(fieldUserPassword, []byte(NegatedUserString([]byte("testPw")))),
		//		NewField(fieldUserAccess, []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}),
		//	),
		//	want: &Transaction{
		//		Fields: []Field{},
		//	},
		// },
		// {
		//	name: "a newUser request from a user without the required access",
		//	mockHandler: func() *mockClientHandler {
		//		mh := mockClientHandler{}
		//		mh.On("Handle", mock.AnythingOfType("*hotline.Client"), mock.MatchedBy(func(t *Transaction) bool {
		//			if !bytes.Equal(t.GetField(fieldError).Data, []byte("You are not allowed to create new accounts.")) {
		//				return false
		//			}
		//			return bytes.Equal(t.Type, []byte{0x01, 0x5e})
		//		})).Return(
		//			[]Transaction{}, nil,
		//		)
		//		return &mh
		//	}(),
		//	teardown: func() {
		//		_ = srv.DeleteUser("testUser")
		//	},
		//	account: Account{
		//		Login:    "test",
		//		Name:     "unnamed",
		//		Password: "test",
		//		Access:   &[]byte{0, 0, 0, 0, 0, 0, 0, 0},
		//	},
		//	request: NewTransaction(
		//		tranNewUser, nil,
		//		NewField(fieldUserLogin, []byte(NegatedUserString([]byte("testUser")))),
		//		NewField(fieldUserName, []byte("testUserName")),
		//		NewField(fieldUserPassword, []byte(NegatedUserString([]byte("testPw")))),
		//		NewField(fieldUserAccess, []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}),
		//	),
		// },
		// {
		//	name: "when user does not have required permission",
		//	mockHandler: func() map[int]*mockClientHandler {
		//		mockHandlers := make(map[int]*mockClientHandler)
		//
		//		mh := mockClientHandler{}
		//		mh.On("Handle", mock.AnythingOfType("*hotline.Client"), mock.MatchedBy(func(t *Transaction) bool {
		//			return t.equal(Transaction{
		//				Type:      []byte{0x01, 0x5e},
		//				IsReply:   1,
		//				ErrorCode: []byte{0, 0, 0, 1},
		//				Fields: []Field{
		//					NewField(fieldError, []byte("You are not allowed to create new accounts.")),
		//				},
		//			})
		//		})).Return(
		//			[]Transaction{}, nil,
		//		)
		//		mockHandlers[tranNewUser] = &mh
		//
		//		return mockHandlers
		//	}(),
		//
		//	teardown: func() {
		//		_ = srv.DeleteUser("testUser")
		//	},
		//	account: Account{
		//		Login:    "test",
		//		Name:     "unnamed",
		//		Password: "test",
		//		Access:   &[]byte{0, 0, 0, 0, 0, 0, 0, 0},
		//	},
		//	request: NewTransaction(
		//		tranNewUser, nil,
		//		NewField(fieldUserLogin, []byte(NegatedUserString([]byte("testUser")))),
		//		NewField(fieldUserName, []byte("testUserName")),
		//		NewField(fieldUserPassword, []byte(NegatedUserString([]byte("testPw")))),
		//		NewField(fieldUserAccess, []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}),
		//	),
		// },

		// {
		//	name: "a request to create a user that already exists",
		//	setup: func() {
		//
		//	},
		//	teardown: func() {
		//		_ = srv.DeleteUser("testUser")
		//	},
		//	account: Account{
		//		Login:    "test",
		//		Name:     "unnamed",
		//		Password: "test",
		//		Access:   &[]byte{255, 255, 255, 255, 255, 255, 255, 255},
		//	},
		//	request: NewTransaction(
		//		tranNewUser, nil,
		//		NewField(fieldUserLogin, []byte(NegatedUserString([]byte("guest")))),
		//		NewField(fieldUserName, []byte("testUserName")),
		//		NewField(fieldUserPassword, []byte(NegatedUserString([]byte("testPw")))),
		//		NewField(fieldUserAccess, []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}),
		//	),
		//	want: &Transaction{
		//		Fields: []Field{
		//			NewField(fieldError, []byte("Cannot create account guest because there is already an account with that login.")),
		//		},
		//	},
		// },
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.Setup(srv)

			// move to Setup?
			c := NewClient(test.account.Name, NewTestLogger())
			err := c.JoinServer(fmt.Sprintf(":%v", srv.APIPort()), test.account.Login, test.account.Password)
			if err != nil {
				t.Errorf("login failed: %v", err)
			}
			// end move to Setup??

			for key, value := range test.mockHandler {
				c.Handlers[uint16(key)] = value
			}

			// send test case request
			_ = c.Send(*test.request)

			// time.Sleep(1 * time.Second)
			// ===

			transactions, _ := readN(c.Connection, 1)
			for _, t := range transactions {
				_ = c.HandleTransaction(&t)
			}

			// ===

			for _, handler := range test.mockHandler {
				handler.AssertExpectations(t)
			}

			test.Teardown(srv)
		})
	}
}

// tranAssertEqual compares equality of transactions slices after stripping out the random ID
func tranAssertEqual(t *testing.T, tran1, tran2 []Transaction) bool {
	var newT1 []Transaction
	var newT2 []Transaction
	for _, trans := range tran1 {
		trans.ID = []byte{0, 0, 0, 0}
		newT1 = append(newT1, trans)
	}

	for _, trans := range tran2 {
		trans.ID = []byte{0, 0, 0, 0}
		newT2 = append(newT2, trans)

	}

	return assert.Equal(t, newT1, newT2)
}
