# Untappd Recorder

Untappd Recorder fetches your recent Untappd check-ins and saves the associated photos (including WebP format) to a cloud storage bucket (e.g., AWS S3, Cloudflare R2). Crucially, it also embeds metadata such as comments, ratings, beer names, and brewery information directly into the photo files. It's designed to be run periodically to create a personal backup of your check-in history with rich metadata.

The project consists of two main parts:
- A recorder that fetches the latest check-ins.
- A backfill script to import historical data from a CSV file.

## Getting Started

### Prerequisites

- Go 1.24
- An Untappd account and an API access token.
- A cloud storage bucket (AWS S3, Cloudflare R2, or compatible).

### Configuration

Add the following environment variables to your service:

```
UNTAPPD_ACCESS_TOKEN="your_untappd_api_token"
BUCKET_NAME="your_bucket_name"

# Optional: blur detected faces before photos are uploaded
BLUR_FACES="true"

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
go run cmd/record/main.go
```

This will fetch your recent check-ins and upload any associated photos to your configured storage bucket.

### Backfilling Historical Data

If you are an Untappd Insider, you can download a CSV file of your entire check-in history. The backfill script can use this file to download and save photos for all your historical check-ins.

To run the backfill script, use the following command:

```bash
go run cmd/backfill/main.go -csv untappd_history.csv
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

Detection confidence ranges 0 to 1 (default cutoff 0.5). Tune it with
`-min-quality`: raise it if non-faces get blurred, lower it if faces are missed.
Once tuned, set `BLUR_MIN_QUALITY` on the recorder service so new checkins use
the same cutoff. The `blurfaces` command only needs the bucket credentials,
`UNTAPPD_ACCESS_TOKEN` is required by the recorder alone.

It scans every photo in the bucket, and rewrites, in place, only those where faces are
found (regenerating the WebP copy as well). Replacement is permanent, so run `-dry-run`
first, or enable bucket versioning if you want a way back.

## Deployment

This application can be easily deployed as a serverless or cloud function (e.g., AWS Lambda, Google Cloud Functions) and scheduled to run on a daily basis to keep your check-in archive up to date.
