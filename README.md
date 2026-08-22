# Untappd Recorder

Untappd Recorder fetches your recent Untappd check-ins and saves the associated photos (including WebP format) to a cloud storage bucket (e.g., AWS S3, Cloudflare R2). Crucially, it also embeds metadata such as comments, ratings, beer names, and brewery information directly into the photo files. It's designed to be run periodically to create a personal backup of your check-in history with rich metadata.

The project consists of four commands:
- `record`: fetches the latest check-ins and uploads their photos (run on a schedule).
- `backfill`: imports historical check-ins from an Untappd Insider CSV export.
- `blurfaces`: one-off pass that anonymises faces in photos already stored in the bucket.
- `manifest`: rebuilds the bucket's `index.json` photo manifest from scratch.

Optionally, faces can be detected (YOLOv8-face on ONNX Runtime) and blurred before
photos are uploaded, and a metadata manifest is maintained for gallery frontends.

## Getting Started

### Prerequisites

- Go 1.25
- libvips (`apt install libvips-dev`, `dnf install vips-devel` or `brew install vips`),
  needed by every command except `manifest`.
- An Untappd account and an API access token (for `record` only).
- A cloud storage bucket (AWS S3, Cloudflare R2, or compatible).
- For face blurring, the ONNX Runtime shared library (see Blurring Faces below).

Generate the libvips bindings once before building or running locally:

```bash
make vipsgen
```

### Configuration

Add the following environment variables to your service:

```
UNTAPPD_ACCESS_TOKEN="your_untappd_api_token" # Only required by the record command
BUCKET_NAME="your_bucket_name"

# Optional
BLUR_FACES="true"        # Blur detected faces before photos are uploaded (default false)
BLUR_MIN_QUALITY="0.25"  # Detection confidence cutoff, 0 to 1
NUM_WORKERS="4"          # Concurrent photo workers

# For Cloudflare R2:
R2_ACCOUNT_ID="your_r2_account_id"
R2_ACCESS_KEY_ID="your_r2_access_key_id"
R2_SECRET_ACCESS_KEY="your_r2_secret_access_key"

# For AWS S3:
AWS_REGION="your_aws_region" # e.g., us-east-1
AWS_ACCESS_KEY_ID="your_aws_access_key_id" # Required if not using IAM roles or shared credentials file
AWS_SECRET_ACCESS_KEY="your_aws_secret_access_key" # Required if not using IAM roles or shared credentials file
```

## Usage

### Recording Recent Check-ins

To record your latest check-ins, run the following command:

```bash
go run ./cmd/record
```

This will fetch your recent check-ins and upload any associated photos to your configured storage bucket.

### Backfilling Historical Data

If you are an Untappd Insider, you can download a CSV file of your entire check-in history. The backfill script can use this file to download and save photos for all your historical check-ins.

To run the backfill script, use the following command:

```bash
go run ./cmd/backfill -csv untappd_history.csv
```

### Blurring Faces

With `BLUR_FACES=true`, faces are detected with a YOLOv8-face model (embedded in the
binary, running on ONNX Runtime) and gaussian-blurred before photos are uploaded. The
Docker image ships the ONNX Runtime library; nothing extra is needed on Railway.

To anonymise photos already stored in the bucket, run the one-off `blurfaces` command
locally. It needs libvips and the ONNX Runtime shared library installed:

```bash
# Linux
ORT_BASE=https://github.com/microsoft/onnxruntime/releases/download
curl -sL "$ORT_BASE/v1.29.0/onnxruntime-linux-x64-1.29.0.tgz" | sudo tar xz -C /opt
export ONNXRUNTIME_LIB=/opt/onnxruntime-linux-x64-1.29.0/lib/libonnxruntime.so

# macOS (found automatically in /opt/homebrew/lib)
brew install onnxruntime
```

```bash
go run ./cmd/blurfaces -dry-run   # report what would be blurred, with confidence scores
go run ./cmd/blurfaces            # replace photos in place
```

Detection confidence ranges 0 to 1 (default cutoff 0.25). Tune it with
`-min-quality`: raise it if non-faces get blurred, lower it if faces are missed.
Once tuned, set `BLUR_MIN_QUALITY` on the recorder service so new checkins use
the same cutoff. The `blurfaces` command only needs the bucket credentials,
`UNTAPPD_ACCESS_TOKEN` is required by the recorder alone.

To redo specific photos where faces were missed, pass their keys as arguments,
typically with a more permissive cutoff:

```bash
go run ./cmd/blurfaces -min-quality 0.15 2019/08/25/796751183.jpg 2020/01/12/812345678.jpg
```

By default it scans every photo in the bucket, and rewrites, in place, only those where faces are
found (regenerating the WebP copy as well). Replacement is permanent, so run `-dry-run`
first, or enable bucket versioning if you want a way back.

### Photo Manifest

The recorder and backfill maintain a manifest at the bucket root, `index.json`: a JSON
array with one record per WEBP photo (object key plus all decoded metadata fields),
sorted newest first. The gallery frontend reads it to filter check-ins without
per-object requests. It updates automatically at the end of each run.

The manifest is derived data, the bucket stays the single source of truth. To create it
for an existing bucket, or repair it if it is ever wrong or lost, run the rebuild
command locally (it only needs the bucket credentials):

```bash
go run ./cmd/manifest
```

It walks every WEBP object, reads its metadata, and writes a fresh `index.json`.

## Deployment

The Dockerfile builds the `record` command into a Debian-based image with libvips and
the ONNX Runtime library included, so face blurring works with no extra setup. Deploy
it to any container platform with a cron scheduler (e.g. Railway) and run it daily to
keep the check-in archive up to date.

Notes:
- The recorder only advances its progress marker when every check-in saved, so failed
  runs are retried automatically on the next schedule.
- With `BLUR_FACES=true`, expect roughly 400-500MB of peak memory during a run.
- The `backfill`, `blurfaces` and `manifest` commands are intended to be run locally,
  not deployed.
