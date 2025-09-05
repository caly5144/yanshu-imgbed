package storage

import (
	"io" // 导入 io 包
	"mime/multipart"
)

type Uploader interface {
	// --- 已修改：增加 io.Reader 参数 ---
	Upload(fileHeader *multipart.FileHeader, uniqueFilename string, fileReader io.Reader) (string, error)
	Type() string
	UploadFromFile(localPath string, uniqueFilename string) (string, error)
	Delete(deleteIdentifier string) error
}
