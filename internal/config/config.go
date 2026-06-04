package config

import (
	"fmt"

	"github.com/caarlos0/env/v6"
)

type Config struct {
	UntappdAccessToken   string `env:"UNTAPPD_ACCESS_TOKEN,required"`
	R2AccountID          string `env:"R2_ACCOUNT_ID"`
	R2AccessKeyID        string `env:"R2_ACCESS_KEY_ID"`
	R2AccessKeySecret    string `env:"R2_SECRET_ACCESS_KEY"`
	AWSRegion            string `env:"AWS_REGION"`
	BucketName           string `env:"BUCKET_NAME,required"`
	NumWorkers           int    `env:"NUM_WORKERS,required"          envDefault:"4"`
	PlaceholderPhotoPath string `env:"PLACEHOLDER_PHOTO_PATH"        envDefault:"img/missing.jpg"`
}

func Load() (*Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, err
	}
	if err := cfg.validateProvider(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// validateProvider ensures exactly one storage provider is fully configured.
// R2 credentials form a single group: either all of them are set, or none.
func (c *Config) validateProvider() error {
	r2 := []string{c.R2AccountID, c.R2AccessKeyID, c.R2AccessKeySecret}

	var set int
	for _, v := range r2 {
		if v != "" {
			set++
		}
	}

	switch {
	case set > 0 && set < len(r2):
		return fmt.Errorf(
			"incomplete R2 configuration: R2_ACCOUNT_ID, R2_ACCESS_KEY_ID and " +
				"R2_SECRET_ACCESS_KEY must all be set together",
		)
	case set == len(r2):
		return nil
	case c.AWSRegion != "":
		return nil
	default:
		return fmt.Errorf(
			"no storage provider configured: set the R2 credentials or AWS_REGION",
		)
	}
}
