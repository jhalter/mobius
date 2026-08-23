package mobius

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"

	"github.com/jhalter/mobius/hotline"
)

var errNewsArticleListTooLarge = errors.New("Hotline news article list exceeds 65,535 bytes")

type feedImportResult struct {
	count        int
	limitReached bool
}

func (n *ThreadedNewsYAML) validateFeedCategory(newsPath []string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	_, _, category, ok := n.feedCategoryLocked(newsPath)
	if !ok {
		return errors.New("target category does not exist")
	}
	if category.Type != hotline.NewsCategory {
		return errors.New("target must be an ordinary news category")
	}
	return nil
}

func (n *ThreadedNewsYAML) feedValidators(newsPath []string, sourceURL string) (feedHTTPValidators, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	_, _, category, ok := n.feedCategoryLocked(newsPath)
	if !ok {
		return feedHTTPValidators{}, errors.New("feed target category no longer exists")
	}
	if category.Type != hotline.NewsCategory {
		return feedHTTPValidators{}, errors.New("feed target is no longer an ordinary news category")
	}
	if category.FeedState == nil || category.FeedState.SourceURL != sourceURL {
		return feedHTTPValidators{}, nil
	}
	return feedHTTPValidators{
		etag:         category.FeedState.ETag,
		lastModified: category.FeedState.LastModified,
	}, nil
}

func (n *ThreadedNewsYAML) importFeedArticles(
	newsPath []string,
	sourceURL string,
	etag string,
	lastModified string,
	items []normalizedFeedItem,
) (feedImportResult, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	parent, name, original, ok := n.feedCategoryLocked(newsPath)
	if !ok {
		return feedImportResult{}, errors.New("feed target category no longer exists")
	}
	if original.Type != hotline.NewsCategory {
		return feedImportResult{}, errors.New("feed target is no longer an ordinary news category")
	}

	working := cloneNewsCategory(original)
	if working.Articles == nil {
		working.Articles = make(map[uint32]*hotline.NewsArtData)
	}
	if working.FeedState == nil {
		working.FeedState = &hotline.NewsFeedCategoryState{Imported: make(map[string]uint32)}
	}
	if working.FeedState.Imported == nil {
		working.FeedState.Imported = make(map[string]uint32)
	}

	result := feedImportResult{}
	for _, item := range items {
		identityHash := feedIdentityHash(sourceURL, item.identity)
		if _, seen := working.FeedState.Imported[identityHash]; seen {
			continue
		}

		articleID, previousID, err := nextNewsArticleID(working.Articles)
		if err != nil {
			return feedImportResult{}, err
		}
		article := item.article
		article.PrevArt = [4]byte{}
		article.NextArt = [4]byte{}
		article.ParentArt = [4]byte{}
		article.FirstChildArt = [4]byte{}
		article.DataFlav = slices.Clone(hotline.NewsFlavor)

		var oldPreviousNext [4]byte
		if previousID != 0 {
			oldPreviousNext = working.Articles[previousID].NextArt
			binary.BigEndian.PutUint32(article.PrevArt[:], previousID)
			binary.BigEndian.PutUint32(working.Articles[previousID].NextArt[:], articleID)
		}
		working.Articles[articleID] = &article

		if err := validateNewsArticleListSize(working.Articles); err != nil {
			delete(working.Articles, articleID)
			if previousID != 0 {
				working.Articles[previousID].NextArt = oldPreviousNext
			}
			if errors.Is(err, errNewsArticleListTooLarge) {
				result.limitReached = true
				break
			}
			return feedImportResult{}, err
		}

		working.FeedState.Imported[identityHash] = articleID
		result.count++
	}

	if !result.limitReached {
		working.FeedState.SourceURL = sourceURL
		working.FeedState.ETag = etag
		working.FeedState.LastModified = lastModified
	}

	if result.count == 0 && feedStateEqual(original.FeedState, working.FeedState) {
		return result, nil
	}
	parent[name] = working
	if err := n.writeFile(); err != nil {
		parent[name] = original
		return feedImportResult{}, fmt.Errorf("persist imported feed articles: %w", err)
	}
	return result, nil
}

func (n *ThreadedNewsYAML) feedCategoryLocked(newsPath []string) (map[string]hotline.NewsCategoryListData15, string, hotline.NewsCategoryListData15, bool) {
	if len(newsPath) == 0 {
		return nil, "", hotline.NewsCategoryListData15{}, false
	}
	categories := n.ThreadedNews.Categories
	for i, segment := range newsPath {
		category, ok := categories[segment]
		if !ok {
			return nil, "", hotline.NewsCategoryListData15{}, false
		}
		if i == len(newsPath)-1 {
			return categories, segment, category, true
		}
		categories = category.SubCats
		if categories == nil {
			return nil, "", hotline.NewsCategoryListData15{}, false
		}
	}
	return nil, "", hotline.NewsCategoryListData15{}, false
}

func cloneNewsCategory(category hotline.NewsCategoryListData15) hotline.NewsCategoryListData15 {
	clone := category
	clone.Articles = make(map[uint32]*hotline.NewsArtData, len(category.Articles))
	for id, article := range category.Articles {
		if article == nil {
			clone.Articles[id] = nil
			continue
		}
		articleClone := *article
		articleClone.DataFlav = slices.Clone(article.DataFlav)
		clone.Articles[id] = &articleClone
	}
	clone.SubCats = make(map[string]hotline.NewsCategoryListData15, len(category.SubCats))
	for name, subcategory := range category.SubCats {
		clone.SubCats[name] = cloneNewsCategory(subcategory)
	}
	if category.FeedState != nil {
		state := *category.FeedState
		state.Imported = make(map[string]uint32, len(category.FeedState.Imported))
		for identity, id := range category.FeedState.Imported {
			state.Imported[identity] = id
		}
		clone.FeedState = &state
	}
	return clone
}

func nextNewsArticleID(articles map[uint32]*hotline.NewsArtData) (uint32, uint32, error) {
	var previousID uint32
	for id := range articles {
		if id > previousID {
			previousID = id
		}
	}
	if previousID == math.MaxUint32 {
		return 0, 0, errors.New("news category exhausted article IDs")
	}
	return previousID + 1, previousID, nil
}

func validateNewsArticleListSize(articles map[uint32]*hotline.NewsArtData) error {
	category := hotline.NewsCategoryListData15{Articles: articles}
	list, err := category.GetNewsArtListData()
	if err != nil {
		return err
	}
	encoded, err := io.ReadAll(&list)
	if err != nil {
		return err
	}
	if len(encoded) > math.MaxUint16 {
		return errNewsArticleListTooLarge
	}
	return nil
}

func feedIdentityHash(sourceURL, identity string) string {
	hash := sha256.Sum256([]byte(sourceURL + "\x00" + identity))
	return hex.EncodeToString(hash[:])
}

func feedStateEqual(a, b *hotline.NewsFeedCategoryState) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.SourceURL != b.SourceURL || a.ETag != b.ETag || a.LastModified != b.LastModified || len(a.Imported) != len(b.Imported) {
		return false
	}
	for identity, id := range a.Imported {
		if b.Imported[identity] != id {
			return false
		}
	}
	return true
}
