# Text Encoding

## Background

The Hotline protocol was designed for classic Mac OS, which used an encoding called Mac Roman for text. Modern operating systems use UTF-8 instead. Mobius automatically translates between these two encodings so that classic Hotline clients and modern filesystems can work together.

By default, Mobius assumes all clients use Mac Roman encoding. If your server exclusively serves modern UTF-8 clients, you can disable the Mac Roman conversion. This document explains how encoding works and how to configure it.

## How It Works

### File and folder names are translated

When a Hotline client uploads, downloads, browses, creates, or renames files and folders, Mobius translates the names between Mac Roman and UTF-8:

- **Client to server**: File and folder names arriving from clients are converted from Mac Roman to UTF-8 before being written to the filesystem.
- **Server to client**: File and folder names read from the filesystem are converted from UTF-8 to Mac Roman before being sent to clients.

This means files on disk always use UTF-8 names, regardless of what encoding the client uses. You can place files with Unicode names in the server's file directory and clients will see them — as long as the characters have Mac Roman equivalents.

### Chat, news, and usernames are NOT translated

Text in chat messages, news articles, usernames, and private messages is passed through as raw bytes with no encoding conversion. This means:

- If all your users are on classic Mac clients, they'll see each other's text correctly (all Mac Roman).
- If all your users are on modern UTF-8 clients, they'll also see each other's text correctly.
- If you have a mix of classic and modern clients, users may occasionally see garbled characters in chat and news from users on a differently-encoded client.

There is no way to configure this behavior — the Hotline protocol has no mechanism for clients to declare what encoding they use.

## Configuration

The `Encoding` field in `config.yaml` controls how file and folder names are translated:

```yaml
# Default — translates between Mac Roman and UTF-8 (compatible with classic Hotline clients)
Encoding: macintosh

# No-op — passes file names through without conversion (for servers with only modern UTF-8 clients)
Encoding: utf8
```

If `Encoding` is omitted, it defaults to `macintosh`, preserving backward compatibility with existing configurations.

**Impact of `utf8` on classic Mac clients**: With `Encoding: utf8`, the server no longer translates between Mac Roman and UTF-8. Classic Hotline clients send file and folder names encoded as Mac Roman, and with this setting those bytes are stored on disk without conversion. This means non-ASCII characters (accented letters, curly quotes, etc.) will be written to the filesystem as their raw Mac Roman byte values, producing mojibake in file names when viewed on the host OS. Going the other direction, UTF-8 file names on disk will be sent to classic clients without conversion, so any characters that differ between UTF-8 and Mac Roman will display incorrectly on the client side. **Only use `utf8` if you are confident that no classic Mac clients will connect to your server.**

## Practical Implications

### Server file root path

The server's configured file root path (the directory that holds your shared files) can safely contain non-ASCII characters, such as `/srv/données` or `/home/café/files`. Only the client-provided portion of file paths is subject to encoding translation — the server root path is treated as native UTF-8.

### Characters outside Mac Roman

Mac Roman supports 256 characters, covering most Western European languages. If a file on disk has a name containing characters outside the Mac Roman set (e.g., Chinese, Japanese, Cyrillic, or certain symbols), those characters cannot be represented when sent to clients. They will be replaced with a fallback character.

### Mixing client types

Modern Hotline clients that send UTF-8 for file operations may produce unexpected file names on disk, since the server assumes all clients send Mac Roman. If you primarily serve modern clients, consider setting `Encoding: utf8` in your config to disable the Mac Roman conversion.

## Summary

| What                      | Encoding translation? | Notes                                         |
|---------------------------|----------------------|-----------------------------------------------|
| File and folder names     | Yes                  | Mac Roman <-> UTF-8 at the filesystem boundary |
| Chat messages             | No                   | Raw bytes, passed through as-is               |
| News articles and titles  | No                   | Raw bytes, passed through as-is               |
| Usernames                 | No                   | Raw bytes, passed through as-is               |
| Private messages          | No                   | Raw bytes, passed through as-is               |
| File comments             | No                   | Stored and retrieved as raw bytes             |
| Login credentials         | No                   | Obfuscated, no charset conversion             |
