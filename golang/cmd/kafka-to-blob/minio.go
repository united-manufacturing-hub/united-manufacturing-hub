package main

import (
	"context"
	"fmt"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"
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
