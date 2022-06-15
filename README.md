<picture>
  <source media="(prefers-color-scheme: dark)" srcset="dark_logo.png">
  <source media="(prefers-color-scheme: light)" srcset="light_logo.png">
  <img src="dark_logo.png" alt="Mobius Logo">
</picture>

# Mobius

Mobius is a cross-platform command line [Hotline](https://en.wikipedia.org/wiki/Hotline_Communications) client and server implemented in Golang.

The goal of Mobius server is to make it simple to run a Hotline server on macOS, Linux, and Windows, with full compatibility for all popular Hotline clients.

The goal of the Mobius client is to make it fun and easy to connect to multiple Hotline servers through a [cool retro terminal UI](https://github.com/jhalter/mobius/wiki/Mobius-Client-Screenshot-Gallery).

## Getting started

### Docker

If you run Docker, you can quickly try out the Mobius server with the official image from Docker hub:

    docker run --rm -p 5500:5500 -p 5501:5501 jhalter/mobius-hotline-server:latest

This will start the Mobius server with the Hotline ports 5500 and 5501 exposed on localhost using a default configuration from the image.

To edit the configuration and serve files from your host OS, include the `-v` option to setup a Docker [bind mount](https://docs.docker.com/storage/bind-mounts/):

	export HLFILES=/Users/foo/HotlineFiles #
 	docker run --rm -p 5500:5500 -p 5501:5501 -v $HLFILES:/usr/local/var/mobius/config jhalter/mobius-hotline-server:latest -init

You'll now find a configuration directory on your host OS populated with a default configuration:

```
❯ ls -al $HLFILES
total 32
drwxr-xr-x   8 jhalter  staff   256 Jun 12 17:11 .
drwxr-x---+ 49 jhalter  staff  1568 Jun 12 17:11 ..
-rw-r--r--   1 jhalter  staff    38 Jun 12 17:11 Agreement.txt
drwxr-xr-x   3 jhalter  staff    96 Jun 12 17:11 Files
-rw-r--r--   1 jhalter  staff    19 Jun 12 17:11 MessageBoard.txt
-rw-r--r--   1 jhalter  staff    15 Jun 12 17:11 ThreadedNews.yaml
drwxr-xr-x   4 jhalter  staff   128 Jun 12 17:11 Users
-rw-r--r--   1 jhalter  staff   313 Jun 12 17:11 config.yaml
```

Edit `config.yaml` to get started personalizing your server.


### Mac OS

For Mac OS the easiest path to installation is through Homebrew.

#### Client

To install the client:

    brew install jhalter/mobius-hotline-client/mobius-hotline-client

Then run `mobius-hotline-client` to get started.

#### Server

To install the server:

    brew install jhalter/mobius-hotline-server/mobius-hotline-server

After installation `mobius-hotline-server` will be installed at `$HOMEBREW_PREFIX/bin/mobius-hotline-server` and should be in your $PATH.

The server config file directory is under `$HOMEBREW_PREFIX/var/mobius` which by default contains:

    /opt/homebrew/mobius/config/MessageBoard.txt
    /opt/homebrew/var/mobius/config/config.yaml
    /opt/homebrew/var/mobius/config/ThreadedNews.yaml
    /opt/homebrew/var/mobius/config/Agreement.txt
    /opt/homebrew/var/mobius/config/Users/guest.yaml
    /opt/homebrew/var/mobius/config/Users/admin.yaml

Edit `/usr/local/var/mobius/config/config.yaml` to change your server name and other settings.

Edit `/usr/local/var/mobius/config/Agreement.txt` to set your server agreement.

Run `mobius-hotline-server -help` for usage options.

### Linux

Download a compiled release for your architecture from the Releases page or follow build from source instructions

### Windows

    TODO

### Build from source

To build from source, run:

    make all

Then grab your desired build from `dist`
