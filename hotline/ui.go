package hotline

import (
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
)

type UI struct {
	chatBox     *tview.TextView
	chatInput   *tview.InputField
	App         *tview.Application
	Pages       *tview.Pages
	userList    *tview.TextView
	trackerList *tview.List
	HLClient    *Client
}

// pages
const (
	pageServerUI = "serverUI"
)

func NewUI(c *Client) *UI {
	app := tview.NewApplication()
	chatBox := tview.NewTextView().
		SetScrollable(true).
		SetDynamicColors(true).
		SetWordWrap(true).
		SetChangedFunc(func() {
			app.Draw() // TODO: docs say this is bad but it's the only way to show content during initial render??
		})
	chatBox.Box.SetBorder(true).SetTitle("| Chat |")

	chatInput := tview.NewInputField()
	chatInput.
		SetLabel("> ").
		SetFieldBackgroundColor(tcell.ColorDimGray).
		SetDoneFunc(func(key tcell.Key) {
			// skip send if user hit enter with no other text
			if len(chatInput.GetText()) == 0 {
				return
			}

			_ = c.Send(
				*NewTransaction(TranChatSend, nil,
					NewField(FieldData, []byte(chatInput.GetText())),
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
	for i, srv := range ui.HLClient.Pref.Bookmarks {
		addr := srv.Addr
		login := srv.Login
		pass := srv.Password
		list.AddItem(srv.Name, srv.Addr, rune(shortcut+i), func() {
			ui.Pages.RemovePage("joinServer")

			newJS := ui.renderJoinServerForm("", addr, login, pass, "bookmarks", true, true)

			ui.Pages.AddPage("joinServer", newJS, true, true)
		})
	}

	return list
}

func (ui *UI) getTrackerList() *tview.List {
	listing, err := GetListing(ui.HLClient.Pref.Tracker)
	if err != nil {
		// TODO
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
		srvName := srv.Name
		list.AddItem(string(srv.Name), string(srv.Description), rune(shortcut+i), func() {
			ui.Pages.RemovePage("joinServer")

			newJS := ui.renderJoinServerForm(string(srvName), addr, GuestAccount, "", trackerListPage, false, true)

			ui.Pages.AddPage("joinServer", newJS, true, true)
			ui.Pages.ShowPage("joinServer")
		})
	}

	return list
}

func (ui *UI) renderSettingsForm() *tview.Flex {
	iconStr := strconv.Itoa(ui.HLClient.Pref.IconID)
	settingsForm := tview.NewForm()
	settingsForm.AddInputField("Your Name", ui.HLClient.Pref.Username, 0, nil, nil)
	settingsForm.AddInputField("IconID", iconStr, 0, func(idStr string, _ rune) bool {
		_, err := strconv.Atoi(idStr)
		return err == nil
	}, nil)
	settingsForm.AddInputField("Tracker", ui.HLClient.Pref.Tracker, 0, nil, nil)
	settingsForm.AddCheckbox("Enable Terminal Bell", ui.HLClient.Pref.EnableBell, nil)
	settingsForm.AddButton("Save", func() {
		usernameInput := settingsForm.GetFormItem(0).(*tview.InputField).GetText()
		if len(usernameInput) == 0 {
			usernameInput = "unnamed"
		}
		ui.HLClient.Pref.Username = usernameInput
		iconStr = settingsForm.GetFormItem(1).(*tview.InputField).GetText()
		ui.HLClient.Pref.IconID, _ = strconv.Atoi(iconStr)
		ui.HLClient.Pref.Tracker = settingsForm.GetFormItem(2).(*tview.InputField).GetText()
		ui.HLClient.Pref.EnableBell = settingsForm.GetFormItem(3).(*tview.Checkbox).IsChecked()

		out, err := yaml.Marshal(&ui.HLClient.Pref)
		if err != nil {
			// TODO: handle err
		}
		// TODO: handle err
		err = ioutil.WriteFile(ui.HLClient.cfgPath, out, 0666)
		if err != nil {
			println(ui.HLClient.cfgPath)
			panic(err)
		}
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

func (ui *UI) joinServer(addr, login, password string) error {
	// append default port to address if no port supplied
	if len(strings.Split(addr, ":")) == 1 {
		addr += ":5500"
	}
	if err := ui.HLClient.Connect(addr, login, password); err != nil {
		return fmt.Errorf("Error joining server: %v\n", err)
	}

	go func() {
		if err := ui.HLClient.HandleTransactions(); err != nil {
			ui.Pages.SwitchToPage("home")
		}

		loginErrModal := tview.NewModal().
			AddButtons([]string{"Ok"}).
			SetText("The server connection has closed.").
			SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				ui.Pages.SwitchToPage("home")
			})
		loginErrModal.Box.SetTitle("Server Connection Error")

		ui.Pages.AddPage("loginErr", loginErrModal, false, true)
		ui.App.Draw()
	}()

	return nil
}

func (ui *UI) renderJoinServerForm(name, server, login, password, backPage string, save, defaultConnect bool) *tview.Flex {
	joinServerForm := tview.NewForm()
	joinServerForm.
		//	AddInputField("Name", server, 0, func(textToCheck string, lastChar rune) bool {
		//	return false
		// }, nil).
		AddInputField("Server", server, 0, nil, nil).
		AddInputField("Login", login, 0, nil, nil).
		AddPasswordField("Password", password, 0, '*', nil).
		AddCheckbox("Save", save, func(checked bool) {
			ui.HLClient.Logger.Infow("saving bookmark")
			// TODO: Implement bookmark saving

			ui.HLClient.Pref.AddBookmark(joinServerForm.GetFormItem(0).(*tview.InputField).GetText(), joinServerForm.GetFormItem(0).(*tview.InputField).GetText(), joinServerForm.GetFormItem(1).(*tview.InputField).GetText(), joinServerForm.GetFormItem(2).(*tview.InputField).GetText())
			out, err := yaml.Marshal(ui.HLClient.Pref)
			if err != nil {
				panic(err)
			}

			err = ioutil.WriteFile(ui.HLClient.cfgPath, out, 0666)
			if err != nil {
				panic(err)
			}
			// 		Pref := ui.HLClient.Pref
		}).
		AddButton("Cancel", func() {
			ui.Pages.SwitchToPage(backPage)
		}).
		AddButton("Connect", func() {
			srvAddr := joinServerForm.GetFormItem(0).(*tview.InputField).GetText()
			loginInput := joinServerForm.GetFormItem(1).(*tview.InputField).GetText()
			err := ui.joinServer(
				srvAddr,
				loginInput,
				joinServerForm.GetFormItem(2).(*tview.InputField).GetText(),
			)
			if name == "" {
				name = fmt.Sprintf("%s@%s", loginInput, srvAddr)
			}
			ui.HLClient.serverName = name

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

func (ui *UI) renderServerUI() *tview.Flex {
	ui.chatBox.SetText("") // clear any previously existing chatbox text
	commandList := tview.NewTextView().SetDynamicColors(true)
	commandList.
		SetText("[yellow]^n[-::]: Read News   [yellow]^p[-::]: Post News\n[yellow]^l[-::]: View Logs   [yellow]^f[-::]: View Files\n").
		SetBorder(true).
		SetTitle("| Keyboard Shortcuts| ")

	modal := tview.NewModal().
		SetText("Disconnect from the server?").
		AddButtons([]string{"Cancel", "Exit"}).
		SetFocus(1)
	modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
		if buttonIndex == 1 {
			_ = ui.HLClient.Disconnect()
			ui.Pages.RemovePage(pageServerUI)
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
	serverUI.SetBorder(true).SetTitle("| Mobius - Connected to " + ui.HLClient.serverName + " |").SetTitleAlign(tview.AlignLeft)
	serverUI.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			ui.Pages.AddPage("modal", modal, false, true)
		}

		// List files
		if event.Key() == tcell.KeyCtrlF {
			if err := ui.HLClient.Send(*NewTransaction(TranGetFileNameList, nil)); err != nil {
				ui.HLClient.Logger.Errorw("err", "err", err)
			}
		}

		// Show News
		if event.Key() == tcell.KeyCtrlN {
			if err := ui.HLClient.Send(*NewTransaction(TranGetMsgs, nil)); err != nil {
				ui.HLClient.Logger.Errorw("err", "err", err)
			}
		}

		// Post news
		if event.Key() == tcell.KeyCtrlP {

			newsFlex := tview.NewFlex()
			newsFlex.SetBorderPadding(0, 0, 1, 1)
			newsPostTextArea := tview.NewTextView()
			newsPostTextArea.SetBackgroundColor(tcell.ColorDarkSlateGrey)
			newsPostTextArea.SetChangedFunc(func() {
				ui.App.Draw() // TODO: docs say this is bad but it's the only way to show content during initial render??
			})

			newsPostForm := tview.NewForm().
				SetButtonsAlign(tview.AlignRight).
				// AddButton("Cancel", nil). // TODO: implement cancel button behavior
				AddButton("Send", nil)
			newsPostForm.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
				switch event.Key() {
				case tcell.KeyEscape:
					ui.Pages.RemovePage("newsInput")
				case tcell.KeyTab:
					ui.App.SetFocus(newsPostTextArea)
				case tcell.KeyEnter:
					newsText := strings.ReplaceAll(newsPostTextArea.GetText(true), "\n", "\r")
					if len(newsText) == 0 {
						return event
					}
					err := ui.HLClient.Send(
						*NewTransaction(TranOldPostNews, nil,
							NewField(FieldData, []byte(newsText)),
						),
					)
					if err != nil {
						ui.HLClient.Logger.Errorw("Error posting news", "err", err)
						// TODO: display errModal to user
					}
					ui.Pages.RemovePage("newsInput")
				}

				return event
			})

			newsFlex.
				SetDirection(tview.FlexRow).
				SetBorder(true).
				SetTitle("| Post Message |")

			newsPostTextArea.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
				switch event.Key() {
				case tcell.KeyEscape:
					ui.Pages.RemovePage("newsInput")
				case tcell.KeyTab:
					ui.App.SetFocus(newsPostForm)
				case tcell.KeyEnter:
					_, _ = fmt.Fprintf(newsPostTextArea, "\n")
				default:
					const windowsBackspaceRune = 8
					const macBackspaceRune = 127
					switch event.Rune() {
					case macBackspaceRune, windowsBackspaceRune:
						curTxt := newsPostTextArea.GetText(true)
						if len(curTxt) > 0 {
							curTxt = curTxt[:len(curTxt)-1]
							newsPostTextArea.SetText(curTxt)
						}
					default:
						_, _ = fmt.Fprintf(newsPostTextArea, string(event.Rune()))
					}
				}

				return event
			})

			newsFlex.AddItem(newsPostTextArea, 10, 0, true)
			newsFlex.AddItem(newsPostForm, 3, 0, false)

			newsPostPage := tview.NewFlex().
				AddItem(nil, 0, 1, false).
				AddItem(tview.NewFlex().
					SetDirection(tview.FlexRow).
					AddItem(nil, 0, 1, false).
					AddItem(newsFlex, 15, 1, true).
					// AddItem(newsPostForm, 3, 0, false).
					AddItem(nil, 0, 1, false), 40, 1, false).
				AddItem(nil, 0, 1, false)

			ui.Pages.AddPage("newsInput", newsPostPage, true, true)
			ui.App.SetFocus(newsPostTextArea)
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
		14, 1, false)
	home.AddItem(tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(mainMenu, 0, 1, true).
		AddItem(nil, 0, 1, false),
		0, 1, true,
	)

	mainMenu.AddItem("Join Server", "", 'j', func() {
		joinServerPage := ui.renderJoinServerForm("", "", GuestAccount, "", "home", false, false)
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
			ui.HLClient.DebugBuf.TextView.ScrollToEnd()
			ui.HLClient.DebugBuf.TextView.SetBorder(true).SetTitle("Logs")
			ui.HLClient.DebugBuf.TextView.SetDoneFunc(func(key tcell.Key) {
				if key == tcell.KeyEscape {
					ui.Pages.RemovePage("logs")
				}
			})

			ui.Pages.AddPage("logs", ui.HLClient.DebugBuf.TextView, true, true)
		}
		return event
	})

	if err := ui.App.SetRoot(ui.Pages, true).SetFocus(ui.Pages).Run(); err != nil {
		ui.App.Stop()
		os.Exit(1)
	}
}
