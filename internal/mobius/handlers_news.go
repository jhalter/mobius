package mobius

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jhalter/mobius/hotline"
)

// HandleTranOldPostNews posts to the flat news (message board).
//
// Access: News Post Article (21)
//
// Fields used in the request:
//   - 101 Data  Required - News post content
//
// Fields used in the reply: None
func HandleTranOldPostNews(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessNewsPostArt) {
		return cc.NewErrReply(t, ErrMsgNotAllowedPostNews)
	}

	newsDateTemplate := hotline.NewsDateFormat
	if cc.Server.Config.NewsDateFormat != "" {
		newsDateTemplate = cc.Server.Config.NewsDateFormat
	}

	newsTemplate := hotline.NewsTemplate
	if cc.Server.Config.NewsDelimiter != "" {
		newsTemplate = cc.Server.Config.NewsDelimiter
	}

	newsPost := fmt.Sprintf(newsTemplate+"\r", cc.GetUserName(), time.Now().Format(newsDateTemplate), t.GetField(hotline.FieldData).Data)
	newsPost = strings.ReplaceAll(newsPost, "\n", "\r")

	_, err := cc.Server.MessageBoard.Write([]byte(newsPost))
	if err != nil {
		cc.Logger.Error("error writing news post", "err", err)
		return cc.NewErrReply(t, ErrMsgPostNews)
	}

	// Notify all clients of updated news
	cc.SendAll(
		hotline.TranNewMsg,
		hotline.NewField(hotline.FieldData, []byte(newsPost)),
	)

	return append(res, cc.NewReply(t))
}

// HandleGetNewsCatNameList gets the list of category names at the specified news path.
//
// Fields used in the request:
//   - 325 News path  Optional
//
// Fields used in the reply:
//   - 323 News category list data  Repeated - Category information
func HandleGetNewsCatNameList(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessNewsReadArt) {
		return cc.NewErrReply(t, ErrMsgNotAllowedReadNews)
	}

	pathStrs, err := t.GetField(hotline.FieldNewsPath).DecodeNewsPath()
	if err != nil {
		cc.Logger.Error("get news path", "err", err)
		return cc.NewErrReply(t, ErrMsgReadNewsCategories)
	}

	var fields []hotline.Field
	for _, cat := range cc.Server.ThreadedNewsMgr.GetCategories(pathStrs) {
		b, err := io.ReadAll(&cat)
		if err != nil {
			cc.Logger.Error("get news categories", "err", err)
			return cc.NewErrReply(t, ErrMsgReadNewsCategories)
		}

		fields = append(fields, hotline.NewField(hotline.FieldNewsCatListData15, b))
	}

	return append(res, cc.NewReply(t, fields...))
}

// HandleNewNewsCat creates a new news category on the server.
//
// Access: News Create Category (34)
//
// Fields used in the request:
//   - 322 News category name  Required
//   - 325 News path           Required
//
// Fields used in the reply: None
func HandleNewNewsCat(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessNewsCreateCat) {
		return cc.NewErrReply(t, ErrMsgNotAllowedCreateNewsCategories)
	}

	name := string(t.GetField(hotline.FieldNewsCatName).Data)
	pathStrs, err := t.GetField(hotline.FieldNewsPath).DecodeNewsPath()
	if err != nil {
		cc.Logger.Error("new news category: decode news path", "err", err)
		return cc.NewErrReply(t, ErrMsgCreateNewsCategory)
	}

	err = cc.Server.ThreadedNewsMgr.CreateGrouping(pathStrs, name, hotline.NewsCategory)
	if err != nil {
		cc.Logger.Error("error creating news category", "err", err)
		return cc.NewErrReply(t, ErrMsgCreateNewsCategory)
	}

	return []hotline.Transaction{cc.NewReply(t)}
}

// HandleNewNewsFldr creates a new news folder on the server.
//
// Access: News Create Folder (36)
//
// Fields used in the request:
//   - 201 File name   Required
//   - 325 News path   Required
//
// Fields used in the reply: None
func HandleNewNewsFldr(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessNewsCreateFldr) {
		return cc.NewErrReply(t, ErrMsgNotAllowedCreateNewsfolders)
	}

	name := string(t.GetField(hotline.FieldFileName).Data)
	pathStrs, err := t.GetField(hotline.FieldNewsPath).DecodeNewsPath()
	if err != nil {
		cc.Logger.Error("new news folder: decode news path", "err", err)
		return cc.NewErrReply(t, ErrMsgCreateNewsFolder)
	}

	err = cc.Server.ThreadedNewsMgr.CreateGrouping(pathStrs, name, hotline.NewsBundle)
	if err != nil {
		cc.Logger.Error("error creating news bundle", "err", err)
		return cc.NewErrReply(t, ErrMsgCreateNewsFolder)
	}

	return append(res, cc.NewReply(t))
}

// HandleGetNewsArtNameList gets the list of article names at the specified news path.
//
// Fields used in the request:
//   - 325 News path  Optional
//
// Fields used in the reply:
//   - 321 News article list data  Optional
func HandleGetNewsArtNameList(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessNewsReadArt) {
		return cc.NewErrReply(t, ErrMsgNotAllowedReadNews)
	}

	pathStrs, err := t.GetField(hotline.FieldNewsPath).DecodeNewsPath()
	if err != nil {
		cc.Logger.Error("get news article list: decode news path", "err", err)
		return cc.NewErrReply(t, ErrMsgReadNewsArticles)
	}

	nald, err := cc.Server.ThreadedNewsMgr.ListArticles(pathStrs)
	if err != nil {
		cc.Logger.Error("get news article list: list articles", "err", err)
		return cc.NewErrReply(t, ErrMsgReadNewsArticles)
	}

	b, err := io.ReadAll(&nald)
	if err != nil {
		cc.Logger.Error("get news article list: read article list data", "err", err)
		return cc.NewErrReply(t, ErrMsgReadNewsArticles)
	}

	return append(res, cc.NewReply(t, hotline.NewField(hotline.FieldNewsArtListData, b)))
}

// HandleGetNewsArtData requests information about a specific news article.
//
// Access: News Read Article (20)
//
// Fields used in the request:
//   - 325 News path                 Required
//   - 326 News article ID           Required
//   - 327 News article data flavor  Required
//
// Fields used in the reply:
//   - 328 News article title        Article title
//   - 329 News article poster       Author
//   - 330 News article date         Publication date
//   - 331 Previous article ID       ID of previous article
//   - 332 Next article ID           ID of next article
//   - 335 Parent article ID         ID of parent article
//   - 336 First child article ID    ID of first reply
//   - 327 News article data flavor  Should be "text/plain"
//   - 333 News article data         Optional - Article content
func HandleGetNewsArtData(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessNewsReadArt) {
		return cc.NewErrReply(t, ErrMsgNotAllowedReadNews)
	}

	newsPath, err := t.GetField(hotline.FieldNewsPath).DecodeNewsPath()
	if err != nil {
		cc.Logger.Error("get news article: decode news path", "err", err)
		return cc.NewErrReply(t, ErrMsgReadNewsArticles)
	}

	convertedID, err := t.GetField(hotline.FieldNewsArtID).DecodeInt()
	if err != nil {
		cc.Logger.Error("get news article: decode article ID", "err", err)
		return cc.NewErrReply(t, ErrMsgReadNewsArticles)
	}

	art := cc.Server.ThreadedNewsMgr.GetArticle(newsPath, uint32(convertedID))
	if art == nil {
		return append(res, cc.NewReply(t))
	}

	res = append(res, cc.NewReply(t,
		hotline.NewField(hotline.FieldNewsArtTitle, []byte(art.Title)),
		hotline.NewField(hotline.FieldNewsArtPoster, []byte(art.Poster)),
		hotline.NewField(hotline.FieldNewsArtDate, art.Date[:]),
		hotline.NewField(hotline.FieldNewsArtPrevArt, art.PrevArt[:]),
		hotline.NewField(hotline.FieldNewsArtNextArt, art.NextArt[:]),
		hotline.NewField(hotline.FieldNewsArtParentArt, art.ParentArt[:]),
		hotline.NewField(hotline.FieldNewsArt1stChildArt, art.FirstChildArt[:]),
		hotline.NewField(hotline.FieldNewsArtDataFlav, []byte("text/plain")),
		hotline.NewField(hotline.FieldNewsArtData, []byte(art.Data)),
	))
	return res
}

// HandleDelNewsItem deletes an existing news item from the server.
//
// Access: News Delete Folder (37) or News Delete Category (35)
//
// Fields used in the request:
//   - 325 News path  Required
//
// Fields used in the reply: None
func HandleDelNewsItem(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	pathStrs, err := t.GetField(hotline.FieldNewsPath).DecodeNewsPath()
	if err != nil || len(pathStrs) == 0 {
		cc.Logger.Error("delete news item: invalid news path", "err", err)
		return cc.NewErrReply(t, ErrMsgDeleteNewsItem)
	}

	item := cc.Server.ThreadedNewsMgr.NewsItem(pathStrs)

	if item.Type == [2]byte{0, 3} {
		if !cc.Authorize(hotline.AccessNewsDeleteCat) {
			return cc.NewErrReply(t, ErrMsgNotAllowedDeleteNewsCategories)
		}
	} else {
		if !cc.Authorize(hotline.AccessNewsDeleteFldr) {
			return cc.NewErrReply(t, ErrMsgNotAllowedDeleteNewsFolders)
		}
	}

	err = cc.Server.ThreadedNewsMgr.DeleteNewsItem(pathStrs)
	if err != nil {
		cc.Logger.Error("delete news item", "err", err)
		return cc.NewErrReply(t, ErrMsgDeleteNewsItem)
	}

	return append(res, cc.NewReply(t))
}

// HandleDelNewsArt deletes a specific news article.
//
// Access: News Delete Article (33)
//
// Fields used in the request:
//   - 325 News path                       Required
//   - 326 News article ID                 Required
//   - 337 News article – recursive delete Optional - Delete child articles (1) or not (0)
//
// Fields used in the reply: None
func HandleDelNewsArt(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessNewsDeleteArt) {
		return cc.NewErrReply(t, ErrMsgNotAllowedDeleteNewsArticles)

	}

	pathStrs, err := t.GetField(hotline.FieldNewsPath).DecodeNewsPath()
	if err != nil {
		cc.Logger.Error("delete news article: decode news path", "err", err)
		return cc.NewErrReply(t, ErrMsgDeleteNewsArticle)
	}

	articleID, err := t.GetField(hotline.FieldNewsArtID).DecodeInt()
	if err != nil {
		cc.Logger.Error("delete news article: decode article ID", "err", err)
		return cc.NewErrReply(t, ErrMsgDeleteNewsArticle)
	}

	deleteRecursive := bytes.Equal([]byte{0, 1}, t.GetField(hotline.FieldNewsArtRecurseDel).Data)

	err = cc.Server.ThreadedNewsMgr.DeleteArticle(pathStrs, uint32(articleID), deleteRecursive)
	if err != nil {
		cc.Logger.Error("error deleting news article", "err", err)
		return cc.NewErrReply(t, ErrMsgDeleteNewsArticle)
	}

	return []hotline.Transaction{cc.NewReply(t)}
}

// HandlePostNewsArt posts a new news article on the server.
//
// Access: News Post Article (21)
//
// Fields used in the request:
//   - 325 News path                 Required
//   - 326 News article ID           ID of the parent article
//   - 328 News article title        Required
//   - 334 News article flags        Optional
//   - 327 News article data flavor  Currently "text/plain"
//   - 333 News article data         Required
//
// Fields used in the reply: None
func HandlePostNewsArt(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessNewsPostArt) {
		return cc.NewErrReply(t, ErrMsgNotAllowedPostNewsArticles)
	}

	pathStrs, err := t.GetField(hotline.FieldNewsPath).DecodeNewsPath()
	if err != nil || len(pathStrs) == 0 {
		cc.Logger.Error("post news article: invalid news path", "err", err)
		return cc.NewErrReply(t, ErrMsgPostNewsArticle)
	}

	parentArticleID, err := t.GetField(hotline.FieldNewsArtID).DecodeInt()
	if err != nil {
		cc.Logger.Error("post news article: decode parent article ID", "err", err)
		return cc.NewErrReply(t, ErrMsgPostNewsArticle)
	}

	err = cc.Server.ThreadedNewsMgr.PostArticle(
		pathStrs,
		uint32(parentArticleID),
		hotline.NewsArtData{
			Title:    string(t.GetField(hotline.FieldNewsArtTitle).Data),
			Poster:   string(cc.GetUserName()),
			Date:     hotline.NewTime(time.Now()),
			DataFlav: hotline.NewsFlavor,
			Data:     string(t.GetField(hotline.FieldNewsArtData).Data),
		},
	)
	if err != nil {
		cc.Logger.Error("error posting news article", "err", err)
		return cc.NewErrReply(t, ErrMsgPostNewsArticle)
	}

	return append(res, cc.NewReply(t))
}

// HandleGetMsgs returns the flat news data (message board content).
//
// Fields used in the request: None
//
// Fields used in the reply:
//   - 101 Data  Message text
func HandleGetMsgs(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessNewsReadArt) {
		return cc.NewErrReply(t, ErrMsgNotAllowedReadNews)
	}

	_, _ = cc.Server.MessageBoard.Seek(0, 0)

	newsData, err := io.ReadAll(cc.Server.MessageBoard)
	if err != nil {
		cc.Logger.Error("Error reading messageboard", "err", err)
		return cc.NewErrReply(t, ErrMsgReadMessageBoard)
	}

	return append(res, cc.NewReply(t, hotline.NewField(hotline.FieldData, newsData)))
}
