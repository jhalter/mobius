package mobius

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/jhalter/mobius/hotline"
	"golang.org/x/sync/singleflight"
	textencoding "golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
)

// FeedNewsManager refreshes configured feeds when their existing category is
// opened. Everything except ListArticles remains ordinary threaded-news
// behavior because imported entries are ordinary ThreadedNews.yaml articles.
type FeedNewsManager struct {
	base     *ThreadedNewsYAML
	feeds    map[string]configuredNewsFeed
	logger   *slog.Logger
	fetcher  *feedFetcher
	encoding string
	refresh  singleflight.Group
}

type configuredNewsFeed struct {
	config   hotline.NewsFeedConfig
	wirePath []string
}

var _ hotline.ThreadedNewsMgr = (*FeedNewsManager)(nil)

func NewFeedNewsManager(
	base *ThreadedNewsYAML,
	feeds []hotline.NewsFeedConfig,
	logger *slog.Logger,
	userAgent string,
	clientEncoding string,
) (*FeedNewsManager, error) {
	if base == nil {
		return nil, errors.New("feed news manager requires a threaded news YAML manager")
	}
	if logger == nil {
		logger = slog.Default()
	}

	normalized := &hotline.Config{NewsFeeds: slices.Clone(feeds)}
	if err := normalizeNewsFeedConfig(normalized); err != nil {
		return nil, err
	}

	manager := &FeedNewsManager{
		base:     base,
		feeds:    make(map[string]configuredNewsFeed, len(feeds)),
		logger:   logger,
		fetcher:  newFeedFetcher(userAgent),
		encoding: normalizedFeedEncodingName(clientEncoding),
	}
	for _, config := range normalized.NewsFeeds {
		wirePath := encodeFeedPath(config.CategoryPath, manager.encoding)
		key := newsPathKey(wirePath)
		if _, exists := manager.feeds[key]; exists {
			return nil, fmt.Errorf("NewsFeeds categories collide after %s encoding", manager.encoding)
		}
		if err := base.validateFeedCategory(wirePath); err != nil {
			return nil, fmt.Errorf("NewsFeeds category %q: %w", strings.Join(config.CategoryPath, "/"), err)
		}
		manager.feeds[key] = configuredNewsFeed{config: config, wirePath: wirePath}
	}
	return manager, nil
}

func (m *FeedNewsManager) ListArticles(newsPath []string) (hotline.NewsArtListData, error) {
	feed, mapped := m.feeds[newsPathKey(newsPath)]
	if !mapped {
		return m.base.ListArticles(newsPath)
	}

	_, err, _ := m.refresh.Do(newsPathKey(newsPath), func() (any, error) {
		return nil, m.refreshFeed(feed)
	})
	if err != nil {
		m.logger.Warn("Unable to refresh feed-backed news; serving local articles",
			"category", strings.Join(feed.config.CategoryPath, "/"),
			"err", err,
		)
	}
	return m.base.ListArticles(newsPath)
}

func (m *FeedNewsManager) refreshFeed(feed configuredNewsFeed) error {
	validators, err := m.base.feedValidators(feed.wirePath, feed.config.URL)
	if err != nil {
		return err
	}
	result, err := m.fetcher.fetch(context.Background(), feed.config.URL, validators)
	if err != nil {
		return err
	}
	if result.notModified {
		return nil
	}
	if result.skippedNoIdentity > 0 {
		m.logger.Warn("Skipped feed entries without a stable identity",
			"category", strings.Join(feed.config.CategoryPath, "/"),
			"count", result.skippedNoIdentity,
		)
	}

	items := make([]normalizedFeedItem, len(result.items))
	for i, item := range result.items {
		items[i] = item
		items[i].article = encodeFeedArticle(item.article, m.encoding)
	}
	imported, err := m.base.importFeedArticles(
		feed.wirePath,
		feed.config.URL,
		result.etag,
		result.lastModified,
		items,
	)
	if err != nil {
		return err
	}
	if imported.limitReached {
		m.logger.Warn("Stopped importing feed entries at the Hotline article-list size limit",
			"category", strings.Join(feed.config.CategoryPath, "/"),
			"imported", imported.count,
		)
	}
	return nil
}

func (m *FeedNewsManager) GetArticle(newsPath []string, articleID uint32) *hotline.NewsArtData {
	return m.base.GetArticle(newsPath, articleID)
}

func (m *FeedNewsManager) DeleteArticle(newsPath []string, articleID uint32, recursive bool) error {
	return m.base.DeleteArticle(newsPath, articleID, recursive)
}

func (m *FeedNewsManager) PostArticle(newsPath []string, parentArticleID uint32, article hotline.NewsArtData) error {
	return m.base.PostArticle(newsPath, parentArticleID, article)
}

func (m *FeedNewsManager) CreateGrouping(newsPath []string, name string, itemType [2]byte) error {
	return m.base.CreateGrouping(newsPath, name, itemType)
}

func (m *FeedNewsManager) GetCategories(newsPath []string) []hotline.NewsCategoryListData15 {
	return m.base.GetCategories(newsPath)
}

func (m *FeedNewsManager) NewsItem(newsPath []string) hotline.NewsCategoryListData15 {
	return m.base.NewsItem(newsPath)
}

func (m *FeedNewsManager) DeleteNewsItem(newsPath []string) error {
	return m.base.DeleteNewsItem(newsPath)
}

func newsPathKey(path []string) string {
	return strings.Join(path, "\x00")
}

func normalizedFeedEncodingName(value string) string {
	if value == "utf8" {
		return "utf8"
	}
	return "macintosh"
}

func encodeFeedPath(path []string, clientEncoding string) []string {
	encoded := slices.Clone(path)
	for i := range encoded {
		encoded[i] = encodeFeedText(encoded[i], clientEncoding)
	}
	return encoded
}

func encodeFeedArticle(article hotline.NewsArtData, clientEncoding string) hotline.NewsArtData {
	article.Title = truncateEncodedFeedText(article.Title, maxNewsStringBytes, clientEncoding)
	article.Poster = truncateEncodedFeedText(article.Poster, maxNewsStringBytes, clientEncoding)
	article.Data = truncateEncodedFeedText(article.Data, maxNewsBodyBytes, clientEncoding)
	article.DataFlav = slices.Clone(hotline.NewsFlavor)
	return article
}

func truncateEncodedFeedText(value string, maxBytes int, clientEncoding string) string {
	encoded := encodeFeedText(value, clientEncoding)
	if len(encoded) <= maxBytes {
		return encoded
	}
	if normalizedFeedEncodingName(clientEncoding) == "utf8" {
		return truncateUTF8(encoded, maxBytes)
	}
	return encoded[:maxBytes]
}

func encodeFeedText(value, clientEncoding string) string {
	if normalizedFeedEncodingName(clientEncoding) == "utf8" {
		return value
	}
	encoded, err := textencoding.ReplaceUnsupported(charmap.Macintosh.NewEncoder()).String(value)
	if err == nil {
		return encoded
	}
	return strings.Map(func(r rune) rune {
		if r <= 0x7f {
			return r
		}
		return '?'
	}, value)
}
