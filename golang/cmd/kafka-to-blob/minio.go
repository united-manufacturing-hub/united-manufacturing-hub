package main

import (
	"context"
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
		err := mioClient.MakeBucket(context.Background(), MinioBucketName, minio.MakeBucketOptions{
			ObjectLocking: false,
		})
		if err != nil {
			zap.S().Errorf("Bucket '%s' does not exist and failed to create !", MinioBucketName)
			panic(err)
		}
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
