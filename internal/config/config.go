package config

import (
	"fmt"
	"os"
)

type Config struct {
	UntappdClientID     string
	UntappdClientSecret string
	UntappdAccessToken  string
	R2AccountID         string
	R2AccessKeyID       string
	R2AccessKeySecret   string
	R2BucketName        string
}

func Load() (*Config, error) {
	cfg := &Config{
		UntappdClientID:     os.Getenv("UNTAPPD_CLIENT_ID"),
		UntappdClientSecret: os.Getenv("UNTAPPD_CLIENT_SECRET"),
		UntappdAccessToken:  os.Getenv("UNTAPPD_ACCESS_TOKEN"),
		R2AccountID:         os.Getenv("R2_ACCOUNT_ID"),
		R2AccessKeyID:       os.Getenv("R2_ACCESS_KEY_ID"),
		R2AccessKeySecret:   os.Getenv("R2_ACCESS_KEY_SECRET"),
		R2BucketName:        os.Getenv("R2_BUCKET_NAME"),
	}

	if cfg.UntappdClientID == "" || cfg.UntappdClientSecret == "" || cfg.UntappdAccessToken == "" || cfg.R2AccountID == "" || cfg.R2AccessKeyID == "" || cfg.R2AccessKeySecret == "" || cfg.R2BucketName == "" {
		return nil, fmt.Errorf("missing one or more required environment variables")
	}

	return cfg, nil
}
