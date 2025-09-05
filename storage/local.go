package storage

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
)

// LocalUploader 实现了 Uploader 接口
type LocalUploader struct {
	StoragePath string // 存储路径，例如 "uploads"
	PublicURL   string // 对外访问的基础 URL，例如 "http://localhost:8080"
}

// NewLocalUploader 创建一个新的本地存储实例
func NewLocalUploader(storagePath, publicURL string) *LocalUploader {
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		os.MkdirAll(storagePath, os.ModePerm)
	}
	return &LocalUploader{StoragePath: storagePath, PublicURL: publicURL}
}

// --- 已修改：匹配新的接口，直接使用 src ---
func (l *LocalUploader) Upload(fileHeader *multipart.FileHeader, uniqueFilename string, src io.Reader) (string, error) {
	// 不再需要从 fileHeader.Open()，直接使用传入的 src
	return l.saveFile(src, uniqueFilename)
}

func (l *LocalUploader) UploadFromFile(localPath string, uniqueFilename string) (string, error) {
	src, err := os.Open(localPath)
	if err != nil {
		return "", err
	}
	defer src.Close()

	return l.saveFile(src, uniqueFilename)
}

func (l *LocalUploader) saveFile(src io.Reader, uniqueFilename string) (string, error) {
	dst := filepath.Join(l.StoragePath, uniqueFilename)
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer out.Close()

	if _, err = io.Copy(out, src); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s/uploads/%s", l.PublicURL, uniqueFilename), nil
}

func (l *LocalUploader) Type() string {
	return "local"
}

func (l *LocalUploader) Delete(deleteIdentifier string) error {
	if deleteIdentifier == "" {
		return fmt.Errorf("local delete identifier is empty")
	}
	fullPath := filepath.Join(l.StoragePath, deleteIdentifier)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(fullPath)
}
