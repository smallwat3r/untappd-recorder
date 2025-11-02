# Untappd Recorder

This project is a tool to record your Untappd check-ins, including photos, to a cloud storage provider.

## Description

Untappd Recorder fetches your recent Untappd check-ins and saves the associated photos to a cloud storage bucket (e.g., AWS S3, Cloudflare R2). Crucially, it also embeds metadata such as comments, ratings, beer names, and brewery information directly into the photo files. It's designed to be run periodically to create a personal backup of your check-in history with rich metadata.

The project consists of two main parts:
- A recorder that fetches the latest check-ins.
- A backfill script to import historical data from a CSV file.

## Getting Started

### Prerequisites

- Go installed on your machine.
- An Untappd account and an API access token.
- A cloud storage bucket (AWS S3, Cloudflare R2, or compatible).

### Configuration

Create a `.env` file in the root of the project and add the following environment variables:

```
UNTAPPD_ACCESS_TOKEN="your_untappd_api_token"
BUCKET_NAME="your_bucket_name"

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

## Deployment

This application can be easily deployed as a serverless or cloud function (e.g., AWS Lambda, Google Cloud Functions) and scheduled to run on a daily basis to keep your check-in archive up to date.
