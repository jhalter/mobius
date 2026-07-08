# Cloudflare R2 File Store

Mobius can store its file library — the files clients browse, upload, and download — in
[Cloudflare R2](https://developers.cloudflare.com/r2/) instead of on the server's local disk. R2 is
an S3-compatible object store with no egress fees, which makes it a good fit for hosting a file
library that is served to many clients.

The backend is selected with `-file-store r2` and configured entirely through `R2_*` environment
variables, so no secrets are written to config files or visible in the process list.

## How it works

Object stores are not filesystems, so the R2 backend adapts a few things transparently:

- **Directories** are derived from object key prefixes. Empty folders created in a client are
  persisted as zero-byte marker objects so they don't disappear.
- **In-progress uploads** are staged on the server's local disk (object stores have no "append"
  operation). Each `.incomplete` transfer is written to a local staging directory and only promoted
  to a finished R2 object once the upload completes. This preserves resumable uploads without
  re-uploading already-transferred bytes.
- **Aliases (symlinks)** have no object-store equivalent and are not supported on this backend.
  Attempts to create an alias fail gracefully; the rest of the file library is unaffected.

Only the browsable file library uses this backend. Server configuration, accounts, news, the
message board, and the banner continue to be read from the local config directory.

## Prerequisites

1. A Cloudflare account with R2 enabled.
2. An **R2 bucket** to hold the file library.
3. An **R2 API token** (Access Key ID + Secret Access Key) with read/write access to that bucket.
   Create one in the Cloudflare dashboard under **R2 → Manage R2 API Tokens → Create API Token**
   (choose *Object Read & Write*).
4. Your Cloudflare **Account ID** (shown on the R2 overview page), used to derive the S3 endpoint.

## Environment Variables

| Variable | Required | Description |
|---|---|---|
| `R2_BUCKET` | Yes | Name of the R2 bucket that holds the file library. |
| `R2_ACCESS_KEY_ID` | Yes | Access Key ID from your R2 API token. |
| `R2_SECRET_ACCESS_KEY` | Yes | Secret Access Key from your R2 API token. |
| `R2_ACCOUNT_ID` | Yes\* | Cloudflare Account ID. Used to build the endpoint `https://<account-id>.r2.cloudflarestorage.com`. |
| `R2_ENDPOINT` | Yes\* | Explicit S3 endpoint URL. Provide this *instead of* `R2_ACCOUNT_ID` (e.g. when using an R2 custom endpoint). |
| `R2_PREFIX` | No | Key prefix within the bucket to namespace the file library (e.g. `hotline/files`). Defaults to the bucket root. |
| `R2_STAGING_DIR` | No | Local directory for buffering in-progress uploads. Defaults to `<system temp dir>/mobius-uploads`. |

\* Provide **either** `R2_ACCOUNT_ID` **or** `R2_ENDPOINT`. If both are set, `R2_ENDPOINT` wins.

## Command-Line Options

| Flag | Description | Default |
|------|-------------|---------|
| `-file-store` | File library storage backend: `os`, `memory`, or `r2` | `os` |

## Usage

Set the environment variables and start the server with `-file-store r2`:

```bash
export R2_ACCOUNT_ID="your-cloudflare-account-id"
export R2_ACCESS_KEY_ID="your-r2-access-key-id"
export R2_SECRET_ACCESS_KEY="your-r2-secret-access-key"
export R2_BUCKET="my-hotline-files"

mobius-hotline-server -file-store r2
```

### With an optional key prefix

Useful when a single bucket is shared across environments or applications:

```bash
export R2_PREFIX="hotline/files"
mobius-hotline-server -file-store r2
```

### With an explicit endpoint

```bash
export R2_ENDPOINT="https://<account-id>.r2.cloudflarestorage.com"
export R2_ACCESS_KEY_ID="..."
export R2_SECRET_ACCESS_KEY="..."
export R2_BUCKET="my-hotline-files"

mobius-hotline-server -file-store r2
```

### Docker Compose

```yaml
services:
  mobius:
    image: ghcr.io/jhalter/mobius:latest
    command: ["-file-store", "r2"]
    ports:
      - "5500:5500"
      - "5501:5501"
    environment:
      R2_ACCOUNT_ID: "your-cloudflare-account-id"
      R2_ACCESS_KEY_ID: "your-r2-access-key-id"
      R2_SECRET_ACCESS_KEY: "your-r2-secret-access-key"
      R2_BUCKET: "my-hotline-files"
      # R2_PREFIX: "hotline/files"
    volumes:
      # Optional: persist the upload staging dir across restarts so interrupted
      # uploads can resume. Point R2_STAGING_DIR at this path if you mount it.
      - mobius-uploads:/tmp/mobius-uploads
volumes:
  mobius-uploads:
```

## Verifying it's working

On startup you'll see a log line confirming the backend and bucket:

```
Using Cloudflare R2 file store bucket=my-hotline-files
```

Then connect with a Hotline client and:

1. Open the **Files** window — you should see the contents of your bucket (empty on a fresh bucket).
2. **Upload** a file and confirm the object appears in the bucket (Cloudflare dashboard, or
   `aws s3 ls` / `rclone` pointed at the R2 endpoint).
3. **Download** it back and confirm the bytes match.
4. Create a **new folder** and confirm it persists (a zero-byte marker object appears in the bucket).

If startup fails with a configuration error, the message names the missing variable, for example:

```
Error configuring Cloudflare R2 file store err="R2_BUCKET, R2_ACCESS_KEY_ID, and R2_SECRET_ACCESS_KEY must be set"
```

## Notes and limitations

- **Resource forks and metadata** are preserved. Each file's data fork, resource fork (`.rsrc_*`),
  and info fork (`.info_*`) are stored as separate objects alongside each other.
- **In-progress uploads are not visible in listings** until they complete, since they live in the
  local staging directory rather than in R2. Resuming an interrupted upload still works.
- **The staging directory must have enough free space** for concurrent in-progress uploads. If it is
  ephemeral (e.g. a container's default temp dir), a server restart mid-upload discards the partial
  transfer — the same behavior as any interrupted upload; the client simply re-uploads.
- **Aliases (Make Alias)** are unavailable on this backend.
- **Migrating an existing library:** copy your current `Files` directory into the bucket (preserving
  the `.rsrc_*` and `.info_*` sidecar files) using any S3 tool pointed at the R2 endpoint, e.g.
  `rclone copy ./Files r2:my-hotline-files/` or the AWS CLI with `--endpoint-url`.
