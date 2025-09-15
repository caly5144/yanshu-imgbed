package storage

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"yanshu-imgbed/util"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// OssUploader 实现了 Uploader 接口，用于阿里云OSS
type OssUploader struct {
	Client     *oss.Client
	Bucket     *oss.Bucket
	PublicURL  string // 对外访问的基础 URL，用于自定义域名
	UploadPath string // OSS上的存储路径前缀
}

// NewOssUploader 创建一个新的OSS存储实例
func NewOssUploader(config map[string]string) (*OssUploader, error) {
	endpoint := config["endpoint"]
	bucketName := config["bucket"]
	accessKeyId := config["accessKeyId"]
	accessKeySecret := config["accessKeySecret"]

	if endpoint == "" || bucketName == "" || accessKeyId == "" || accessKeySecret == "" {
		return nil, fmt.Errorf("OSS config is missing required fields (endpoint, bucket, accessKeyId, accessKeySecret)")
	}

	client, err := oss.New(endpoint, accessKeyId, accessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("failed to create OSS client: %w", err)
	}

	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to get OSS bucket '%s': %w", bucketName, err)
	}

	uploader := &OssUploader{
		Client:     client,
		Bucket:     bucket,
		PublicURL:  config["publicUrl"],
		UploadPath: config["uploadPath"],
	}

	return uploader, nil
}

func (o *OssUploader) Upload(fileHeader *multipart.FileHeader, uniqueFilename string, src io.Reader) (string, error) {
	objectKey := filepath.ToSlash(filepath.Join(o.UploadPath, uniqueFilename))

	err := o.Bucket.PutObject(objectKey, src)
	if err != nil {
		return "", fmt.Errorf("failed to upload object to OSS: %w", err)
	}

	var publicURL string
	if o.PublicURL != "" {
		publicURL = fmt.Sprintf("%s/%s", o.PublicURL, objectKey)
	} else {
		publicURL = fmt.Sprintf("https://%s.%s/%s", o.Bucket.BucketName, util.ExtractEndpointHost(o.Client.Config.Endpoint), objectKey)
	}

	// --- 已修改：返回包含URL和Object Key的特殊格式 ---
	// 格式为 "public_url@@@object_key"
	return fmt.Sprintf("%s@@@%s", publicURL, objectKey), nil
}

func (o *OssUploader) Type() string {
	return "oss"
}

func (o *OssUploader) UploadFromFile(localPath string, uniqueFilename string) (string, error) {
	src, err := os.Open(localPath)
	if err != nil {
		return "", err
	}
	defer src.Close()

	return o.Upload(nil, uniqueFilename, src)
}

// Delete 从OSS删除文件
func (o *OssUploader) Delete(objectKey string) error {
	if objectKey == "" {
		return fmt.Errorf("OSS delete identifier (object key) is empty")
	}
	return o.Bucket.DeleteObject(objectKey)
}
