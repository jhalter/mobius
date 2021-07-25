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

    brew install jhalter/mobius-hotline-client/mobius-hotline-client

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

## Usage

### Precompiled binaries
To get started quickly, download the precompiled binaries for your platform:

* [Linux]()
* [Mac OS X]()

## Compatibility



# TODO

* Implement 1.5+ protocol account editing
* Implement folder transfer resume
* Implement file transfer queuing
* Map additional file extensions to file type and creator codes
* Implement file comment read/write
* Implement user banning
* Implement Maximum Simultaneous Downloads
* Maximum simultaneous downloads/client
* Maximum simultaneous connections/IP
* Implement server broadcast
* Implement statistics:
    * Currently Connected
    * Downloads in progress
    * Uploads in progress
    * Waiting Downloads
    * Connection Peak
    * Connection Counter
    * Download Counter
    * Upload Counter
    * Since


# TODO - Someday Maybe

* Implement Pitbull protocol extensions