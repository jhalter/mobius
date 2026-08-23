# Feed-backed threaded news

Mobius can import a public RSS or Atom feed into an existing Hotline news
category. This is useful for project announcements, Sparkle appcasts, and
GitHub release feeds.

When a user opens a mapped category, Mobius checks its source and adds entries
it has not seen before as ordinary root articles. Imported articles live in
`ThreadedNews.yaml`, alongside locally posted articles and replies. The category
continues to work as ordinary Hotline news: authorized users can post roots,
reply, and delete any article.

## Configure a category

First, create the target as an ordinary news category with a Hotline client.
Every component in `CategoryPath`, including the final category, must already
exist before the server starts.

Then add a mapping to `config.yaml` and restart Mobius:

```yaml
NewsFeeds:
  - CategoryPath: ["Software Updates", "Afterglow"]
    URL: "https://morphing.cloud/afterglow/appcast.xml"

  - CategoryPath: ["Software Updates", "Mobius"]
    URL: "https://github.com/jhalter/mobius/releases.atom"
```

`CategoryPath` is the complete path from the root of threaded news to the
existing category. `URL` must be an absolute public `http` or `https` URL and
cannot contain embedded credentials. Only one feed may map to a category.

Mobius detects RSS and Atom automatically. Sparkle appcasts are RSS; GitHub's
`releases.atom` endpoints are Atom. JSON Feed and provider-specific APIs are not
supported.

## Import behavior

Every article-list request for the mapped category performs a source check.
Mobius sends the saved `ETag` and `Last-Modified` values when the source
provides them, allowing an unchanged source to answer with a small `304 Not
Modified` response. Concurrent requests for the same category share one
in-flight check.

On a successful response, Mobius:

1. Finds entries not previously observed from that URL.
2. Converts HTML descriptions to plain text and retains useful source, release
   notes, and enclosure links.
3. Imports new entries oldest-first as ordinary root articles.
4. Saves the articles and import metadata together in one atomic update to
   `ThreadedNews.yaml`.

An entry needs a stable identity: a GUID or Atom ID, an HTTP(S) article link,
or an HTTP(S) enclosure URL, in that order. Entries without any of these are
skipped and logged. Mobius intentionally does not derive identities from titles
or bodies because an edited entry could then be mistaken for a new article.

Once an identity has been imported, later changes to that entry's title,
author, date, or body are ignored. If an imported article is deleted locally,
its identity remains in the seen set, so the feed does not recreate it. Items
also remain in Hotline when they disappear from the source. This prevents a
feed's rolling window from rolling articles out of the Hotline category.

The initial import can only include entries returned by the source. Mobius does
not paginate GitHub or reconstruct older releases that are already absent from
its Atom feed.

## Failures and limits

A network error, timeout, non-success HTTP response, invalid feed, or failed
disk write is logged. Mobius still returns the category's current local article
list, including an empty list before the first successful import. A later
category load tries the source again. Requests time out after 10 seconds and a
response body may be at most 2 MiB.

The Hotline protocol limits the complete encoded article-list payload for one
category to 65,535 bytes. This listing contains article metadata—not article
bodies—so the practical capacity depends mostly on the number and encoded
length of titles and authors. Mobius checks the full list after each candidate
article. It saves the oldest prefix that fits and stops before an addition
would exceed the limit. It does not save new HTTP validators while unseen
entries remain, so the next category load fetches and retries them. Mobius does
not prune local or imported articles automatically; split a large source across
categories when a category approaches this protocol limit.

Individual imported titles and authors are limited to 255 encoded bytes, and
article bodies to 65,535 encoded bytes, matching Hotline's field sizes.

## Durable state and backups

Feed history is part of each category in `ThreadedNews.yaml` under a YAML-only
`FeedState` key. It records the current source URL, HTTP validators, and hashes
of identities that have already been imported. This metadata is not sent to
Hotline clients. Back up `ThreadedNews.yaml` as usual; there are no additional
feed state or cache files.

Changing a category's URL clears its HTTP validators but retains its seen
history. Identity hashes include the source URL, so the new source can import
its entries even if it uses the same GUID values. Switching back to a previous
URL does not duplicate entries that URL imported earlier.

The earlier experimental `FeedNewsState.yaml` and `FeedNewsCache.json` formats
are not migrated or read. If they exist from a development build, Mobius
ignores them; their contents do not participate in this feature.

## Text encoding

Feed parsers produce UTF-8 text. Mobius converts each entry once, when it is
imported, according to the server-wide `Encoding` setting. The default
`macintosh` setting converts to Mac Roman and replaces unsupported characters;
`utf8` stores UTF-8 unchanged. Existing imported articles are ordinary raw-byte
news data, so changing `Encoding` affects only future imports and does not
rewrite history.

## Troubleshooting

- If startup fails, verify that every mapped path already exists and ends at a
  news category rather than a bundle. Also check for duplicate paths and
  invalid or credential-bearing URLs.
- If new entries do not appear, open the category and inspect the server log.
  Confirm that the URL is public RSS or Atom and that entries have stable IDs,
  links, or enclosures.
- If old releases are missing on the first import, inspect the source feed. Its
  published window is the initial cutoff; Mobius does not use pagination or a
  provider API.
- If a source is unavailable, existing local news remains usable and the next
  category load retries it.
