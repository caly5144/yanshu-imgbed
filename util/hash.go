package util

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"mime/multipart"
)

// CalculateFileMD5 计算 multipart.FileHeader 的 MD5 哈希值
func CalculateFileMD5(file *multipart.FileHeader) (string, error) {
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, src); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}
