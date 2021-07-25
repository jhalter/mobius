package hotline

import (
	"bytes"
	"embed"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"math/big"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const clientConfigPath = "/usr/local/etc/mobius-client-config.yaml"
const (
	trackerListPage = "trackerList"
)

//go:embed client/banners/*.txt
var bannerDir embed.FS

type Bookmark struct {
	Name     string `yaml:"Name"`
	Addr     string `yaml:"Addr"`
	Login    string `yaml:"Login"`
	Password string `yaml:"Password"`
}

type ClientPrefs struct {
	Username  string     `yaml:"Username"`
	IconID    int        `yaml:"IconID"`
	Bookmarks []Bookmark `yaml:"Bookmarks"`
	Tracker   string     `yaml:"Tracker"`
}

func readConfig(cfgPath string) (*ClientPrefs, error) {
	fh, err := os.Open(cfgPath)
	if err != nil {
		return nil, err
	}

	prefs := ClientPrefs{}
	decoder := yaml.NewDecoder(fh)
	decoder.SetStrict(true)
	if err := decoder.Decode(&prefs); err != nil {
		return nil, err
	}
	return &prefs, nil
}

type Client struct {
	DebugBuf    *DebugBuffer
	Connection  net.Conn
	UserName    []byte
	Login       *[]byte
	Password    *[]byte
	Icon        *[]byte
	Flags       *[]byte
	ID          *[]byte
	Version     []byte
	UserAccess  []byte
	Agreed      bool
	UserList    []User
	Logger      *zap.SugaredLogger
	activeTasks map[uint32]*Transaction

	pref *ClientPrefs

	Handlers map[uint16]clientTHandler

	UI *UI

	outbox chan *Transaction
	Inbox  chan *Transaction
}

type UI struct {
	chatBox      *tview.TextView
	chatInput    *tview.InputField
	App          *tview.Application
	Pages        *tview.Pages
	userList     *tview.TextView
	agreeModal   *tview.Modal
	trackerList  *tview.List
	settingsPage *tview.Box
	HLClient     *Client
}

func NewUI(c *Client) *UI {
	app := tview.NewApplication()
	chatBox := tview.NewTextView().
		SetScrollable(true).
		SetDynamicColors(true).
		SetWordWrap(true).
		SetChangedFunc(func() {
			app.Draw() // TODO: docs say this is bad but it's the only way to show content during initial render??
		})
	chatBox.Box.SetBorder(true).SetTitle("Chat")

	chatInput := tview.NewInputField()
	chatInput.
		SetLabel("> ").
		SetFieldBackgroundColor(tcell.ColorDimGray).
		SetDoneFunc(func(key tcell.Key) {
			// skip send if user hit enter with no other text
			if len(chatInput.GetText()) == 0 {
				return
			}

			c.Send(
				*NewTransaction(tranChatSend, nil,
					NewField(fieldData, []byte(chatInput.GetText())),
				),
			)
			chatInput.SetText("") // clear the input field after chat send
		})

	chatInput.Box.SetBorder(true).SetTitle("Send")

	userList := tview.
		NewTextView().
		SetDynamicColors(true).
		SetChangedFunc(func() {
			app.Draw() // TODO: docs say this is bad but it's the only way to show content during initial render??
		})
	userList.Box.SetBorder(true).SetTitle("Users")

	return &UI{
		App:         app,
		chatBox:     chatBox,
		Pages:       tview.NewPages(),
		chatInput:   chatInput,
		userList:    userList,
		trackerList: tview.NewList(),
		agreeModal:  tview.NewModal(),
		HLClient:    c,
	}
}

func (ui *UI) showBookmarks() *tview.List {
	list := tview.NewList()
	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc {
			ui.Pages.SwitchToPage("home")
		}
		return event
	})
	list.Box.SetBorder(true).SetTitle("| Bookmarks |")

	shortcut := 97 // rune for "a"
	for i, srv := range ui.HLClient.pref.Bookmarks {
		addr := srv.Addr
		login := srv.Login
		pass := srv.Password
		list.AddItem(srv.Name, srv.Addr, rune(shortcut+i), func() {
			ui.Pages.RemovePage("joinServer")

			newJS := ui.renderJoinServerForm(addr, login, pass, "bookmarks", true, true)

			ui.Pages.AddPage("joinServer", newJS, true, true)
		})
	}

	return list
}

func (ui *UI) getTrackerList() *tview.List {
	listing, err := GetListing(ui.HLClient.pref.Tracker)
	if err != nil {
		spew.Dump(err)
	}

	list := tview.NewList()
	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc {
			ui.Pages.SwitchToPage("home")
		}
		return event
	})
	list.Box.SetBorder(true).SetTitle("| Servers |")

	shortcut := 97 // rune for "a"
	for i, srv := range listing {
		addr := srv.Addr()
		list.AddItem(string(srv.Name), string(srv.Description), rune(shortcut+i), func() {
			ui.Pages.RemovePage("joinServer")

			newJS := ui.renderJoinServerForm(addr, GuestAccount, "", trackerListPage, false, true)

			ui.Pages.AddPage("joinServer", newJS, true, true)
			ui.Pages.ShowPage("joinServer")
		})
	}

	return list
}

func (ui *UI) renderSettingsForm() *tview.Flex {
	iconStr := strconv.Itoa(ui.HLClient.pref.IconID)
	settingsForm := tview.NewForm()
	settingsForm.AddInputField("Your Name", ui.HLClient.pref.Username, 0, nil, nil)
	settingsForm.AddInputField("IconID",iconStr, 0, func(idStr string, _ rune) bool {
		_, err := strconv.Atoi(idStr)
		return err == nil
	}, nil)
	settingsForm.AddInputField("Tracker", ui.HLClient.pref.Tracker, 0, nil, nil)
	settingsForm.AddButton("Save", func() {
		ui.HLClient.pref.Username = settingsForm.GetFormItem(0).(*tview.InputField).GetText()
		iconStr = settingsForm.GetFormItem(1).(*tview.InputField).GetText()
		ui.HLClient.pref.IconID, _ = strconv.Atoi(iconStr)
		ui.HLClient.pref.Tracker = settingsForm.GetFormItem(2).(*tview.InputField).GetText()

		out, err := yaml.Marshal(&ui.HLClient.pref)
		if err != nil {
			// TODO: handle err
		}
		// TODO: handle err
		_ = ioutil.WriteFile(clientConfigPath, out, 0666)
		ui.Pages.RemovePage("settings")
	})
	settingsForm.SetBorder(true)
	settingsForm.SetCancelFunc(func() {
		ui.Pages.RemovePage("settings")
	})
	settingsPage := tview.NewFlex().SetDirection(tview.FlexRow)
	settingsPage.Box.SetBorder(true).SetTitle("Settings")
	settingsPage.AddItem(settingsForm, 0, 1, true)

	centerFlex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(settingsForm, 15, 1, true).
			AddItem(nil, 0, 1, false), 40, 1, true).
		AddItem(nil, 0, 1, false)

	return centerFlex
}

var (
	srvIP    string
	srvLogin string
	srvPass  string
)

// DebugBuffer wraps a *tview.TextView and adds a Sync() method to make it available as a Zap logger
type DebugBuffer struct {
	TextView *tview.TextView
}

func (db *DebugBuffer) Write(p []byte) (int, error) {
	return db.TextView.Write(p)
}

// Sync is a noop function that exists to satisfy the zapcore.WriteSyncer interface
func (db *DebugBuffer) Sync() error {
	return nil
}

func (ui *UI) joinServer(addr, login, password string) error {
	if err := ui.HLClient.JoinServer(addr, login, password); err != nil {
		return errors.New(fmt.Sprintf("Error joining server: %v\n", err))
	}

	go func() {
		err := ui.HLClient.ReadLoop()
		if err != nil {
			ui.HLClient.Logger.Errorw("read error", "err", err)
		}
	}()
	return nil
}

func (ui *UI) renderJoinServerForm(server, login, password, backPage string, save, defaultConnect bool) *tview.Flex {
	srvIP = server
	joinServerForm := tview.NewForm()
	joinServerForm.
		AddInputField("Server", server, 20, nil, func(text string) {
			srvIP = text
		}).
		AddInputField("Login", login, 20, nil, func(text string) {
			l := []byte(text)
			ui.HLClient.Login = &l
		}).
		AddPasswordField("Password", password, 20, '*', nil).
		AddCheckbox("Save", save, func(checked bool) {
			// TODO
		}).
		AddButton("Cancel", func() {
			ui.Pages.SwitchToPage(backPage)
		}).
		AddButton("Connect", func() {
			err := ui.joinServer(
				joinServerForm.GetFormItem(0).(*tview.InputField).GetText(),
				joinServerForm.GetFormItem(1).(*tview.InputField).GetText(),
				joinServerForm.GetFormItem(2).(*tview.InputField).GetText(),
			)
			if err != nil {
				ui.HLClient.Logger.Errorw("login error", "err", err)
				loginErrModal := tview.NewModal().
					AddButtons([]string{"Oh no"}).
					SetText(err.Error()).
					SetDoneFunc(func(buttonIndex int, buttonLabel string) {
						ui.Pages.SwitchToPage(backPage)
					})

				ui.Pages.AddPage("loginErr", loginErrModal, false, true)
			}

			// Save checkbox
			if joinServerForm.GetFormItem(3).(*tview.Checkbox).IsChecked() {
				// TODO: implement bookmark saving
			}
		})

	joinServerForm.Box.SetBorder(true).SetTitle("| Connect |")
	joinServerForm.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			ui.Pages.SwitchToPage(backPage)
		}
		return event
	})

	if defaultConnect {
		joinServerForm.SetFocus(5)
	}

	joinServerPage := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(joinServerForm, 14, 1, true).
			AddItem(nil, 0, 1, false), 40, 1, true).
		AddItem(nil, 0, 1, false)

	return joinServerPage
}

func randomBanner() string {
	rand.Seed(time.Now().UnixNano())

	bannerFiles, _ := bannerDir.ReadDir("client/banners")
	file, _ := bannerDir.ReadFile("client/banners/" + bannerFiles[rand.Intn(len(bannerFiles))].Name())

	return fmt.Sprintf("\n\n\nWelcome to...\n\n[red::b]%s[-:-:-]\n\n", file)
}

func (ui *UI) renderServerUI() *tview.Flex {
	commandList := tview.NewTextView().SetDynamicColors(true)
	commandList.
		SetText("[yellow]^n[-::]: Read News\n[yellow]^l[-::]: View Logs\n").
		SetBorder(true).
		SetTitle("Keyboard Shortcuts")

	modal := tview.NewModal().
		SetText("Disconnect from the server?").
		AddButtons([]string{"Cancel", "Exit"}).
		SetFocus(1)
	modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
		if buttonIndex == 1 {
			_ = ui.HLClient.Disconnect()
			ui.Pages.SwitchToPage("home")
		} else {
			ui.Pages.HidePage("modal")
		}
	})

	serverUI := tview.NewFlex().
		AddItem(tview.NewFlex().
			SetDirection(tview.FlexRow).
			AddItem(commandList, 4, 0, false).
			AddItem(ui.chatBox, 0, 8, false).
			AddItem(ui.chatInput, 3, 0, true), 0, 1, true).
		AddItem(ui.userList, 25, 1, false)
	serverUI.SetBorder(true).SetTitle("| Mobius - Connected to " + "TODO" + " |").SetTitleAlign(tview.AlignLeft)
	serverUI.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			ui.Pages.AddPage("modal", modal, false, true)
		}

		// Show News
		if event.Key() == tcell.KeyCtrlN {
			if err := ui.HLClient.Send(*NewTransaction(tranGetMsgs, nil)); err != nil {
				ui.HLClient.Logger.Errorw("err", "err", err)
			}
		}

		return event
	})
	return serverUI
}

func (ui *UI) Start() {
	home := tview.NewFlex().SetDirection(tview.FlexRow)
	home.Box.SetBorder(true).SetTitle("| Mobius v" + VERSION + " |").SetTitleAlign(tview.AlignLeft)
	mainMenu := tview.NewList()

	bannerItem := tview.NewTextView().
		SetText(randomBanner()).
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	home.AddItem(
		tview.NewFlex().AddItem(bannerItem, 0, 1, false),
		13, 1, false)
	home.AddItem(tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(mainMenu, 0, 1, true).
		AddItem(nil, 0, 1, false),
		0, 1, true,
	)

	mainMenu.AddItem("Join Server", "", 'j', func() {
		joinServerPage := ui.renderJoinServerForm("", GuestAccount, "", "home", false, false)
		ui.Pages.AddPage("joinServer", joinServerPage, true, true)
	}).
		AddItem("Bookmarks", "", 'b', func() {
			ui.Pages.AddAndSwitchToPage("bookmarks", ui.showBookmarks(), true)
		}).
		AddItem("Browse Tracker", "", 't', func() {
			ui.trackerList = ui.getTrackerList()
			ui.Pages.AddAndSwitchToPage("trackerList", ui.trackerList, true)
		}).
		AddItem("Settings", "", 's', func() {
			//ui.Pages.AddPage("settings", ui.renderSettingsForm(), true, false)

			ui.Pages.AddPage("settings", ui.renderSettingsForm(), true, true)
		}).
		AddItem("Quit", "", 'q', func() {
			ui.App.Stop()
		})

	ui.Pages.AddPage("home", home, true, true)

	// App level input capture
	ui.App.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlC {
			ui.HLClient.Logger.Infow("Exiting")
			ui.App.Stop()
			os.Exit(0)
		}
		// Show Logs
		if event.Key() == tcell.KeyCtrlL {
			//curPage, _ := ui.Pages.GetFrontPage()
			ui.HLClient.DebugBuf.TextView.ScrollToEnd()
			ui.HLClient.DebugBuf.TextView.SetBorder(true).SetTitle("Logs")
			ui.HLClient.DebugBuf.TextView.SetDoneFunc(func(key tcell.Key) {
				if key == tcell.KeyEscape {
					//ui.Pages.SwitchToPage("serverUI")
					ui.Pages.RemovePage("logs")
				}
			})

			ui.Pages.AddAndSwitchToPage("logs", ui.HLClient.DebugBuf.TextView, true)
		}
		return event
	})

	if err := ui.App.SetRoot(ui.Pages, true).SetFocus(ui.Pages).Run(); err != nil {
		panic(err)
	}
}

func NewClient(username string, logger *zap.SugaredLogger) *Client {
	c := &Client{
		Icon:        &[]byte{0x07, 0xd7},
		Logger:      logger,
		activeTasks: make(map[uint32]*Transaction),
		Handlers:    clientHandlers,
	}
	c.UI = NewUI(c)

	prefs, err := readConfig(clientConfigPath)
	if err != nil {
		return c
	}
	c.pref = prefs

	return c
}

type clientTransaction struct {
	Name    string
	Handler func(*Client, *Transaction) ([]Transaction, error)
}

func (ch clientTransaction) Handle(cc *Client, t *Transaction) ([]Transaction, error) {
	return ch.Handler(cc, t)
}

type clientTHandler interface {
	Handle(*Client, *Transaction) ([]Transaction, error)
}

type mockClientHandler struct {
	mock.Mock
}

func (mh *mockClientHandler) Handle(cc *Client, t *Transaction) ([]Transaction, error) {
	args := mh.Called(cc, t)
	return args.Get(0).([]Transaction), args.Error(1)
}

var clientHandlers = map[uint16]clientTHandler{
	// Server initiated
	tranChatMsg: clientTransaction{
		Name:    "tranChatMsg",
		Handler: handleClientChatMsg,
	},
	tranLogin: clientTransaction{
		Name:    "tranLogin",
		Handler: handleClientTranLogin,
	},
	tranShowAgreement: clientTransaction{
		Name:    "tranShowAgreement",
		Handler: handleClientTranShowAgreement,
	},
	tranUserAccess: clientTransaction{
		Name:    "tranUserAccess",
		Handler: handleClientTranUserAccess,
	},
	tranGetUserNameList: clientTransaction{
		Name:    "tranGetUserNameList",
		Handler: handleClientGetUserNameList,
	},
	tranNotifyChangeUser: clientTransaction{
		Name:    "tranNotifyChangeUser",
		Handler: handleNotifyChangeUser,
	},
	tranNotifyDeleteUser: clientTransaction{
		Name:    "tranNotifyDeleteUser",
		Handler: handleNotifyDeleteUser,
	},
	tranGetMsgs: clientTransaction{
		Name:    "tranNotifyDeleteUser",
		Handler: handleGetMsgs,
	},
}

func handleGetMsgs(c *Client, t *Transaction) (res []Transaction, err error) {
	newsText := string(t.GetField(fieldData).Data)
	newsText = strings.ReplaceAll(newsText, "\r", "\n")

	newsTextView := tview.NewTextView().
		SetText(newsText).
		SetDoneFunc(func(key tcell.Key) {
			c.UI.Pages.SwitchToPage("serverUI")
			c.UI.App.SetFocus(c.UI.chatInput)
		})
	newsTextView.SetBorder(true).SetTitle("News")

	c.UI.Pages.AddPage("news", newsTextView, true, true)
	c.UI.Pages.SwitchToPage("news")
	c.UI.App.SetFocus(newsTextView)

	c.UI.App.Draw()

	return res, err
}

func handleNotifyChangeUser(c *Client, t *Transaction) (res []Transaction, err error) {
	newUser := User{
		ID:    t.GetField(fieldUserID).Data,
		Name:  string(t.GetField(fieldUserName).Data),
		Icon:  t.GetField(fieldUserIconID).Data,
		Flags: t.GetField(fieldUserFlags).Data,
	}

	// Possible cases:
	// user is new to the server
	// user is already on the server but has a new name

	var oldName string
	var newUserList []User
	updatedUser := false
	for _, u := range c.UserList {
		c.Logger.Debugw("Comparing Users", "userToUpdate", newUser.ID, "myID", u.ID, "userToUpdateName", newUser.Name, "myname", u.Name)
		if bytes.Equal(newUser.ID, u.ID) {
			oldName = u.Name
			u.Name = newUser.Name
			if u.Name != newUser.Name {
				_, _ = fmt.Fprintf(c.UI.chatBox, " <<< "+oldName+" is now known as "+newUser.Name+" >>>\n")
			}
			updatedUser = true
		}
		newUserList = append(newUserList, u)
	}

	if !updatedUser {
		newUserList = append(newUserList, newUser)
	}

	c.UserList = newUserList

	c.renderUserList()

	return res, err
}

func handleNotifyDeleteUser(c *Client, t *Transaction) (res []Transaction, err error) {
	exitUser := t.GetField(fieldUserID).Data

	var newUserList []User
	for _, u := range c.UserList {
		if !bytes.Equal(exitUser, u.ID) {
			newUserList = append(newUserList, u)
		}
	}

	c.UserList = newUserList

	c.renderUserList()

	return res, err
}

const readBuffSize = 1024000 // 1KB - TODO: what should this be?

func (c *Client) ReadLoop() error {
	tranBuff := make([]byte, 0)
	tReadlen := 0
	// Infinite loop where take action on incoming client requests until the connection is closed
	for {
		buf := make([]byte, readBuffSize)
		tranBuff = tranBuff[tReadlen:]

		readLen, err := c.Connection.Read(buf)
		if err != nil {
			return err
		}
		tranBuff = append(tranBuff, buf[:readLen]...)

		// We may have read multiple requests worth of bytes from Connection.Read.  readTransactions splits them
		// into a slice of transactions
		var transactions []Transaction
		if transactions, tReadlen, err = readTransactions(tranBuff); err != nil {
			c.Logger.Errorw("Error handling transaction", "err", err)
		}

		// iterate over all of the transactions that were parsed from the byte slice and handle them
		for _, t := range transactions {
			if err := c.HandleTransaction(&t); err != nil {
				c.Logger.Errorw("Error handling transaction", "err", err)
			}
		}
	}
}

func (c *Client) GetTransactions() error {
	tranBuff := make([]byte, 0)
	tReadlen := 0

	buf := make([]byte, readBuffSize)
	tranBuff = tranBuff[tReadlen:]

	readLen, err := c.Connection.Read(buf)
	if err != nil {
		return err
	}
	tranBuff = append(tranBuff, buf[:readLen]...)

	return nil
}

func handleClientGetUserNameList(c *Client, t *Transaction) (res []Transaction, err error) {
	var users []User
	for _, field := range t.Fields {
		u, _ := ReadUser(field.Data)
		//flagBitmap := big.NewInt(int64(binary.BigEndian.Uint16(u.Flags)))
		//if flagBitmap.Bit(userFlagAdmin) == 1 {
		//	fmt.Fprintf(UserList, "[red::b]%s[-:-:-]\n", u.Name)
		//} else {
		//	fmt.Fprintf(UserList, "%s\n", u.Name)
		//}

		users = append(users, *u)
	}
	c.UserList = users

	c.renderUserList()

	return res, err
}

func (c *Client) renderUserList() {
	c.UI.userList.Clear()
	for _, u := range c.UserList {
		flagBitmap := big.NewInt(int64(binary.BigEndian.Uint16(u.Flags)))
		if flagBitmap.Bit(userFlagAdmin) == 1 {
			fmt.Fprintf(c.UI.userList, "[red::b]%s[-:-:-]\n", u.Name)
		} else {
			fmt.Fprintf(c.UI.userList, "%s\n", u.Name)
		}
	}
}

func handleClientChatMsg(c *Client, t *Transaction) (res []Transaction, err error) {
	fmt.Fprintf(c.UI.chatBox, "%s \n", t.GetField(fieldData).Data)

	return res, err
}

func handleClientTranUserAccess(c *Client, t *Transaction) (res []Transaction, err error) {
	c.UserAccess = t.GetField(fieldUserAccess).Data

	return res, err
}

func handleClientTranShowAgreement(c *Client, t *Transaction) (res []Transaction, err error) {
	agreement := string(t.GetField(fieldData).Data)
	agreement = strings.ReplaceAll(agreement, "\r", "\n")

	c.UI.agreeModal = tview.NewModal().
		SetText(agreement).
		AddButtons([]string{"Agree", "Disagree"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonIndex == 0 {
				res = append(res,
					*NewTransaction(
						tranAgreed, nil,
						NewField(fieldUserName, []byte(c.pref.Username)),
						NewField(fieldUserIconID, *c.Icon),
						NewField(fieldUserFlags, []byte{0x00, 0x00}),
						NewField(fieldOptions, []byte{0x00, 0x00}),
					),
				)
				c.Agreed = true
				c.UI.Pages.HidePage("agreement")
				c.UI.App.SetFocus(c.UI.chatInput)
			} else {
				c.Disconnect()
				c.UI.Pages.SwitchToPage("home")
			}
		},
		)

	c.Logger.Debug("show agreement page")
	c.UI.Pages.AddPage("agreement", c.UI.agreeModal, false, true)

	c.UI.Pages.ShowPage("agreement ")

	c.UI.App.Draw()
	return res, err
}

func handleClientTranLogin(c *Client, t *Transaction) (res []Transaction, err error) {
	if !bytes.Equal(t.ErrorCode, []byte{0, 0, 0, 0}) {
		errMsg := string(t.GetField(fieldError).Data)
		errModal := tview.NewModal()
		errModal.SetText(errMsg)
		errModal.AddButtons([]string{"Oh no"})
		errModal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			c.UI.Pages.RemovePage("errModal")
		})
		c.UI.Pages.RemovePage("joinServer")
		c.UI.Pages.AddPage("errModal", errModal, false, true)

		c.UI.App.Draw() // TODO: errModal doesn't render without this.  wtf?

		c.Logger.Error(string(t.GetField(fieldError).Data))
		return nil, errors.New("login error: " + string(t.GetField(fieldError).Data))
	}
	c.UI.Pages.AddAndSwitchToPage("serverUI", c.UI.renderServerUI(), true)
	c.UI.App.SetFocus(c.UI.chatInput)

	if err := c.Send(*NewTransaction(tranGetUserNameList, nil)); err != nil {
		c.Logger.Errorw("err", "err", err)
	}
	return res, err
}

// JoinServer connects to a Hotline server and completes the login flow
func (c *Client) JoinServer(address, login, passwd string) error {
	// Establish TCP connection to server
	if err := c.connect(address); err != nil {
		return err
	}

	// Send handshake sequence
	if err := c.Handshake(); err != nil {
		return err
	}

	// Authenticate (send tranLogin 107)
	if err := c.LogIn(login, passwd); err != nil {
		return err
	}

	return nil
}

// connect establishes a connection with a Server by sending handshake sequence
func (c *Client) connect(address string) error {
	var err error
	c.Connection, err = net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return err
	}
	return nil
}

var ClientHandshake = []byte{
	0x54, 0x52, 0x54, 0x50, // TRTP
	0x48, 0x4f, 0x54, 0x4c, // HOTL
	0x00, 0x01,
	0x00, 0x02,
}

var ServerHandshake = []byte{
	0x54, 0x52, 0x54, 0x50, // TRTP
	0x00, 0x00, 0x00, 0x00, // ErrorCode
}

func (c *Client) Handshake() error {
	//Protocol ID	4	‘TRTP’	0x54 52 54 50
	//Sub-protocol ID	4		User defined
	//Version	2	1	Currently 1
	//Sub-version	2		User defined
	if _, err := c.Connection.Write(ClientHandshake); err != nil {
		return fmt.Errorf("handshake write err: %s", err)
	}

	replyBuf := make([]byte, 8)
	_, err := c.Connection.Read(replyBuf)
	if err != nil {
		return err
	}

	//spew.Dump(replyBuf)
	if bytes.Compare(replyBuf, ServerHandshake) == 0 {
		return nil
	}
	// In the case of an error, client and server close the connection.

	return fmt.Errorf("handshake response err: %s", err)
}

func (c *Client) LogIn(login string, password string) error {
	return c.Send(
		*NewTransaction(
			tranLogin, nil,
			NewField(fieldUserName, []byte(c.pref.Username)),
			NewField(fieldUserIconID, []byte{0x07, 0xd1}),
			NewField(fieldUserLogin, []byte(NegatedUserString([]byte(login)))),
			NewField(fieldUserPassword, []byte(NegatedUserString([]byte(password)))),
			NewField(fieldVersion, []byte{0, 2}),
		),
	)
}

func (c *Client) Send(t Transaction) error {
	requestNum := binary.BigEndian.Uint16(t.Type)
	tID := binary.BigEndian.Uint32(t.ID)

	//handler := TransactionHandlers[requestNum]

	// if transaction is NOT reply, add it to the list to transactions we're expecting a response for
	if t.IsReply == 0 {
		c.activeTasks[tID] = &t
	}

	var n int
	var err error
	if n, err = c.Connection.Write(t.Payload()); err != nil {
		return err
	}
	c.Logger.Debugw("Sent Transaction",
		"IsReply", t.IsReply,
		"type", requestNum,
		"sentBytes", n,
	)
	return nil
}

func (c *Client) HandleTransaction(t *Transaction) error {
	var origT Transaction
	if t.IsReply == 1 {
		requestID := binary.BigEndian.Uint32(t.ID)
		origT = *c.activeTasks[requestID]
		t.Type = origT.Type
	}

	requestNum := binary.BigEndian.Uint16(t.Type)
	c.Logger.Infow(
		"Received Transaction",
		"RequestType", requestNum,
	)

	if handler, ok := c.Handlers[requestNum]; ok {
		outT, _ := handler.Handle(c, t)
		for _, t := range outT {
			c.Send(t)
		}
	} else {
		c.Logger.Errorw(
			"Unimplemented transaction type received",
			"RequestID", requestNum,
			"TransactionID", t.ID,
		)
	}

	return nil
}

func (c *Client) Connected() bool {
	fmt.Printf("Agreed: %v UserAccess: %v\n", c.Agreed, c.UserAccess)
	// c.Agreed == true &&
	if c.UserAccess != nil {
		return true
	}
	return false
}

func (c *Client) Disconnect() error {
	err := c.Connection.Close()
	if err != nil {
		return err
	}
	return nil
}
