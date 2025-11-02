package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Run("should load config when all env vars are set", func(t *testing.T) {
		os.Setenv("UNTAPPD_ACCESS_TOKEN", "test_token")
		os.Setenv("R2_ACCOUNT_ID", "test_account_id")
		os.Setenv("R2_ACCESS_KEY_ID", "test_key_id")
		os.Setenv("R2_SECRET_ACCESS_KEY", "test_key_secret")
		os.Setenv("BUCKET_NAME", "test_bucket_name")

		cfg, err := Load()
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if cfg.BucketName != "test_bucket_name" {
			t.Errorf("expected BucketName to be 'test_bucket_name', got %s", cfg.BucketName)
		}

		if cfg.UntappdAccessToken != "test_token" {
			t.Errorf(
				"expected UntappdAccessToken to be 'test_token', got %s",
				cfg.UntappdAccessToken,
			)
		}
	})

	t.Run("should return error when an env var is missing", func(t *testing.T) {
		os.Unsetenv("UNTAPPD_ACCESS_TOKEN")

		_, err := Load()
		if err == nil {
			t.Errorf("expected an error, got nil")
		}
	})
}
