package mobius

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	textencoding "golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
)

const testFeedURL = "https://example.com/releases.xml"

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func testFeedResponse(status int, body string, headers map[string]string) *http.Response {
	response := &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	for key, value := range headers {
		response.Header.Set(key, value)
	}
	return response
}

func testNewsFeedConfig(url string, path ...string) hotline.NewsFeedConfig {
	return hotline.NewsFeedConfig{CategoryPath: path, URL: url}
}

func addTestNewsCategory(t *testing.T, base *ThreadedNewsYAML, path []string, name string) {
	t.Helper()
	require.NoError(t, base.CreateGrouping(path, name, hotline.NewsCategory))
}

func newTestFeedNewsManager(
	t *testing.T,
	base *ThreadedNewsYAML,
	config hotline.NewsFeedConfig,
	transport http.RoundTripper,
	encoding string,
) *FeedNewsManager {
	t.Helper()
	manager, err := NewFeedNewsManager(
		base,
		[]hotline.NewsFeedConfig{config},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		"mobius-test",
		encoding,
	)
	require.NoError(t, err)
	if transport != nil {
		manager.fetcher.client = &http.Client{Transport: transport}
	}
	return manager
}

func feedState(t *testing.T, base *ThreadedNewsYAML, path ...string) *hotline.NewsFeedCategoryState {
	t.Helper()
	state := base.NewsItem(path).FeedState
	require.NotNil(t, state)
	return state
}

func TestFeedNewsManager_ImportsAfterglowAsOrdinaryNews(t *testing.T) {
	base := newTestThreadedNews(t)
	addTestNewsCategory(t, base, nil, "Afterglow Releases")
	requests := 0
	manager := newTestFeedNewsManager(t, base, testNewsFeedConfig(testFeedURL, "Afterglow Releases"), roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		assert.Equal(t, "application/rss+xml, application/atom+xml, application/xml;q=0.9, text/xml;q=0.8", request.Header.Get("Accept"))
		assert.Equal(t, "mobius-test", request.Header.Get("User-Agent"))
		return testFeedResponse(http.StatusOK, feedFixture(t, "afterglow-appcast.xml"), map[string]string{
			"ETag":          `"afterglow-23"`,
			"Last-Modified": "Sat, 22 Aug 2026 18:09:55 GMT",
		}), nil
	}), "utf8")

	list, err := manager.ListArticles([]string{"Afterglow Releases"})
	require.NoError(t, err)
	assert.Equal(t, 1, list.Count)
	assert.Equal(t, 1, requests)

	state := feedState(t, base, "Afterglow Releases")
	assert.Equal(t, testFeedURL, state.SourceURL)
	assert.Equal(t, `"afterglow-23"`, state.ETag)
	require.Len(t, state.Imported, 1)
	var rootID uint32
	for _, id := range state.Imported {
		rootID = id
	}
	root := manager.GetArticle([]string{"Afterglow Releases"}, rootID)
	require.NotNil(t, root)
	assert.Equal(t, "Afterglow 1.0rc4", root.Title)
	assert.Equal(t, "Afterglow", root.Poster)
	assert.Contains(t, root.Data, "Fixed main-window resizing.")
	assert.Contains(t, root.Data, "Release notes: https://morphing.cloud/afterglow/1.0rc4-notes.html")
	assert.Contains(t, root.Data, "Download: https://morphing.cloud/afterglow/Afterglow-v1.0rc4.zip")
	assert.NotContains(t, root.Data, "<h3>")
	expectedTime := time.Date(2026, time.August, 22, 18, 9, 55, 0, time.FixedZone("appcast", -7*60*60))
	assert.True(t, hotline.Time(root.Date).NewsTime().Equal(expectedTime))

	// Imported roots are completely ordinary: local roots, replies, and deletion
	// all use the conventional manager.
	require.NoError(t, manager.PostArticle([]string{"Afterglow Releases"}, 0, hotline.NewsArtData{Title: "Local root"}))
	require.NoError(t, manager.PostArticle([]string{"Afterglow Releases"}, rootID, hotline.NewsArtData{Title: "Reply"}))
	require.NoError(t, manager.DeleteArticle([]string{"Afterglow Releases"}, rootID, true))
	assert.Nil(t, manager.GetArticle([]string{"Afterglow Releases"}, rootID))

	// The durable seen marker survives deletion, so the same feed entry stays
	// deleted even though the source still returns it.
	list, err = manager.ListArticles([]string{"Afterglow Releases"})
	require.NoError(t, err)
	assert.Equal(t, 2, list.Count)
	assert.Nil(t, manager.GetArticle([]string{"Afterglow Releases"}, rootID))
	assert.Equal(t, 2, requests)

	yamlData, err := os.ReadFile(base.filePath)
	require.NoError(t, err)
	assert.Contains(t, string(yamlData), "FeedState:")
	assert.Contains(t, string(yamlData), "Imported:")
	assert.NoFileExists(t, filepath.Join(filepath.Dir(base.filePath), "FeedNewsState.yaml"))
	assert.NoFileExists(t, filepath.Join(filepath.Dir(base.filePath), "FeedNewsCache.json"))
}

func TestFeedNewsManager_AutoDetectsAtomAndIgnoresUpdates(t *testing.T) {
	base := newTestThreadedNews(t)
	addTestNewsCategory(t, base, nil, "Mobius Releases")
	var requests int
	manager := newTestFeedNewsManager(t, base, testNewsFeedConfig(testFeedURL, "Mobius Releases"), roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			return testFeedResponse(http.StatusOK, feedFixture(t, "mobius-releases.atom"), map[string]string{"ETag": `"v1"`}), nil
		}
		assert.Equal(t, `"v1"`, request.Header.Get("If-None-Match"))
		return testFeedResponse(http.StatusOK, atomBatchFixture(
			atomEntry("v0.22.0 renamed", "tag:github.com,2008:Repository/272052223/v0.22.0", "2026-06-12T16:03:33Z"),
			atomEntry("v0.23.0", "tag:github.com,2008:Repository/272052223/v0.23.0", "2026-07-01T12:00:00Z"),
		), map[string]string{"ETag": `"v2"`}), nil
	}), "utf8")

	_, err := manager.ListArticles([]string{"Mobius Releases"})
	require.NoError(t, err)
	first := manager.GetArticle([]string{"Mobius Releases"}, 1)
	require.NotNil(t, first)
	assert.Equal(t, "v0.22.0", first.Title)

	list, err := manager.ListArticles([]string{"Mobius Releases"})
	require.NoError(t, err)
	assert.Equal(t, 2, list.Count)
	assert.Equal(t, "v0.22.0", manager.GetArticle([]string{"Mobius Releases"}, 1).Title, "same-identity changes must be ignored")
	assert.Equal(t, "v0.23.0", manager.GetArticle([]string{"Mobius Releases"}, 2).Title)
	assert.Equal(t, `"v2"`, feedState(t, base, "Mobius Releases").ETag)
}

func TestFeedNewsManager_RestartUsesConditionalGETWithoutDuplicates(t *testing.T) {
	base := newTestThreadedNews(t)
	addTestNewsCategory(t, base, nil, "Releases")
	config := testNewsFeedConfig(testFeedURL, "Releases")
	manager := newTestFeedNewsManager(t, base, config, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return testFeedResponse(http.StatusOK, rssFixture("One", "one", "Sat, 22 Aug 2026 18:09:55 -0700"), map[string]string{
			"ETag":          `"one"`,
			"Last-Modified": "Sat, 22 Aug 2026 18:09:55 GMT",
		}), nil
	}), "utf8")
	_, err := manager.ListArticles([]string{"Releases"})
	require.NoError(t, err)

	reloaded, err := NewThreadedNewsYAML(base.filePath)
	require.NoError(t, err)
	restarted := newTestFeedNewsManager(t, reloaded, config, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, `"one"`, request.Header.Get("If-None-Match"))
		assert.Equal(t, "Sat, 22 Aug 2026 18:09:55 GMT", request.Header.Get("If-Modified-Since"))
		return testFeedResponse(http.StatusNotModified, "", nil), nil
	}), "utf8")
	before, err := os.ReadFile(base.filePath)
	require.NoError(t, err)
	list, err := restarted.ListArticles([]string{"Releases"})
	require.NoError(t, err)
	after, err := os.ReadFile(base.filePath)
	require.NoError(t, err)
	assert.Equal(t, 1, list.Count)
	assert.Equal(t, before, after, "304 responses must not rewrite ThreadedNews.yaml")
}

func TestFeedNewsManager_SourceURLChangesKeepSeenHistory(t *testing.T) {
	base := newTestThreadedNews(t)
	addTestNewsCategory(t, base, nil, "Releases")
	path := []string{"Releases"}
	urlA := "https://example.com/a.xml"
	urlB := "https://example.com/b.xml"
	fixture := rssFixture("Release", "shared-id", "Sat, 22 Aug 2026 18:09:55 -0700")

	for _, sourceURL := range []string{urlA, urlB, urlA} {
		manager := newTestFeedNewsManager(t, base, testNewsFeedConfig(sourceURL, path...), roundTripFunc(func(request *http.Request) (*http.Response, error) {
			assert.Empty(t, request.Header.Get("If-None-Match"), "a URL switch must clear validators")
			return testFeedResponse(http.StatusOK, fixture, map[string]string{"ETag": `"etag"`}), nil
		}), "utf8")
		_, err := manager.ListArticles(path)
		require.NoError(t, err)
	}

	list, err := base.ListArticles(path)
	require.NoError(t, err)
	assert.Equal(t, 2, list.Count, "the same ID is distinct across URLs but not reimported after switching back")
	state := feedState(t, base, path...)
	assert.Equal(t, urlA, state.SourceURL)
	assert.Len(t, state.Imported, 2)
}

func TestFeedNewsManager_FailuresServeLocalAndEachLoadRetries(t *testing.T) {
	base := newTestThreadedNews(t)
	addTestNewsCategory(t, base, nil, "Releases")
	var requests atomic.Int32
	manager := newTestFeedNewsManager(t, base, testNewsFeedConfig(testFeedURL, "Releases"), roundTripFunc(func(*http.Request) (*http.Response, error) {
		if requests.Add(1) == 1 {
			return nil, errors.New("offline")
		}
		return testFeedResponse(http.StatusOK, rssFixture("Recovered", "recovered", "Sat, 22 Aug 2026 18:09:55 -0700"), nil), nil
	}), "utf8")

	list, err := manager.ListArticles([]string{"Releases"})
	require.NoError(t, err)
	assert.Zero(t, list.Count)
	list, err = manager.ListArticles([]string{"Releases"})
	require.NoError(t, err)
	assert.Equal(t, 1, list.Count)
	assert.Equal(t, int32(2), requests.Load())
}

func TestFeedNewsManager_CoalescesConcurrentLoads(t *testing.T) {
	base := newTestThreadedNews(t)
	addTestNewsCategory(t, base, nil, "Releases")
	var requests atomic.Int32
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	var startedOnce sync.Once
	manager := newTestFeedNewsManager(t, base, testNewsFeedConfig(testFeedURL, "Releases"), roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		startedOnce.Do(func() { close(requestStarted) })
		<-releaseRequest
		return testFeedResponse(http.StatusOK, rssFixture("One", "one", "Sat, 22 Aug 2026 18:09:55 -0700"), nil), nil
	}), "utf8")

	const callers = 12
	start := make(chan struct{})
	var ready sync.WaitGroup
	var done sync.WaitGroup
	ready.Add(callers)
	done.Add(callers)
	for range callers {
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			_, err := manager.ListArticles([]string{"Releases"})
			assert.NoError(t, err)
		}()
	}
	ready.Wait()
	close(start)
	<-requestStarted
	time.Sleep(25 * time.Millisecond)
	close(releaseRequest)
	done.Wait()
	assert.Equal(t, int32(1), requests.Load())
}

func TestFeedNewsManager_SkipsEntriesWithoutIdentityAndRejectsJSON(t *testing.T) {
	t.Run("missing identity", func(t *testing.T) {
		base := newTestThreadedNews(t)
		addTestNewsCategory(t, base, nil, "Releases")
		fixture := `<?xml version="1.0"?><rss version="2.0"><channel><title>Releases</title><item><title>No identity</title><description>Notes</description></item></channel></rss>`
		manager := newTestFeedNewsManager(t, base, testNewsFeedConfig(testFeedURL, "Releases"), roundTripFunc(func(*http.Request) (*http.Response, error) {
			return testFeedResponse(http.StatusOK, fixture, map[string]string{"ETag": `"empty"`}), nil
		}), "utf8")
		list, err := manager.ListArticles([]string{"Releases"})
		require.NoError(t, err)
		assert.Zero(t, list.Count)
		assert.Empty(t, feedState(t, base, "Releases").Imported)
	})

	t.Run("JSON", func(t *testing.T) {
		base := newTestThreadedNews(t)
		addTestNewsCategory(t, base, nil, "Releases")
		manager := newTestFeedNewsManager(t, base, testNewsFeedConfig(testFeedURL, "Releases"), roundTripFunc(func(*http.Request) (*http.Response, error) {
			return testFeedResponse(http.StatusOK, `{"version":"https://jsonfeed.org/version/1.1","items":[]}`, nil), nil
		}), "utf8")
		list, err := manager.ListArticles([]string{"Releases"})
		require.NoError(t, err)
		assert.Zero(t, list.Count)
		assert.Nil(t, base.NewsItem([]string{"Releases"}).FeedState)
	})
}

func TestFeedNewsManager_EncodingIsAppliedOnceAtImport(t *testing.T) {
	base := newTestThreadedNews(t)
	addTestNewsCategory(t, base, nil, "Releases")
	title := "Café — Snowman ☃"
	fixture := rssFixture(title, "unicode", "Sat, 22 Aug 2026 18:09:55 -0700")
	manager := newTestFeedNewsManager(t, base, testNewsFeedConfig(testFeedURL, "Releases"), roundTripFunc(func(*http.Request) (*http.Response, error) {
		return testFeedResponse(http.StatusOK, fixture, nil), nil
	}), "macintosh")
	_, err := manager.ListArticles([]string{"Releases"})
	require.NoError(t, err)

	wantTitle, err := textencoding.ReplaceUnsupported(charmap.Macintosh.NewEncoder()).String(title)
	require.NoError(t, err)
	article := base.GetArticle([]string{"Releases"}, 1)
	require.NotNil(t, article)
	assert.Equal(t, wantTitle, article.Title)
	yamlData, err := os.ReadFile(base.filePath)
	require.NoError(t, err)
	assert.Contains(t, string(yamlData), "!!binary", "MacRoman bytes that are invalid UTF-8 must round-trip through YAML")

	reloaded, err := NewThreadedNewsYAML(base.filePath)
	require.NoError(t, err)
	assert.Equal(t, wantTitle, reloaded.GetArticle([]string{"Releases"}, 1).Title)

	// Changing server encoding later affects future imports only.
	utf8Manager := newTestFeedNewsManager(t, reloaded, testNewsFeedConfig(testFeedURL, "Releases"), roundTripFunc(func(*http.Request) (*http.Response, error) {
		return testFeedResponse(http.StatusOK, rssFixture("UTF-8 ☃", "unicode-2", "Sun, 23 Aug 2026 18:09:55 -0700"), nil), nil
	}), "utf8")
	_, err = utf8Manager.ListArticles([]string{"Releases"})
	require.NoError(t, err)
	assert.Equal(t, wantTitle, reloaded.GetArticle([]string{"Releases"}, 1).Title)
	assert.Equal(t, "UTF-8 ☃", reloaded.GetArticle([]string{"Releases"}, 2).Title)
}

func TestFeedNewsManager_ArticleListLimitPersistsFittingPrefixAndRetries(t *testing.T) {
	base := newTestThreadedNews(t)
	addTestNewsCategory(t, base, nil, "Releases")
	fixture := rssBatchFixture("large", 300, 255)
	var requests atomic.Int32
	manager := newTestFeedNewsManager(t, base, testNewsFeedConfig(testFeedURL, "Releases"), roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		assert.Empty(t, request.Header.Get("If-None-Match"), "validators must not advance while unseen entries remain")
		return testFeedResponse(http.StatusOK, fixture, map[string]string{"ETag": `"too-large"`}), nil
	}), "utf8")

	list, err := manager.ListArticles([]string{"Releases"})
	require.NoError(t, err)
	assert.Greater(t, list.Count, 0)
	assert.Less(t, list.Count, 300)
	encoded, err := io.ReadAll(&list)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(encoded), 65535)
	state := feedState(t, base, "Releases")
	assert.Empty(t, state.ETag)
	assert.Len(t, state.Imported, list.Count)

	second, err := manager.ListArticles([]string{"Releases"})
	require.NoError(t, err)
	assert.Equal(t, list.Count, second.Count)
	assert.Equal(t, int32(2), requests.Load(), "the first unseen item must be retried on the next load")
}

func TestFeedNewsManager_ValidatesExistingOrdinaryTarget(t *testing.T) {
	base := newTestThreadedNews(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	_, err := NewFeedNewsManager(base, []hotline.NewsFeedConfig{testNewsFeedConfig(testFeedURL, "Missing")}, logger, "test", "utf8")
	require.ErrorContains(t, err, "does not exist")
	_, err = NewFeedNewsManager(base, []hotline.NewsFeedConfig{testNewsFeedConfig(testFeedURL, "Archive")}, logger, "test", "utf8")
	require.ErrorContains(t, err, "ordinary news category")
	_, err = NewFeedNewsManager(base, []hotline.NewsFeedConfig{
		testNewsFeedConfig("https://example.com/one.xml", "General"),
		testNewsFeedConfig("https://example.com/two.xml", "General"),
	}, logger, "test", "utf8")
	require.ErrorContains(t, err, "duplicates")
}

func TestFeedNewsManager_WriteFailureRollsBackImport(t *testing.T) {
	base := newTestThreadedNews(t)
	addTestNewsCategory(t, base, nil, "Releases")
	manager := newTestFeedNewsManager(t, base, testNewsFeedConfig(testFeedURL, "Releases"), roundTripFunc(func(*http.Request) (*http.Response, error) {
		return testFeedResponse(http.StatusOK, rssFixture("One", "one", "Sat, 22 Aug 2026 18:09:55 -0700"), nil), nil
	}), "utf8")
	base.filePath = filepath.Join(t.TempDir(), "missing", "ThreadedNews.yaml")

	list, err := manager.ListArticles([]string{"Releases"})
	require.NoError(t, err, "import failures serve local news")
	assert.Zero(t, list.Count)
	item := base.NewsItem([]string{"Releases"})
	assert.Empty(t, item.Articles)
	assert.Nil(t, item.FeedState)
}

func TestFeedFetcherLimitsAndConditional304(t *testing.T) {
	t.Run("oversized response", func(t *testing.T) {
		fetcher := newFeedFetcher("test")
		fetcher.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return testFeedResponse(http.StatusOK, strings.Repeat("x", maxFeedBodyBytes+1), nil), nil
		})}
		_, err := fetcher.fetch(t.Context(), testFeedURL, feedHTTPValidators{})
		assert.ErrorIs(t, err, errFeedResponseTooLarge)
	})

	t.Run("unprompted 304", func(t *testing.T) {
		fetcher := newFeedFetcher("test")
		fetcher.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return testFeedResponse(http.StatusNotModified, "", nil), nil
		})}
		_, err := fetcher.fetch(t.Context(), testFeedURL, feedHTTPValidators{})
		require.ErrorContains(t, err, "without a conditional request")
	})
}

func TestIsPublicAddr(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"93.184.215.14", true},
		{"2606:4700::6810:84e5", true},
		{"127.0.0.1", false},
		{"::1", false},
		{"10.1.2.3", false},
		{"172.16.0.1", false},
		{"192.168.1.1", false},
		{"169.254.169.254", false},
		{"100.64.0.1", false},
		{"0.0.0.0", false},
		{"::", false},
		{"fd00::1", false},
		{"fe80::1", false},
		{"224.0.0.1", false},
		{"::ffff:127.0.0.1", false},
		{"::ffff:192.168.1.1", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, isPublicAddr(netip.MustParseAddr(tt.addr)), tt.addr)
	}
}

// publicFeedURL is a literal public address, so whether the feed is public never depends on DNS.
const publicFeedURL = "http://93.184.215.14/feed.xml"

func TestFeedFetcherKeepsAPublicFeedOffTheLocalNetwork(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/moved" {
			http.Redirect(w, request, "/feed.xml", http.StatusFound)
			return
		}
		hits.Add(1)
		_, _ = io.WriteString(w, rssFixture("Release", "release-1", "Mon, 02 Jan 2006 15:04:05 GMT"))
	}))
	defer server.Close()

	t.Run("a feed on the local network is fetched as configured", func(t *testing.T) {
		result, err := newFeedFetcher("test").fetch(t.Context(), server.URL+"/moved", feedHTTPValidators{})
		require.NoError(t, err)
		assert.Len(t, result.items, 1)
	})

	t.Run("a public feed cannot redirect into it", func(t *testing.T) {
		before := hits.Load()
		fetcher := newFeedFetcher("test")
		dialing := fetcher.client.Transport
		fetcher.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.String() == publicFeedURL {
				return testFeedResponse(http.StatusFound, "", map[string]string{"Location": server.URL + "/feed.xml"}), nil
			}
			return dialing.RoundTrip(request)
		})
		_, err := fetcher.fetch(t.Context(), publicFeedURL, feedHTTPValidators{})
		assert.ErrorIs(t, err, errFeedAddressNotPublic)
		assert.Equal(t, before, hits.Load())
	})
}

// A name server can answer loopback to the lookup that judges the feed and its real address to
// the connection, then redirect into the network. The judging lookup's answers are where the
// feed connects, so the server behind the second answer is never reached.
func TestFeedFetcherConnectsWhereTheJudgingLookupSaid(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		hits.Add(1)
		http.Redirect(w, request, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer server.Close()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)

	fetcher := newFeedFetcher("test")
	fetcher.lookup = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("127.0.0.2")}, nil
	}
	_, err = fetcher.fetch(t.Context(), "http://localhost:"+port+"/feed.xml", feedHTTPValidators{})
	require.Error(t, err)
	assert.Zero(t, hits.Load(), "connected to the second answer, not the first")
}

func TestFeedFetcherConnectsToAPrivateProxy(t *testing.T) {
	var proxied []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		proxied = append(proxied, request.URL.String())
		if request.URL.String() == publicFeedURL {
			http.Redirect(w, request, "http://169.254.169.254/feed.xml", http.StatusFound)
			return
		}
		_, _ = io.WriteString(w, rssFixture("Release", "release-1", "Mon, 02 Jan 2006 15:04:05 GMT"))
	}))
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)
	fetcher := newFeedFetcherVia("test", http.ProxyURL(proxyURL))

	result, err := fetcher.fetch(t.Context(), "http://10.1.2.3/feed.xml", feedHTTPValidators{})
	require.NoError(t, err, "a feed on the local network, through the proxy")
	assert.Len(t, result.items, 1)

	_, err = fetcher.fetch(t.Context(), publicFeedURL, feedHTTPValidators{})
	assert.ErrorIs(t, err, errFeedAddressNotPublic)
	assert.Equal(t, []string{"http://10.1.2.3/feed.xml", publicFeedURL}, proxied,
		"the redirect to a metadata address never reaches the proxy")
}

func TestFeedFetcherRefusesHTTPSDowngrade(t *testing.T) {
	fetcher := newFeedFetcher("test")
	var requested []string
	fetcher.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requested = append(requested, request.URL.String())
		return testFeedResponse(http.StatusFound, "", map[string]string{"Location": "http://feeds.example.com/feed.xml"}), nil
	})
	_, err := fetcher.fetch(t.Context(), "https://feeds.example.com/feed.xml", feedHTTPValidators{})
	require.ErrorContains(t, err, "redirected from https to http")
	assert.Equal(t, []string{"https://feeds.example.com/feed.xml"}, requested)
}

func feedFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return string(data)
}

func rssFixture(title, guid, published string) string {
	return fmt.Sprintf(`<?xml version="1.0"?><rss version="2.0"><channel><title>Releases</title><item><title>%s</title><guid>%s</guid><pubDate>%s</pubDate><description><![CDATA[<p>Notes for %s.</p>]]></description></item></channel></rss>`, title, guid, published, title)
}

func atomEntry(title, id, updated string) string {
	return fmt.Sprintf(`<entry><id>%s</id><updated>%s</updated><title>%s</title><content type="html">Notes</content><author><name>Mobius</name></author></entry>`, id, updated, title)
}

func atomBatchFixture(entries ...string) string {
	return `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><title>Releases</title>` + strings.Join(entries, "") + `</feed>`
}

func rssBatchFixture(prefix string, count, titleSize int) string {
	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0"?><rss version="2.0"><channel><title>Releases</title>`)
	for i := 0; i < count; i++ {
		title := strings.Repeat("x", titleSize)
		fmt.Fprintf(&builder, `<item><title>%s</title><guid>%s-%d</guid><description>Notes</description></item>`, title, prefix, i)
	}
	builder.WriteString(`</channel></rss>`)
	return builder.String()
}

func TestEncodeFeedTextDoesNotSplitUTF8(t *testing.T) {
	value := strings.Repeat("é", 200)
	encoded := truncateEncodedFeedText(value, 255, "utf8")
	assert.True(t, bytes.Equal([]byte(encoded), []byte(strings.Repeat("é", 127))))
}
