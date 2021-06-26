# NAME_TBD

NAME_TBD implements the [Hotline protocol](https://en.wikipedia.org/wiki/Hotline_Communications)

## Features

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

### Docker

```
docker run --mount type=bind,source=$HOME/hotline-root/config/,destination=/app/server/config/ -p 5500-5502:5500-5502 -it go-hotline:latest /app/server/server --config /app/server/config/
```

### Build from source

	make build