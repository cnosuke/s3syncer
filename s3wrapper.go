package main

import (
	"os"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
)

type S3Wrapper struct {
	objects    map[string]*string
	oFlag      sync.Mutex
	s3Svc      *s3.S3
	bucketName string
	keyPrefix  string
}

func NewS3Wrapper(s3Svc *s3.S3, bucketName string, keyPrefix string) *S3Wrapper {
	return &S3Wrapper{
		objects:    make(map[string]*string),
		s3Svc:      s3Svc,
		bucketName: bucketName,
		keyPrefix:  keyPrefix,
	}
}

func (c *S3Wrapper) FetchAllKeys() error {
	inputParam := s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucketName),
		Prefix: &c.keyPrefix,
	}

	err := c.s3Svc.ListObjectsV2Pages(&inputParam,
		func(page *s3.ListObjectsV2Output, lastPage bool) bool {
			go c.addContentsToCache(page.Contents)

			return !lastPage
		})

	if err != nil {
		logger.Errorw(err.Error())
		return err
	}

	return nil
}

func (c *S3Wrapper) addContentsToCache(contents []*s3.Object) {
	for _, content := range contents {
		c.oFlag.Lock()
		c.objects[*content.Key] = content.ETag
		c.oFlag.Unlock()
	}
}

func (c *S3Wrapper) PutObject(filePath string, key string) (*s3.PutObjectOutput, error) {
	file, err := os.Open(filePath)
	if err != nil {
		logger.Errorw(err.Error())
	}
	defer file.Close()

	acl := "private"
	input := s3.PutObjectInput{
		ACL:    &acl,
		Bucket: &c.bucketName,
		Key:    &key,
		Body:   file,
	}

	res, err := c.s3Svc.PutObject(&input)

	if err != nil {
		logger.Errorw(err.Error())
		return nil, err
	}

	return res, nil
}

func (c *S3Wrapper) Fetch(key string) *string {
	c.oFlag.Lock()
	res := c.objects[key]
	c.oFlag.Unlock()

	return res
}

func (c *S3Wrapper) HasKey(key string) bool {

	return c.Fetch(key) != nil
}

func (c *S3Wrapper) CacheSize() int {
	return len(c.objects)
}
