package config

import "os"

type R2 struct {
	Bucket          string
	BucketURL       string
	AccountID       string
	AccessKeyID     string
	AccessKeySecret string
}

func (r *R2) Parse() error {
	r.BucketURL = os.Getenv("R2_BUCKET_URL")
	r.Bucket = os.Getenv("R2_BUCKET")
	r.AccountID = os.Getenv("R2_ACCOUNT_ID")
	r.AccessKeyID = os.Getenv("R2_ACCESS_KEY_ID")
	r.AccessKeySecret = os.Getenv("R2_ACCESS_KEY_SECRET")
	return nil
}
