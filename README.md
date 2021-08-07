# Mobius

Mobius is a cross-platform command line [Hotline](https://en.wikipedia.org/wiki/Hotline_Communications) server, client, and library developed in Golang.

The project aims to support Hotline functionality from versions v1.2.3 and >v1.5 (e.g. threaded news and folder transfers).

## Project status

### Server

* Near feature complete

### Client

* Early stage with functionality for chat and news posting only

# Getting started

### Mac OS

For Mac OS the easiest path to installation is to install through Homebrew.

#### Client

    brew install jhalter/mobius-hotline-client/mobius-hotline-client

After installation `mobius-hotline-client` installed to `/usr/local/bin/mobius-hotline-client` and should be in your $PATH. 

The client config file is in `/usr/local/etc/mobius-client-config.yaml`

Run `mobius-hotline-client -help` for usage options.

#### Server

    brew install jhalter/mobius-hotline-server/mobius-hotline-server

After installation `mobius-hotline-server` installed to `/usr/local/bin/mobius-hotline-server` and should be in your $PATH.

The server config file directory is under `/usr/local/var/mobius` which by default contains:

    /usr/local/var/mobius/config/MessageBoard.txt
    /usr/local/var/mobius/config/config.yaml
    /usr/local/var/mobius/config/ThreadedNews.yaml
    /usr/local/var/mobius/config/Agreement.txt
    /usr/local/var/mobius/config/Users/guest.yaml
    /usr/local/var/mobius/config/Users/admin.yaml

Edit `/usr/local/var/mobius/config/config.yaml` to change your server name and other settings.

Edit `/usr/local/var/mobius/config/Agreement.txt` to set your server agreement.

Run `mobius-hotline-server -help` for usage options.

### Linux

Download a compiled release for your architecture from the Releases page

### Windows

    TODO


### Build from source

To build from source, run:

    make build-client
    make build-server
