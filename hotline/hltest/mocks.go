// Package hltest provides testify mocks for the hotline package's manager interfaces, for use by
// tests in other packages (which cannot import another package's _test.go files). Keeping them
// here rather than in the hotline package itself keeps testify out of the production dependency
// graph of importers and the mocks out of the library's API surface.
//
// The hotline package's own in-package tests cannot import this package (it would create an import
// cycle through hotline) and instead use private copies in hotline/mocks_test.go.
package hltest

import (
	"io"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/mock"
)

type MockFileStore struct {
	mock.Mock
}

var _ hotline.FileStore = (*MockFileStore)(nil)

func (mfs *MockFileStore) Mkdir(name string, perm fs.FileMode) error {
	args := mfs.Called(name, perm)
	return args.Error(0)
}

func (mfs *MockFileStore) Stat(name string) (fs.FileInfo, error) {
	args := mfs.Called(name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(fs.FileInfo), args.Error(1)
}

func (mfs *MockFileStore) Open(name string) (io.ReadCloser, error) {
	args := mfs.Called(name)
	f, _ := args.Get(0).(io.ReadCloser)
	return f, args.Error(1)
}

func (mfs *MockFileStore) ReadDir(name string) ([]fs.DirEntry, error) {
	args := mfs.Called(name)
	entries, _ := args.Get(0).([]fs.DirEntry)
	return entries, args.Error(1)
}

func (mfs *MockFileStore) ReadLink(name string) (string, error) {
	args := mfs.Called(name)
	return args.String(0), args.Error(1)
}

func (mfs *MockFileStore) Walk(root string, fn filepath.WalkFunc) error {
	args := mfs.Called(root, fn)
	return args.Error(0)
}

func (mfs *MockFileStore) OpenFile(name string, flag int, perm fs.FileMode) (io.WriteCloser, error) {
	args := mfs.Called(name, flag, perm)
	f, _ := args.Get(0).(io.WriteCloser)
	return f, args.Error(1)
}

func (mfs *MockFileStore) Symlink(oldname, newname string) error {
	args := mfs.Called(oldname, newname)
	return args.Error(0)
}

func (mfs *MockFileStore) RemoveAll(name string) error {
	args := mfs.Called(name)
	return args.Error(0)
}

func (mfs *MockFileStore) Remove(name string) error {
	args := mfs.Called(name)
	return args.Error(0)
}

func (mfs *MockFileStore) Create(name string) (io.WriteCloser, error) {
	args := mfs.Called(name)
	f, _ := args.Get(0).(io.WriteCloser)
	return f, args.Error(1)
}

func (mfs *MockFileStore) WriteFile(name string, data []byte, perm fs.FileMode) error {
	args := mfs.Called(name, data, perm)
	return args.Error(0)
}

func (mfs *MockFileStore) Rename(oldpath, newpath string) error {
	args := mfs.Called(oldpath, newpath)
	return args.Error(0)
}

func (mfs *MockFileStore) ReadFile(name string) ([]byte, error) {
	args := mfs.Called(name)
	return args.Get(0).([]byte), args.Error(1)
}

type MockFileInfo struct {
	mock.Mock
}

var _ fs.FileInfo = (*MockFileInfo)(nil)

func (mfi *MockFileInfo) Name() string {
	args := mfi.Called()
	return args.String(0)
}

func (mfi *MockFileInfo) Size() int64 {
	args := mfi.Called()
	return args.Get(0).(int64)
}

func (mfi *MockFileInfo) Mode() fs.FileMode {
	args := mfi.Called()
	return args.Get(0).(fs.FileMode)
}

func (mfi *MockFileInfo) ModTime() time.Time {
	_ = mfi.Called()
	return time.Now()
}

func (mfi *MockFileInfo) IsDir() bool {
	args := mfi.Called()
	return args.Bool(0)
}

func (mfi *MockFileInfo) Sys() interface{} {
	_ = mfi.Called()
	return nil
}

type MockClientMgr struct {
	mock.Mock
}

var _ hotline.ClientManager = (*MockClientMgr)(nil)

func (m *MockClientMgr) List() []*hotline.ClientConn {
	args := m.Called()

	return args.Get(0).([]*hotline.ClientConn)
}

func (m *MockClientMgr) Get(id hotline.ClientID) *hotline.ClientConn {
	args := m.Called(id)

	return args.Get(0).(*hotline.ClientConn)
}

func (m *MockClientMgr) Add(cc *hotline.ClientConn) {
	m.Called(cc)
}

func (m *MockClientMgr) Delete(id hotline.ClientID) {
	m.Called(id)
}

type MockBanMgr struct {
	mock.Mock
}

var _ hotline.BanMgr = (*MockBanMgr)(nil)

func (m *MockBanMgr) Add(ip string, until *time.Time) error {
	args := m.Called(ip, until)
	return args.Error(0)
}

func (m *MockBanMgr) IsBanned(ip string) (bool, *time.Time) {
	args := m.Called(ip)
	return args.Bool(0), args.Get(1).(*time.Time)
}

func (m *MockBanMgr) UnbanIP(ip string) error {
	args := m.Called(ip)
	return args.Error(0)
}

func (m *MockBanMgr) BanUsername(username string) error {
	args := m.Called(username)
	return args.Error(0)
}

func (m *MockBanMgr) UnbanUsername(username string) error {
	args := m.Called(username)
	return args.Error(0)
}

func (m *MockBanMgr) IsUsernameBanned(username string) bool {
	args := m.Called(username)
	return args.Bool(0)
}

func (m *MockBanMgr) BanNickname(nickname string) error {
	args := m.Called(nickname)
	return args.Error(0)
}

func (m *MockBanMgr) UnbanNickname(nickname string) error {
	args := m.Called(nickname)
	return args.Error(0)
}

func (m *MockBanMgr) IsNicknameBanned(nickname string) bool {
	args := m.Called(nickname)
	return args.Bool(0)
}

func (m *MockBanMgr) ListBannedIPs() ([]string, error) {
	args := m.Called()
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockBanMgr) ListBannedUsernames() ([]string, error) {
	args := m.Called()
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockBanMgr) ListBannedNicknames() ([]string, error) {
	args := m.Called()
	return args.Get(0).([]string), args.Error(1)
}

type MockChatManager struct {
	mock.Mock
}

var _ hotline.ChatManager = (*MockChatManager)(nil)

func (m *MockChatManager) New(cc *hotline.ClientConn) hotline.ChatID {
	args := m.Called(cc)

	return args.Get(0).(hotline.ChatID)
}

func (m *MockChatManager) GetSubject(id hotline.ChatID) string {
	args := m.Called(id)

	return args.String(0)
}

func (m *MockChatManager) Join(id hotline.ChatID, cc *hotline.ClientConn) {
	m.Called(id, cc)
}

func (m *MockChatManager) Leave(id hotline.ChatID, clientID [2]byte) {
	m.Called(id, clientID)
}

func (m *MockChatManager) SetSubject(id hotline.ChatID, subject string) {
	m.Called(id, subject)
}

func (m *MockChatManager) Members(id hotline.ChatID) []*hotline.ClientConn {
	args := m.Called(id)

	return args.Get(0).([]*hotline.ClientConn)
}

type MockThreadNewsMgr struct {
	mock.Mock
}

var _ hotline.ThreadedNewsMgr = (*MockThreadNewsMgr)(nil)

func (m *MockThreadNewsMgr) ListArticles(newsPath []string) (hotline.NewsArtListData, error) {
	args := m.Called(newsPath)

	return args.Get(0).(hotline.NewsArtListData), args.Error(1)
}

func (m *MockThreadNewsMgr) GetArticle(newsPath []string, articleID uint32) *hotline.NewsArtData {
	args := m.Called(newsPath, articleID)

	return args.Get(0).(*hotline.NewsArtData)
}

func (m *MockThreadNewsMgr) DeleteArticle(newsPath []string, articleID uint32, recursive bool) error {
	args := m.Called(newsPath, articleID, recursive)

	return args.Error(0)
}

func (m *MockThreadNewsMgr) PostArticle(newsPath []string, parentArticleID uint32, article hotline.NewsArtData) error {
	args := m.Called(newsPath, parentArticleID, article)

	return args.Error(0)
}

func (m *MockThreadNewsMgr) CreateGrouping(newsPath []string, name string, itemType [2]byte) error {
	args := m.Called(newsPath, name, itemType)

	return args.Error(0)
}

func (m *MockThreadNewsMgr) GetCategories(paths []string) []hotline.NewsCategoryListData15 {
	args := m.Called(paths)

	return args.Get(0).([]hotline.NewsCategoryListData15)
}

func (m *MockThreadNewsMgr) NewsItem(newsPath []string) hotline.NewsCategoryListData15 {
	args := m.Called(newsPath)

	return args.Get(0).(hotline.NewsCategoryListData15)
}

func (m *MockThreadNewsMgr) DeleteNewsItem(newsPath []string) error {
	args := m.Called(newsPath)

	return args.Error(0)
}
