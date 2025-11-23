# TLS Support

Mobius supports TLS (Transport Layer Security) for encrypted connections between clients and the server. When enabled, TLS runs on separate ports alongside the standard unencrypted ports, allowing both secure and legacy client connections simultaneously.

## Ports

| Service       | Standard Port | TLS Port (default) |
|---------------|---------------|-------------------|
| Hotline       | 5500          | 5600              |
| File Transfer | 5501          | 5601              |

## Generating Certificates

### Self-Signed Certificate (Testing/Private Use)

For testing or private servers, you can generate a self-signed certificate using OpenSSL:

```bash
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -days 365 -nodes -subj "/CN=localhost"
```

This creates:
- `server.key` - Private key file
- `server.crt` - Certificate file

For a certificate that includes your server's hostname or IP address:

```bash
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -days 365 -nodes \
  -subj "/CN=your-hostname.example.com" \
  -addext "subjectAltName=DNS:your-hostname.example.com,IP:192.168.1.100"
```

### Let's Encrypt (Production)

For production servers with a public domain name, use [Let's Encrypt](https://letsencrypt.org/) with certbot:

```bash
certbot certonly --standalone -d your-hostname.example.com
```

The certificates are typically stored at:
- `/etc/letsencrypt/live/your-hostname.example.com/fullchain.pem`
- `/etc/letsencrypt/live/your-hostname.example.com/privkey.pem`

## Command-Line Options

| Flag | Description | Default |
|------|-------------|---------|
| `-tls-cert` | Path to TLS certificate file | (none) |
| `-tls-key` | Path to TLS private key file | (none) |
| `-tls-port` | Base TLS port (file transfer uses base + 1) | 5600 |

TLS is enabled when both `-tls-cert` and `-tls-key` are provided.

## Usage Examples

### Basic TLS Setup

```bash
mobius-hotline-server -tls-cert server.crt -tls-key server.key
```

### Custom TLS Port

```bash
mobius-hotline-server -tls-cert server.crt -tls-key server.key -tls-port 5700
```

### Full Example with All Options

```bash
mobius-hotline-server \
  -config /path/to/config \
  -bind 5500 \
  -tls-cert /etc/letsencrypt/live/example.com/fullchain.pem \
  -tls-key /etc/letsencrypt/live/example.com/privkey.pem \
  -tls-port 5600
```

## Verifying TLS is Working

When TLS is enabled, you'll see a log message at startup:

```
TLS enabled port=5600 fileTransferPort=5601
```

You can verify the TLS connection using OpenSSL:

```bash
openssl s_client -connect localhost:5600
```

## Client Configuration

Clients connecting via TLS must:
1. Connect to the TLS port (default 5600) instead of the standard port (5500)
2. Support TLS connections (client-dependent)

Note: Self-signed certificates may require clients to accept or trust the certificate manually.
