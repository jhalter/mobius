package hotline

import (
	"bytes"
	"fmt"
	"github.com/google/go-cmp/cmp"
	"io/ioutil"
	"net"
	"strings"
	"sync"
	"testing"
)

type transactionTest struct {
	description string      // Human understandable description
	account     Account     // Account struct for a user that will test transaction will execute under
	request     Transaction // transaction that will be sent by the client to the server
	want        Transaction // transaction that the client expects to receive in response
	setup       func()      // Optional setup required for the test scenario
	teardown    func()      // Optional teardown for test scenario
}

func (tt *transactionTest) Setup(srv *Server) error {
	if err := srv.NewUser(tt.account.Login, tt.account.Name, NegatedUserString([]byte(tt.account.Password)), tt.account.Access); err != nil {
		return err
	}

	if tt.setup != nil {
		tt.setup()
	}

	return nil
}

func (tt *transactionTest) Teardown(srv *Server) error {
	if err := srv.DeleteUser(tt.account.Login); err != nil {
		return err
	}

	if tt.teardown != nil {
		tt.teardown()
	}

	return nil
}

// StartTestServer
func StartTestServer() (srv *Server, lnPort int) {
	hotlineServer, _ := NewServer("test/config/")
	ln, err := net.Listen("tcp", ":0")

	if err != nil {
		panic(err)
	}
	go func() {
		for {
			conn, _ := ln.Accept()
			go hotlineServer.HandleConnection(conn)
		}
	}()
	return hotlineServer, ln.Addr().(*net.TCPAddr).Port
}

func StartTestClient(serverPort int, login, passwd string) (*Client, error) {
	c := NewClient("")

	err := c.JoinServer(fmt.Sprintf(":%v", serverPort), login, passwd)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func StartTestServerWithClients(clientCount int) ([]*Client, int) {
	_, serverPort := StartTestServer()

	var clients []*Client
	for i := 0; i < clientCount; i++ {
		client, err := StartTestClient(serverPort, "admin", "")
		if err != nil {
			panic(err)
		}
		clients = append(clients, client)
	}
	clients[0].ReadN(2)

	return clients, serverPort
}

func TestHandshake(t *testing.T) {
	_, port := StartTestServer()

	conn, err := net.Dial("tcp", fmt.Sprintf(":%v", port))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	conn.Write([]byte{84, 82, 84, 80, 0, 0, 0, 0})

	replyBuf := make([]byte, 8)
	_, err = conn.Read(replyBuf)
	if err != nil {
		panic(err)
	}

	want := []byte{84, 82, 84, 80, 0, 0, 0, 0}
	if bytes.Compare(replyBuf, want) != 0 {
		t.Errorf("%q, want %q", replyBuf, want)
	}
	return
}

//func TestHandleTranAgreed(t *testing.T) {
//	clients, _ := StartTestServerWithClients(2)
//
//	chatMsg := "Test Chat"
//
//	// Assert that both clients should receive the user join notification
//	var wg sync.WaitGroup
//	for _, client := range clients {
//		wg.Add(1)
//		go func(wg *sync.WaitGroup, c *Client) {
//			defer wg.Done()
//
//			receivedMsg := c.ReadTransactions()[0].GetField(fieldData).Data
//
//			want := []byte(fmt.Sprintf("test: %s\r", chatMsg))
//			if bytes.Compare(receivedMsg, want) != 0 {
//				t.Errorf("%q, want %q", receivedMsg, want)
//			}
//		}(&wg, client)
//	}
//
//	trans := clients[1].ReadTransactions()
//	spew.Dump(trans)
//
//	// Send the agreement
//	clients[1].Connection.Write(
//		NewTransaction(
//			tranAgreed, 0,
//			[]Field{
//				NewField(fieldUserName, []byte("testUser")),
//				NewField(fieldUserIconID, []byte{0x00,0x07}),
//			},
//		).Payload(),
//	)
//
//	wg.Wait()
//}

func TestChatSend(t *testing.T) {
	//srvPort := StartTestServer()
	//
	//senderClient := NewClient("senderClient")
	//senderClient.JoinServer(fmt.Sprintf(":%v", srvPort), "", "")
	//
	//receiverClient := NewClient("receiverClient")
	//receiverClient.JoinServer(fmt.Sprintf(":%v", srvPort), "", "")

	clients, _ := StartTestServerWithClients(2)

	chatMsg := "Test Chat"

	// Both clients should receive the chatMsg
	var wg sync.WaitGroup
	for _, client := range clients {
		wg.Add(1)
		go func(wg *sync.WaitGroup, c *Client) {
			defer wg.Done()

			receivedMsg := c.ReadTransactions()[0].GetField(fieldData).Data

			want := []byte(fmt.Sprintf("         test:  %s\r", chatMsg))
			if bytes.Compare(receivedMsg, want) != 0 {
				t.Errorf("%q, want %q", receivedMsg, want)
			}
		}(&wg, client)
	}

	// Send the chatMsg
	clients[1].Send(
		NewTransaction(
			tranChatSend, 0,
			[]Field{
				NewField(fieldData, []byte(chatMsg)),
			},
		),
	)

	wg.Wait()
}

func TestSetClientUserInfo(t *testing.T) {
	clients, _ := StartTestServerWithClients(2)

	newIcon := []byte{0x00, 0x01}
	newUserName := "newName"

	// Both clients should receive the chatMsg
	var wg sync.WaitGroup
	for _, client := range clients {
		wg.Add(1)
		go func(wg *sync.WaitGroup, c *Client) {
			defer wg.Done()

			tran := c.ReadTransactions()[0]

			want := []byte(newUserName)
			got := tran.GetField(fieldUserName).Data
			if bytes.Compare(got, want) != 0 {
				t.Errorf("%q, want %q", got, want)
			}
		}(&wg, client)
	}

	_, err := clients[1].Connection.Write(
		NewTransaction(
			tranSetClientUserInfo, 0,
			[]Field{
				NewField(fieldUserIconID, newIcon),
				NewField(fieldUserName, []byte(newUserName)),
			},
		).Payload(),
	)
	if err != nil {
		t.Errorf("%v", err)
	}

	wg.Wait()
}

// TestSendInstantMsg tests that client A can send an instant message to client B
//
func TestSendInstantMsg(t *testing.T) {
	clients, _ := StartTestServerWithClients(2)

	instantMsg := "Test IM"

	var wg sync.WaitGroup
	wg.Add(1)
	go func(wg *sync.WaitGroup, c *Client) {
		defer wg.Done()

		tran := c.WaitForTransaction(tranServerMsg)

		receivedMsg := tran.GetField(fieldData).Data
		want := []byte(fmt.Sprintf("%s", instantMsg))
		if bytes.Compare(receivedMsg, want) != 0 {
			t.Errorf("%q, want %q", receivedMsg, want)
		}
	}(&wg, clients[0])

	_ = clients[1].Send(
		NewTransaction(tranGetUserNameList, 0, []Field{}),
	)
	//connectedUsersTran := clients[1].ReadTransactions()[0]
	////connectedUsers := connectedUsersTran.Fields[0].Data[0:2]
	//spew.Dump(connectedUsersTran.Fields)
	//firstUserID := connectedUsersTran.Fields[0].Data[0:2]
	//
	//spew.Dump(firstUserID)

	// Send the IM
	err := clients[1].Send(
		NewTransaction(
			tranSendInstantMsg, 0,
			[]Field{
				NewField(fieldData, []byte(instantMsg)),
				NewField(fieldUserName, clients[1].UserName),
				NewField(fieldUserID, []byte{0, 2}),
				NewField(fieldOptions, []byte{0, 1}),
			},
		),
	)
	if err != nil {
		t.Error(err)
	}

	wg.Wait()
}

func TestOldPostNews(t *testing.T) {
	clients, _ := StartTestServerWithClients(2)

	newsPost := "Test News Post"

	var wg sync.WaitGroup
	wg.Add(1)
	go func(wg *sync.WaitGroup, c *Client) {
		defer wg.Done()

		receivedMsg := c.ReadTransactions()[0].GetField(fieldData).Data

		if strings.Contains(string(receivedMsg), newsPost) == false {
			t.Errorf("news post missing")
		}
	}(&wg, clients[0])

	clients[1].Connection.Write(
		NewTransaction(
			tranOldPostNews, 0,
			[]Field{
				NewField(fieldData, []byte(newsPost)),
			},
		).Payload(),
	)

	wg.Wait()
}

// TODO: Fixme
//func TestGetFileNameList(t *testing.T) {
//	clients, _ := StartTestServerWithClients(2)
//
//	clients[0].Connection.Write(
//		NewTransaction(
//			tranGetFileNameList, 0,
//			[]Field{},
//		).Payload(),
//	)
//
//	ts := clients[0].ReadTransactions()
//	testfileSit := ReadFileNameWithInfo(ts[0].Fields[1].Data)
//
//	want := "testfile.sit"
//	got := testfileSit.Name
//	diff := cmp.Diff(want, got)
//	if diff != "" {
//		t.Fatalf(diff)
//	}
//	if testfileSit.Name != "testfile.sit" {
//		t.Errorf("news post missing")
//		t.Errorf("%q, want %q", testfileSit.Name, "testfile.sit")
//	}
//}

func TestNewsCategoryList(t *testing.T) {
	clients, _ := StartTestServerWithClients(2)
	client := clients[0]

	client.Send(
		NewTransaction(
			tranGetNewsCatNameList, 0,
			[]Field{},
		),
	)

	ts := client.ReadTransactions()
	cats := ts[0].GetFields(fieldNewsCatListData15)

	newsCat := ReadNewsCategoryListData(cats[0].Data)
	want := "TestBundle"
	got := newsCat.Name
	diff := cmp.Diff(want, got)
	if diff != "" {
		t.Fatalf(diff)
	}

	newsBundle := ReadNewsCategoryListData(cats[1].Data)
	want = "TestCat"
	got = newsBundle.Name
	diff = cmp.Diff(want, got)
	if diff != "" {
		t.Fatalf(diff)
	}
}

func TestNestedNewsCategoryList(t *testing.T) {
	clients, _ := StartTestServerWithClients(2)
	client := clients[0]
	newsPath := NewsPath{
		[]string{
			"TestBundle",
			"NestedBundle",
		},
	}

	_, err := client.Connection.Write(
		NewTransaction(
			tranGetNewsCatNameList, 0,
			[]Field{
				NewField(
					fieldNewsPath,
					newsPath.Payload(),
				),
			},
		).Payload(),
	)
	if err != nil {
		t.Errorf("%v", err)
	}

	ts := client.ReadTransactions()
	cats := ts[0].GetFields(fieldNewsCatListData15)

	newsCat := ReadNewsCategoryListData(cats[0].Data)
	want := "NestedCat"
	got := newsCat.Name
	diff := cmp.Diff(want, got)
	if diff != "" {
		t.Fatalf(diff)
	}
}

func TestFileDownload(t *testing.T) {
	clients, _ := StartTestServerWithClients(2)
	client := clients[0]

	type want struct {
		fileSize     []byte
		transferSize []byte
		waitingCount []byte
		refNum       []byte
	}
	var tests = []struct {
		fileName string
		want     want
	}{
		{
			fileName: "testfile.sit",
			want: want{
				fileSize:     []byte{0x0, 0x0, 0x0, 0x13},
				transferSize: []byte{0x0, 0x0, 0x0, 0xa1},
			},
		},
		{
			fileName: "testfile.txt",
			want: want{
				fileSize:     []byte{0x0, 0x0, 0x0, 0x17},
				transferSize: []byte{0x0, 0x0, 0x0, 0xa5},
			},
		},
	}

	for _, test := range tests {
		_, err := client.Connection.Write(
			NewTransaction(
				tranDownloadFile, 0,
				[]Field{
					NewField(fieldFileName, []byte(test.fileName)),
					NewField(fieldFilePath, []byte("")),
				},
			).Payload(),
		)
		if err != nil {
			t.Errorf("%v", err)
		}
		tran := client.ReadTransactions()[0]

		if got := tran.GetField(fieldFileSize).Data; bytes.Compare(got, test.want.fileSize) != 0 {
			t.Errorf("TestFileDownload: fileSize got %#v, want %#v", got, test.want.fileSize)
		}

		if got := tran.GetField(fieldTransferSize).Data; bytes.Compare(got, test.want.transferSize) != 0 {
			t.Errorf("TestFileDownload: fieldTransferSize: %s: got %#v, want %#v", test.fileName, got, test.want.transferSize)
		}
	}
}

func TestFileUpload(t *testing.T) {
	clients, _ := StartTestServerWithClients(2)
	client := clients[0]

	var tests = []struct {
		fileName string
		want     Transaction
	}{
		{
			fileName: "testfile.sit",
			want: Transaction{
				Fields: []Field{
					NewField(fieldRefNum, []byte{0x16, 0x3f, 0x5f, 0xf}),
				},
			},
		},
	}

	for _, test := range tests {
		err := client.Send(
			NewTransaction(
				tranUploadFile, 0,
				[]Field{
					NewField(fieldFileName, []byte(test.fileName)),
					NewField(fieldFilePath, []byte("")),
				},
			),
		)
		if err != nil {
			t.Errorf("%v", err)
		}
		tran := client.ReadTransactions()[0]

		for _, f := range test.want.Fields {
			got := tran.GetField(f.Uint16ID()).Data
			want := test.want.GetField(fieldRefNum).Data
			if bytes.Compare(got, want) != 0 {
				t.Errorf("xxx: yyy got %#v, want %#v", got, want)
			}
		}
	}
}

// TODO: Make canonical
func TestNewUser(t *testing.T) {
	srv, port := StartTestServer()

	var tests = []struct {
		description string
		setup       func()
		teardown    func()
		account     Account
		request     Transaction
		want        Transaction
	}{
		{
			description: "a valid new account",
			teardown: func() {
				_ = srv.DeleteUser("testUser")
			},
			account: Account{
				Login:    "test",
				Name:     "unnamed",
				Password: "test",
				Access:   []byte{255, 255, 255, 255, 255, 255, 255, 255},
			},
			request: NewTransaction(
				tranNewUser, 0,
				[]Field{
					NewField(fieldUserLogin, []byte(NegatedUserString([]byte("testUser")))),
					NewField(fieldUserName, []byte("testUserName")),
					NewField(fieldUserPassword, []byte(NegatedUserString([]byte("testPw")))),
					NewField(fieldUserAccess, []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}),
				},
			),
			want: Transaction{
				Fields: []Field{},
			},
		},
		{
			description: "a newUser request from a user without the required access",
			teardown: func() {
				_ = srv.DeleteUser("testUser")
			},
			account: Account{
				Login:    "test",
				Name:     "unnamed",
				Password: "test",
				Access:   []byte{0, 0, 0, 0, 0, 0, 0, 0},
			},
			request: NewTransaction(
				tranNewUser, 0,
				[]Field{
					NewField(fieldUserLogin, []byte(NegatedUserString([]byte("testUser")))),
					NewField(fieldUserName, []byte("testUserName")),
					NewField(fieldUserPassword, []byte(NegatedUserString([]byte("testPw")))),
					NewField(fieldUserAccess, []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}),
				},
			),
			want: Transaction{
				Fields: []Field{
					NewField(fieldError, []byte("You are not allowed to create new accounts.")),
				},
			},
		},
		{
			description: "a request to create a user that already exists",
			teardown: func() {
				_ = srv.DeleteUser("testUser")
			},
			account: Account{
				Login:    "test",
				Name:     "unnamed",
				Password: "test",
				Access:   []byte{255, 255, 255, 255, 255, 255, 255, 255},
			},
			request: NewTransaction(
				tranNewUser, 0,
				[]Field{
					NewField(fieldUserLogin, []byte(NegatedUserString([]byte("guest")))),
					NewField(fieldUserName, []byte("testUserName")),
					NewField(fieldUserPassword, []byte(NegatedUserString([]byte("testPw")))),
					NewField(fieldUserAccess, []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}),
				},
			),
			want: Transaction{
				Fields: []Field{
					NewField(fieldError, []byte("Cannot create account guest because there is already an account with that login.")),
				},
			},
		},
	}

	for _, test := range tests {
		if test.setup != nil {
			test.setup()
		}

		if err := srv.NewUser(test.account.Login, test.account.Name, NegatedUserString([]byte(test.account.Password)), test.account.Access); err != nil {
			t.Errorf("%v", err)
		}

		c := NewClient("")
		err := c.JoinServer(fmt.Sprintf(":%v", port), test.account.Login, test.account.Password)
		if err != nil {
			t.Errorf("login failed: %v", err)
		}

		if err := c.Send(test.request); err != nil {
			t.Errorf("%v", err)
		}

		tran := c.ReadTransactions()[0]
		for _, want := range test.want.Fields {
			got := tran.GetField(want.Uint16ID())
			if bytes.Compare(got.Data, want.Data) != 0 {
				t.Errorf("%v: field mismatch:  want: %#v got: %#v", test.description, want.Data, got.Data)
			}
		}

		srv.DeleteUser(test.account.Login)

		if test.teardown != nil {
			test.teardown()
		}
	}
}

func TestDeleteUser(t *testing.T) {
	srv, port := StartTestServer()

	var tests = []transactionTest{
		{
			description: "a deleteUser request from a user without the required access",
			account: Account{
				Login:    "test",
				Name:     "unnamed",
				Password: "test",
				Access:   []byte{0, 0, 0, 0, 0, 0, 0, 0},
			},
			request: NewTransaction(
				tranDeleteUser, 0,
				[]Field{
					NewField(fieldUserLogin, []byte(NegatedUserString([]byte("foo")))),
				},
			),
			want: Transaction{
				Fields: []Field{
					NewField(fieldError, []byte("You are not allowed to delete accounts.")),
				},
			},
		},
		{
			description: "a valid deleteUser request",
			setup: func() {
				_ = srv.NewUser("foo", "foo", "foo", []byte{0, 0, 0, 0, 0, 0, 0, 0})
			},
			account: Account{
				Login:    "test",
				Name:     "unnamed",
				Password: "test",
				Access:   []byte{255, 255, 255, 255, 255, 255, 255, 255},
			},
			request: NewTransaction(
				tranDeleteUser, 0,
				[]Field{
					NewField(fieldUserLogin, []byte(NegatedUserString([]byte("foo")))),
				},
			),
			want: Transaction{
				Fields: []Field{},
			},
		},
	}

	for _, test := range tests {
		test.Setup(srv)

		c := NewClient("")
		err := c.JoinServer(fmt.Sprintf(":%v", port), test.account.Login, test.account.Password)
		if err != nil {
			t.Errorf("login failed: %v", err)
		}

		if err := c.Send(test.request); err != nil {
			t.Errorf("%v", err)
		}

		tran := c.ReadTransactions()[0]
		for _, want := range test.want.Fields {
			got := tran.GetField(want.Uint16ID())
			if bytes.Compare(got.Data, want.Data) != 0 {
				t.Errorf("%v: field mismatch:  want: %#v got: %#v", test.description, want.Data, got.Data)
			}
		}

		test.Teardown(srv)
	}
}

func TestDeleteFile(t *testing.T) {
	srv, port := StartTestServer()

	var tests = []transactionTest{
		{
			description: "a request without the required access",
			account: Account{
				Login:    "test",
				Name:     "unnamed",
				Password: "test",
				Access:   []byte{0, 0, 0, 0, 0, 0, 0, 0},
			},
			request: NewTransaction(
				tranDeleteFile, 0,
				[]Field{
					NewField(fieldFileName, []byte("testFile")),
					NewField(fieldFilePath, []byte("")),
				},
			),
			want: Transaction{
				Fields: []Field{},
			},
		},
		{
			description: "a valid deleteFile request",
			setup: func() {
				_ = ioutil.WriteFile(srv.Config.FileRoot+"testFile", []byte{0x00}, 0666)
			},
			account: Account{
				Login:    "test",
				Name:     "unnamed",
				Password: "test",
				Access:   []byte{255, 255, 255, 255, 255, 255, 255, 255},
			},
			request: NewTransaction(
				tranDeleteFile, 0,
				[]Field{
					NewField(fieldFileName, []byte("testFile")),
					NewField(fieldFilePath, []byte("")),
				},
			),
			want: Transaction{
				Fields: []Field{},
			},
		},
		{
			description: "an invalid request for a file that does not exist",
			account: Account{
				Login:    "test",
				Name:     "unnamed",
				Password: "test",
				Access:   []byte{255, 255, 255, 255, 255, 255, 255, 255},
			},
			request: NewTransaction(
				tranDeleteFile, 0,
				[]Field{
					NewField(fieldFileName, []byte("testFile")),
					NewField(fieldFilePath, []byte("")),
				},
			),
			want: Transaction{
				Fields: []Field{
					NewField(fieldError, []byte("Cannot delete file testFile because it does not exist or cannot be found.")),
				},
			},
		},
	}

	for _, test := range tests {
		test.Setup(srv)

		c := NewClient("")

		if err := c.JoinServer(fmt.Sprintf(":%v", port), test.account.Login, test.account.Password); err != nil {
			t.Errorf("login failed: %v", err)
		}

		if err := c.Send(test.request); err != nil {
			t.Errorf("%v", err)
		}

		tran := c.ReadTransactions()[0]
		for _, want := range test.want.Fields {
			got := tran.GetField(want.Uint16ID())
			if bytes.Compare(got.Data, want.Data) != 0 {
				t.Errorf("%v: field mismatch:  want: %#v got: %#v", test.description, want.Data, got.Data)
			}
		}

		test.Teardown(srv)
	}
}
