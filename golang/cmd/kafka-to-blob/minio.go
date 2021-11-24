package main

import (
	"context"
	"fmt"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"
	"time"
)

func setupMinio(MinioUrl string, MinioAccessKey string, MinioSecretKey string, MinioSecure bool, MinioBucketName string) (mioClient *minio.Client) {
	if !MinioSecure {
		zap.S().Warnf("Minio is not running in secure mode !")
	}
	mioClient, err := minio.New(MinioUrl, &minio.Options{
		Creds:  credentials.NewStaticV4(MinioAccessKey, MinioSecretKey, ""),
		Secure: MinioSecure,
	})
	if err != nil {
		panic(err)
	}

	bucketExists, err := mioClient.BucketExists(context.Background(), MinioBucketName)
	if err != nil {
		panic(err)
	}
	if !bucketExists {
		panic(fmt.Sprintf("Bucket '%s' does not exist", MinioBucketName))
	}
	return
}

func reconnectMinio() {
	healthCheck, err := minioClient.HealthCheck(10 * time.Second)
	if err != nil {
		zap.S().Warnf("Minio went down")
	}
	defer healthCheck()

}
