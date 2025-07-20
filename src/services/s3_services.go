package services

import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/google/uuid"
)

type S3Service struct {
	client     *s3.S3
	bucketName string
}

func NewS3Service() (*S3Service, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION")),
	})
	if err != nil {
		return nil, err
	}

	return &S3Service{
		client:     s3.New(sess),
		bucketName: os.Getenv("CSV_BUCKET_NAME"),
	}, nil
}

func (s *S3Service) GeneratePreSignedUploadURL(uploadID uuid.UUID, filename string) (string, string, error) {
	// Strip any directory components and require a .csv suffix: the bucket
	// notification only fires for uploads/*.csv, anything else would never be processed.
	filename = path.Base(strings.ReplaceAll(filename, "\\", "/"))
	if !strings.HasSuffix(strings.ToLower(filename), ".csv") {
		return "", "", fmt.Errorf("filename must end with .csv")
	}

	// Generate S3 key with UUID for uniqueness
	s3Key := fmt.Sprintf("uploads/%s/%s", uploadID.String(), filename)

	req, _ := s.client.PutObjectRequest(&s3.PutObjectInput{
		Bucket:      aws.String(s.bucketName),
		Key:         aws.String(s3Key),
		ContentType: aws.String("text/csv"),
	})

	// Generate pre-signed URL valid for 15 minutes
	url, err := req.Presign(15 * time.Minute)
	if err != nil {
		return "", "", err
	}

	return url, s3Key, nil
}

func (s *S3Service) GetObject(s3Key string) (*s3.GetObjectOutput, error) {
	return s.client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(s3Key),
	})
}

func (s *S3Service) DeleteObject(s3Key string) error {
	_, err := s.client.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(s3Key),
	})
	return err
}
