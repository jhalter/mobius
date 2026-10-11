package mobius

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	stdhtml "html"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/jhalter/mobius/hotline"
	"github.com/mmcdole/gofeed"
	xhtml "golang.org/x/net/html"
)

const (
	feedRequestTimeout = 10 * time.Second
	maxFeedBodyBytes   = 2 << 20
	maxNewsStringBytes = 255
	maxNewsBodyBytes   = 65535
)

var (
	errFeedResponseTooLarge = errors.New("feed response exceeds 2 MiB")
	errFeedAddressNotPublic = errors.New("feed address is not public")
)

// sharedAddressSpace is carrier-grade NAT space (RFC 6598), which netip does not count as private.
var sharedAddressSpace = netip.MustParsePrefix("100.64.0.0/10")

func isPublicAddr(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !sharedAddressSpace.Contains(ip)
}

// privateFeed marks a fetch whose configured URL is itself on a non-public address. The operator
// chose that, so nothing it connects to is refused. For a host name, addrs are the answers that
// decided it, and connections to that host go to them: a second lookup could answer otherwise, and
// a name server answering loopback to the check and a public address to the connection would have
// its redirects followed unchecked.
type privateFeed struct {
	host  string
	addrs []netip.Addr
}

type privateFeedKey struct{}

// privateFeedFor reports whether the configured URL names a non-public address, or a host whose
// every address is one. Behind a proxy, which resolves names where this cannot see, only an address
// makes a feed private, and a host that does not resolve here counts as public.
func (f *feedFetcher) privateFeedFor(ctx context.Context, u *url.URL, proxied bool) *privateFeed {
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil {
		if isPublicAddr(ip) {
			return nil
		}
		return &privateFeed{}
	}
	if proxied {
		return nil
	}
	addrs, err := f.lookup(ctx, u.Hostname())
	if err != nil || len(addrs) == 0 || slices.ContainsFunc(addrs, isPublicAddr) {
		return nil
	}
	return &privateFeed{host: u.Hostname(), addrs: addrs}
}

// refuseNonPublicDial is checked against the resolved address of every direct connection a public
// feed's fetch makes, so a redirect it serves cannot point the server at loopback, the local
// network, or a cloud metadata endpoint.
func refuseNonPublicDial(ctx context.Context, _, address string, _ syscall.RawConn) error {
	if ctx.Value(privateFeedKey{}) != nil {
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return err
	}
	if !isPublicAddr(ip) {
		return fmt.Errorf("%w: %s", errFeedAddressNotPublic, ip)
	}
	return nil
}

type normalizedFeedItem struct {
	identity    string
	article     hotline.NewsArtData
	publishedAt time.Time
	sourceIndex int
}

type feedHTTPValidators struct {
	etag         string
	lastModified string
}

type feedFetchResult struct {
	notModified       bool
	etag              string
	lastModified      string
	items             []normalizedFeedItem
	skippedNoIdentity int
}

type feedFetcher struct {
	client    *http.Client
	parser    *gofeed.Parser
	now       func() time.Time
	userAgent string
	proxy     func(*http.Request) (*url.URL, error)
	lookup    func(ctx context.Context, host string) ([]netip.Addr, error)
}

func newFeedFetcher(userAgent string) *feedFetcher {
	return newFeedFetcherVia(userAgent, http.ProxyFromEnvironment)
}

// newFeedFetcherVia fetches through the proxies proxy chooses. A proxy is the operator's choice,
// and is commonly on the local network, so connections to it are not checked; through it the
// server cannot see what a host name resolves to, so for a public feed only an address written
// into a URL is.
func newFeedFetcherVia(userAgent string, proxy func(*http.Request) (*url.URL, error)) *feedFetcher {
	if userAgent == "" {
		userAgent = "mobius-hotline-server"
	}
	var proxies sync.Map
	direct := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	guarded := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second, ControlContext: refuseNonPublicDial}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		proxyURL, err := proxy(req)
		if err != nil || proxyURL == nil {
			return proxyURL, err
		}
		ip, err := netip.ParseAddr(req.URL.Hostname())
		if err == nil && !isPublicAddr(ip) && req.Context().Value(privateFeedKey{}) == nil {
			return nil, fmt.Errorf("%w: %s", errFeedAddressNotPublic, ip)
		}
		proxies.Store(proxyAddr(proxyURL), struct{}{})
		return proxyURL, nil
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if _, ok := proxies.Load(address); ok {
			return direct.DialContext(ctx, network, address)
		}
		if pin, _ := ctx.Value(privateFeedKey{}).(*privateFeed); pin != nil && pin.addrs != nil {
			if host, port, err := net.SplitHostPort(address); err == nil && host == pin.host {
				return dialFirst(ctx, direct, network, pin.addrs, port)
			}
		}
		return guarded.DialContext(ctx, network, address)
	}
	return &feedFetcher{
		client: &http.Client{
			Transport: transport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return errors.New("stopped after 10 redirects")
				}
				if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
					return fmt.Errorf("redirected to unsupported URL scheme %q", req.URL.Scheme)
				}
				if via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme == "http" {
					return errors.New("redirected from https to http")
				}
				if req.URL.User != nil {
					return errors.New("redirected to a URL containing credentials")
				}
				return nil
			},
		},
		parser:    gofeed.NewParser(),
		now:       time.Now,
		userAgent: userAgent,
		proxy:     proxy,
		lookup: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		},
	}
}

// dialFirst connects to the first of addrs that answers.
func dialFirst(ctx context.Context, d *net.Dialer, network string, addrs []netip.Addr, port string) (net.Conn, error) {
	var err error
	for _, addr := range addrs {
		var conn net.Conn
		if conn, err = d.DialContext(ctx, network, net.JoinHostPort(addr.Unmap().String(), port)); err == nil {
			return conn, nil
		}
	}
	return nil, err
}

// proxyAddr is the address the transport dials for proxyURL.
func proxyAddr(proxyURL *url.URL) string {
	if port := proxyURL.Port(); port != "" {
		return net.JoinHostPort(proxyURL.Hostname(), port)
	}
	port := map[string]string{"http": "80", "https": "443", "socks5": "1080", "socks5h": "1080"}[proxyURL.Scheme]
	return net.JoinHostPort(proxyURL.Hostname(), port)
}

func (f *feedFetcher) fetch(ctx context.Context, sourceURL string, validators feedHTTPValidators) (feedFetchResult, error) {
	ctx, cancel := context.WithTimeout(ctx, feedRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return feedFetchResult{}, errors.New("create request from configured feed URL")
	}
	proxyURL, _ := f.proxy(req)
	if pin := f.privateFeedFor(ctx, req.URL, proxyURL != nil); pin != nil {
		req = req.WithContext(context.WithValue(ctx, privateFeedKey{}, pin))
	}
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml;q=0.9, text/xml;q=0.8")
	req.Header.Set("User-Agent", f.userAgent)
	if validators.etag != "" {
		req.Header.Set("If-None-Match", validators.etag)
	}
	if validators.lastModified != "" {
		req.Header.Set("If-Modified-Since", validators.lastModified)
	}

	response, err := f.client.Do(req)
	if err != nil {
		return feedFetchResult{}, fmt.Errorf("fetch feed: %w", redactHTTPError(err))
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode == http.StatusNotModified {
		if validators.etag == "" && validators.lastModified == "" {
			return feedFetchResult{}, errors.New("feed returned 304 without a conditional request")
		}
		return feedFetchResult{notModified: true}, nil
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return feedFetchResult{}, fmt.Errorf("feed returned %s", response.Status)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxFeedBodyBytes+1))
	if err != nil {
		return feedFetchResult{}, fmt.Errorf("read feed response: %w", err)
	}
	if len(body) > maxFeedBodyBytes {
		return feedFetchResult{}, errFeedResponseTooLarge
	}
	items, skipped, err := f.parseFeedResponse(body, f.now())
	if err != nil {
		return feedFetchResult{}, err
	}

	return feedFetchResult{
		etag:              response.Header.Get("ETag"),
		lastModified:      response.Header.Get("Last-Modified"),
		items:             items,
		skippedNoIdentity: skipped,
	}, nil
}

func (f *feedFetcher) parseFeedResponse(body []byte, fetchedAt time.Time) ([]normalizedFeedItem, int, error) {
	detected := gofeed.DetectFeedType(bytes.NewReader(body))
	if detected != gofeed.FeedTypeRSS && detected != gofeed.FeedTypeAtom {
		return nil, 0, fmt.Errorf("unsupported feed format %q; expected RSS or Atom", detected)
	}
	feed, err := f.parser.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("parse feed type %v: %w", detected, err)
	}
	items, skipped := normalizeSyndicationFeed(feed, fetchedAt)
	return items, skipped, nil
}

func redactHTTPError(err error) error {
	for {
		var urlError *url.Error
		if !errors.As(err, &urlError) || urlError.Err == nil || urlError.Err == err {
			return err
		}
		err = urlError.Err
	}
}

func normalizeSyndicationFeed(feed *gofeed.Feed, fetchedAt time.Time) ([]normalizedFeedItem, int) {
	items := make([]normalizedFeedItem, 0, len(feed.Items))
	seen := make(map[string]struct{}, len(feed.Items))
	skipped := 0
	for sourceIndex, item := range feed.Items {
		if item == nil {
			continue
		}
		identity, ok := feedItemIdentity(item)
		if !ok {
			skipped++
			continue
		}
		if _, exists := seen[identity]; exists {
			continue
		}
		seen[identity] = struct{}{}

		publishedAt := fetchedAt
		if item.PublishedParsed != nil {
			publishedAt = *item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			publishedAt = *item.UpdatedParsed
		}
		rawBody := firstNonEmpty(strings.TrimSpace(item.Content), strings.TrimSpace(item.Description))
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = "(untitled)"
		}
		poster := feedItemPoster(item, feed, "News Feed")
		items = append(items, normalizedFeedItem{
			identity: identity,
			article: hotline.NewsArtData{
				Title:    title,
				Poster:   poster,
				Date:     hotline.NewNewsTime(publishedAt),
				DataFlav: slices.Clone(hotline.NewsFlavor),
				Data:     feedArticleBody(item, rawBody),
			},
			publishedAt: publishedAt,
			sourceIndex: sourceIndex,
		})
	}

	// Most feeds are newest-first. Dates establish the real order; reversing
	// source order resolves equal or missing dates so newer items receive newer
	// ordinary Hotline article IDs.
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].publishedAt.Equal(items[j].publishedAt) {
			return items[i].sourceIndex > items[j].sourceIndex
		}
		return items[i].publishedAt.Before(items[j].publishedAt)
	})
	return items, skipped
}

func feedItemIdentity(item *gofeed.Item) (string, bool) {
	if value := strings.TrimSpace(item.GUID); value != "" {
		return "id:" + value, true
	}
	if value := firstHTTPURL(append([]string{item.Link}, item.Links...)...); value != "" {
		return "link:" + value, true
	}
	for _, enclosure := range item.Enclosures {
		if enclosure != nil {
			if value := safeHTTPURL(enclosure.URL); value != "" {
				return "enclosure:" + value, true
			}
		}
	}
	return "", false
}

func feedItemPoster(item *gofeed.Item, feed *gofeed.Feed, fallback string) string {
	for _, author := range item.Authors {
		if author != nil && strings.TrimSpace(author.Name) != "" {
			return strings.TrimSpace(author.Name)
		}
	}
	if item.Author != nil && strings.TrimSpace(item.Author.Name) != "" {
		return strings.TrimSpace(item.Author.Name)
	}
	for _, author := range feed.Authors {
		if author != nil && strings.TrimSpace(author.Name) != "" {
			return strings.TrimSpace(author.Name)
		}
	}
	if feed.Author != nil && strings.TrimSpace(feed.Author.Name) != "" {
		return strings.TrimSpace(feed.Author.Name)
	}
	if strings.TrimSpace(feed.Title) != "" {
		return strings.TrimSpace(feed.Title)
	}
	return fallback
}

func feedArticleBody(item *gofeed.Item, rawBody string) string {
	body := markupToPlainText(rawBody)
	links := make([]string, 0, len(item.Enclosures)+2)
	if source := firstHTTPURL(append([]string{item.Link}, item.Links...)...); source != "" {
		links = append(links, "Source: "+source)
	}
	if releaseNotes := extensionValue(item, "sparkle", "releaseNotesLink"); releaseNotes != "" {
		if releaseNotes = safeHTTPURL(releaseNotes); releaseNotes != "" {
			links = appendUnique(links, "Release notes: "+releaseNotes)
		}
	}
	for _, enclosure := range item.Enclosures {
		if enclosure == nil {
			continue
		}
		if download := safeHTTPURL(enclosure.URL); download != "" {
			links = appendUnique(links, "Download: "+download)
		}
	}
	if len(links) > 0 {
		if body != "" {
			body += "\r\r"
		}
		body += strings.Join(links, "\r")
	}
	return body
}

func extensionValue(item *gofeed.Item, namespace, name string) string {
	if item.Extensions == nil || item.Extensions[namespace] == nil {
		return ""
	}
	values := item.Extensions[namespace][name]
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0].Value)
}

func markupToPlainText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "<") {
		return normalizePlainText(value)
	}

	tokenizer := xhtml.NewTokenizer(strings.NewReader(value))
	var out strings.Builder
	var anchors []string
	skipDepth := 0
	for {
		tokenType := tokenizer.Next()
		switch tokenType {
		case xhtml.ErrorToken:
			return normalizePlainText(out.String())
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			token := tokenizer.Token()
			tag := strings.ToLower(token.Data)
			if tag == "script" || tag == "style" {
				if tokenType == xhtml.StartTagToken {
					skipDepth++
				}
				continue
			}
			if skipDepth > 0 {
				continue
			}
			switch tag {
			case "br":
				out.WriteByte('\n')
			case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "blockquote", "pre":
				writeLineBreak(&out, 2)
			case "li":
				writeLineBreak(&out, 1)
				out.WriteString("- ")
			case "a":
				anchors = append(anchors, safeHTTPURL(attributeValue(token.Attr, "href")))
			}
		case xhtml.EndTagToken:
			token := tokenizer.Token()
			tag := strings.ToLower(token.Data)
			if tag == "script" || tag == "style" {
				if skipDepth > 0 {
					skipDepth--
				}
				continue
			}
			if skipDepth > 0 {
				continue
			}
			if tag == "a" && len(anchors) > 0 {
				href := anchors[len(anchors)-1]
				anchors = anchors[:len(anchors)-1]
				if href != "" {
					out.WriteString(" (")
					out.WriteString(href)
					out.WriteByte(')')
				}
			}
			switch tag {
			case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "blockquote", "pre":
				writeLineBreak(&out, 2)
			case "li":
				writeLineBreak(&out, 1)
			}
		case xhtml.TextToken:
			if skipDepth > 0 {
				continue
			}
			text := strings.Join(strings.Fields(stdhtml.UnescapeString(string(tokenizer.Text()))), " ")
			if text == "" {
				continue
			}
			if needsSpace(out.String()) {
				out.WriteByte(' ')
			}
			out.WriteString(text)
		}
	}
}

func normalizePlainText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			if len(out) > 0 && !blank {
				out = append(out, "")
				blank = true
			}
			continue
		}
		out = append(out, line)
		blank = false
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\r")
}

func writeLineBreak(out *strings.Builder, count int) {
	value := out.String()
	newlines := 0
	for i := len(value) - 1; i >= 0 && value[i] == '\n'; i-- {
		newlines++
	}
	for newlines < count {
		out.WriteByte('\n')
		newlines++
	}
}

func needsSpace(value string) bool {
	if value == "" {
		return false
	}
	last := value[len(value)-1]
	return last != ' ' && last != '\n' && last != '\t' && last != '-'
}

func attributeValue(attrs []xhtml.Attribute, name string) string {
	for _, attr := range attrs {
		if strings.EqualFold(attr.Key, name) {
			return strings.TrimSpace(attr.Val)
		}
	}
	return ""
}

func safeHTTPURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	return parsed.String()
}

func firstHTTPURL(values ...string) string {
	for _, value := range values {
		if parsed := safeHTTPURL(value); parsed != "" {
			return parsed
		}
	}
	return ""
}

func appendUnique(values []string, value string) []string {
	if !slices.Contains(values, value) {
		return append(values, value)
	}
	return values
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func truncateUTF8(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
