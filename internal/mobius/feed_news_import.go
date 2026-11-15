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

var errNewsArticleListTooLarge = errors.New("hotline news article list exceeds 65,535 bytes")

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

		articleID, previousID, err := nextNewsArticleID(working.Articles, working.FeedState)
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

		err = validateNewsArticleListSize(working.Articles)
		if errors.Is(err, errNewsArticleListTooLarge) {
			if pruned, ok := pruneImportedToFit(working, articleID); ok {
				working, err = pruned, nil
			}
		}
		if err != nil {
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

func nextNewsArticleID(articles map[uint32]*hotline.NewsArtData, state *hotline.NewsFeedCategoryState) (uint32, uint32, error) {
	var previousID uint32
	for id := range articles {
		if id > previousID {
			previousID = id
		}
	}
	highest := max(previousID, maxImportedArticleID(state))
	if highest == math.MaxUint32 {
		return 0, 0, errors.New("news category exhausted article IDs")
	}
	return highest + 1, previousID, nil
}

// maxImportedArticleID is the highest ID a feed has imported an article under. New articles are
// numbered above it even when that article is gone, so an ID the feed state names is never given
// to a locally posted article that pruning could then remove.
func maxImportedArticleID(state *hotline.NewsFeedCategoryState) uint32 {
	var highest uint32
	if state != nil {
		for _, id := range state.Imported {
			highest = max(highest, id)
		}
	}
	return highest
}

// pruneImportedToFit removes the oldest imported articles from a copy of category until its
// article list fits the protocol limit, so a full category keeps the newest feed items rather
// than the oldest. Locally posted articles, imported articles that have replies, and keep are
// never removed. It reports false, leaving category untouched, if removing every candidate
// would not be enough.
func pruneImportedToFit(category hotline.NewsCategoryListData15, keep uint32) (hotline.NewsCategoryListData15, bool) {
	var candidates []uint32
	for _, id := range category.FeedState.Imported {
		if _, ok := category.Articles[id]; ok && id != keep && !hasNewsReplies(category.Articles, id) {
			candidates = append(candidates, id)
		}
	}
	slices.Sort(candidates)
	candidates = slices.Compact(candidates)

	pruned := cloneNewsCategory(category)
	for _, id := range candidates {
		unlinkNewsArticle(pruned.Articles, id)
		err := validateNewsArticleListSize(pruned.Articles)
		if err == nil {
			return pruned, true
		}
		if !errors.Is(err, errNewsArticleListTooLarge) {
			return category, false
		}
	}
	return category, false
}

func hasNewsReplies(articles map[uint32]*hotline.NewsArtData, id uint32) bool {
	for _, article := range articles {
		if article != nil && binary.BigEndian.Uint32(article.ParentArt[:]) == id {
			return true
		}
	}
	return false
}

func unlinkNewsArticle(articles map[uint32]*hotline.NewsArtData, id uint32) {
	article := articles[id]
	if previous, ok := articles[binary.BigEndian.Uint32(article.PrevArt[:])]; ok && previous != nil {
		previous.NextArt = article.NextArt
	}
	if next, ok := articles[binary.BigEndian.Uint32(article.NextArt[:])]; ok && next != nil {
		next.PrevArt = article.PrevArt
	}
	delete(articles, id)
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
