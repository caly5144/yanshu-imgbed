package storage

import "mime/multipart"

type Uploader interface {
	// Upload 上传文件到指定存储，返回可访问的 URL。
	// 对于需要额外删除标识的存储（如SM.MS），URL可能包含特殊分隔符，如 "url@@@deleteIdentifier"。
	Upload(file *multipart.FileHeader, uniqueFilename string) (string, error)
	// Type 返回存储驱动的类型名
	UploadFromFile(localPath string, uniqueFilename string) (string, error)
	Type() string
	// Delete 从存储中删除文件，需要一个 DeleteIdentifier
	// 对于本地存储，DeleteIdentifier 就是文件路径或文件名
	// 对于SM.MS，DeleteIdentifier 是其返回的 hash 值
	Delete(deleteIdentifier string) error // NEW
}
