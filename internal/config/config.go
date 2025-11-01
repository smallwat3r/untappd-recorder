package config

import (
	"github.com/caarlos0/env/v6"
)

type Config struct {
	UntappdAccessToken string `env:"UNTAPPD_ACCESS_TOKEN,required"`
	R2AccountID        string `env:"R2_ACCOUNT_ID,required"`
	R2AccessKeyID      string `env:"R2_ACCESS_KEY_ID,required"`
	R2AccessKeySecret  string `env:"R2_ACCESS_KEY_SECRET,required"`
	R2BucketName       string `env:"R2_BUCKET_NAME,required"`
}

func Load() (*Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
