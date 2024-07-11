<!--<picture>
  <source media="(prefers-color-scheme: dark)" srcset="dark_logo.png">
  <source media="(prefers-color-scheme: light)" srcset="light_logo.png">
  <img src="dark_logo.png" alt="Mobius Logo">
</picture>
-->

# Mobius

Mobius is a cross-platform command line [Hotline](https://en.wikipedia.org/wiki/Hotline_Communications) client and server implemented in Golang.

- **Server Goal:** Make it simple to run a Hotline server on macOS, Linux, and Windows, with full compatibility for all popular Hotline clients.
- **Client Goal:** Make it fun and easy to connect to multiple Hotline servers through a [terminal UI](https://github.com/jhalter/mobius/wiki/Mobius-Client-Screenshot-Gallery).


## Installation

Mobius is distributed through a single binary.

Depending on your platform and preferences, you can acquire the binary in the following ways:

### Build from source

1. Install [Go](https://go.dev) if needed
2. Run `make server`

### Download pre-built release

See [Releases](https://github.com/jhalter/mobius/releases) page.


### Docker

To run a Hotline server with a default, sample configuration with ports forwarded from the host OS to the container:

	docker run --rm -p 5500:5500 -p 5501:5501 ghcr.io/jhalter/mobius:latest

You can now connect to localhost:5500 with your favorite Hotline client and play around, but all changes will be lost on container restart.

To serve files from the host OS and persist configuration changes, you'll want to set up a [bind mount](https://docs.docker.com/storage/bind-mounts/) that maps a directory from the host into the container.

To do this, create a directory in a location of your choice on the host OS.  For clarity, we'll assign the path to the `HLFILES` environment variable and re-use it.

Then run the docker command with `-v` and `-init` like so:

```
export HLFILES=/home/myuser/hotline-files
mdkir $HLFILES

sudo docker run \
    --pull=always \
    --rm \
    -p 5500:5500 \
    -p 5501:5501 \
    -v $HLFILES:/usr/local/var/mobius/config \
    ghcr.io/jhalter/mobius:latest \
    -init
```

It's a good security practice to run your server as a non-root user, which also happens to make editing the configuration files easier from the host OS.

To do this, add the `--user` flag to the docker run arguments with a user ID and group ID of a user on the host OS.

`--user 1001:1001`

### Homebrew

For macOS the easiest path to installation is through Homebrew, as this works around Apple's notarization requirements for downloaded pre-compiled binaries by compiling the binary on your system during brew installation.

To install the server:

    brew install jhalter/mobius-hotline-server/mobius-hotline-server

After installation `mobius-hotline-server` will be installed at `$HOMEBREW_PREFIX/bin/mobius-hotline-server` and should be in your $PATH.

The server config directory will be created under `$HOMEBREW_PREFIX/var/mobius`.

To start the service:

`brew services start mobius-hotline-server`


## Server Configuration

After you have a server binary, the next step is to configure the server.

### Configuration directory

Mobius stores its configuration and server state in a directory tree:

```
config
├── Agreement.txt
├── Files
│   └── hello.txt
├── MessageBoard.txt
├── ThreadedNews.yaml
├── Users
│   ├── admin.yaml
│   └── guest.yaml
├── banner.jpg
└── config.yaml
```

If you acquired the server binary by downloading it or compiling it, this directory doesn't exist yet!  But you can generate it by running the the server with the `-init` flag:

`./mobius-hotline-server -init -config example-config-dir`

Brew users can find the config directory in `$HOMEBREW_PREFIX/var/mobius`.

Within this directory some files are intended to be edited to customize the server, while others are not.

--- 

* 🛠️ Edit this file to customize your server.
* ⚠️ Avoid manual edits outside of special circumstances (e.g. remove offending news content).

---

🛠️ `Agreement.text` - The server agreement sent to users after they join the server.

🛠️ `Files` - Home of your warez or any other files you'd like to share.

⚠️ `MessageBoard.txt` - Plain text file containing the server's message board.  No need to edit this.

⚠️ `ThreadedNews.yaml` - YAML file containing the server's threaded news.  No need to edit this.

⚠️ `Users` - Directory containing user account YAML files.  No need to edit this.

🛠️ `banner.jpg` - Path to server banner image.

🛠️ `config.yaml` - Edit to set your server name, description, and enable tracker registration.


### User accounts

The default installation includes two users: 

* guest (no password) 
* admin (default password admin).

User administration should be performed from a Hotline client.  Avoid editing the files under the `Users` directory.

## Run the server

By default running `mobius-hotline-server` will listen on ports 5500/5501 of all interfaces with info level logging to STDOUT.

Use the -h or -help flag for a list of options:

```
$ mobius-hotline-server -h
Usage of mobius-hotline-server:
  -bind int
    	Base Hotline server port.  File transfer port is base port + 1. (default 5500)
  -config string
    	Path to config root (default "/usr/local/var/mobius/config/")
  -init
    	Populate the config dir with default configuration
  -interface string
    	IP addr of interface to listen on.  Defaults to all interfaces.
  -log-file string
    	Path to log file
  -log-level string
    	Log level (default "info")
  -stats-port string
    	Enable stats HTTP endpoint on address and port
  -version
    	Print version and exit
```


To run as a systemd service, refer to this sample unit file: [mobius-hotline-server.service](https://github.com/jhalter/mobius/blob/master/cmd/mobius-hotline-server/mobius-hotline-server.service)

