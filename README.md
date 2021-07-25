# Mobius

Cross-platform command line [Hotline](https://en.wikipedia.org/wiki/Hotline_Communications) server and client

[![CircleCI](https://circleci.com/gh/jhalter/mobius/tree/master.svg?style=svg&circle-token=7123999e4511cf3eb93d76de98b614a803207bea)](https://circleci.com/gh/jhalter/mobius/tree/master)

# Installation

### Mac OS X

#### Client

    brew install jhalter/mobius-hotline-client/mobius-hotline-client

#### Server

    brew install jhalter/mobius-hotline-client/mobius-hotline-client

### Linux

Download a compiled release for your architecture from the Releases page

### Windows

    TODO

# Build

To build from source, run
`make build`

# Features

* Hotline 1.2.3

## Usage

### Precompiled binaries
To get started quickly, download the precompiled binaries for your platform:

* [Linux]()
* [Mac OS X]()

## Compatibility

The server has been tested with:
 * Hotline Client version 1.2.3
 * Hotline Client version 1.8.2   
 * Hotline Client version 1.9.2
 * Nostalgia

### Build from source

	make build


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