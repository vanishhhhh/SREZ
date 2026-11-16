package config

import (
	"os"
	"srez/internal/objectstorage"
)

// Media is optional outside Compose; a partial configuration fails closed.
func LoadMedia() (objectstorage.Config, bool) {
	c := objectstorage.Config{Endpoint: os.Getenv("SREZ_S3_ENDPOINT"), PublicEndpoint: os.Getenv("SREZ_S3_PUBLIC_ENDPOINT"), Region: os.Getenv("SREZ_S3_REGION"), Bucket: os.Getenv("SREZ_S3_BUCKET"), AccessKey: os.Getenv("SREZ_S3_ACCESS_KEY"), SecretKey: os.Getenv("SREZ_S3_SECRET_KEY")}
	return c, c.Endpoint != "" || c.PublicEndpoint != "" || c.AccessKey != "" || c.SecretKey != ""
}
