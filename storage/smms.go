package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// SmmsUploader 实现了 Uploader 接口
type SmmsUploader struct {
	BaseURL string
	Token   string
}

// NewSmmsUploader 创建一个新的 SM.MS 存储实例
func NewSmmsUploader(baseURL, token string) *SmmsUploader {
	return &SmmsUploader{BaseURL: baseURL, Token: token}
}

// --- 已修改：匹配新的接口，直接使用 fileReader ---
func (s *SmmsUploader) Upload(fileHeader *multipart.FileHeader, uniqueFilename string, fileReader io.Reader) (string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("smfile", uniqueFilename)
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}

	// 不再需要从 fileHeader.Open()，直接使用传入的 fileReader
	_, err = io.Copy(part, fileReader)
	if err != nil {
		return "", fmt.Errorf("failed to copy file data: %w", err)
	}
	writer.Close()

	req, err := http.NewRequest("POST", s.BaseURL+"upload", body)
	if err != nil {
		return "", fmt.Errorf("failed to create upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", s.Token)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send upload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("SM.MS upload failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode upload response: %w", err)
	}

	if success, ok := result["success"].(bool); !ok || !success {
		message := "unknown SM.MS upload error"
		if msg, ok := result["message"].(string); ok {
			message = msg
		}
		return "", fmt.Errorf("SM.MS upload failed: %s", message)
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return "", errors.New("SM.MS upload response 'data' field missing or invalid")
	}

	imageUrl, ok := data["url"].(string)
	if !ok {
		return "", errors.New("SM.MS upload response 'data.url' field missing or invalid")
	}

	deleteHash, ok := data["hash"].(string)
	if ok && deleteHash != "" {
		return fmt.Sprintf("%s@@@%s", imageUrl, deleteHash), nil
	}

	return imageUrl, nil
}

func (s *SmmsUploader) UploadFromFile(localPath string, uniqueFilename string) (string, error) {
	file, err := os.Open(localPath)
	if err != nil {
		return "", fmt.Errorf("failed to open local file %s: %w", localPath, err)
	}
	defer file.Close()
	fileInfo, _ := file.Stat()

	// 构造一个临时的 FileHeader，因为 Upload 方法需要它
	tempHeader := &multipart.FileHeader{
		Filename: filepath.Base(localPath),
		Size:     fileInfo.Size(),
	}

	return s.Upload(tempHeader, uniqueFilename, file)
}

func (s *SmmsUploader) Type() string {
	return "sm.ms"
}

func (s *SmmsUploader) Delete(deleteHash string) error {
	if deleteHash == "" {
		return errors.New("SM.MS delete hash is empty")
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("%sdelete/%s", s.BaseURL, deleteHash), nil)
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}
	req.Header.Set("Authorization", s.Token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send delete request: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode delete response: %w", err)
	}

	if success, ok := result["success"].(bool); !ok || !success {
		message := "unknown SM.MS delete error"
		if msg, ok := result["message"].(string); ok {
			message = msg
		}
		return fmt.Errorf("SM.MS delete failed: %s", message)
	}
	return nil
}

func (s *SmmsUploader) CheckToken() error {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.Close()

	req, err := http.NewRequest("POST", s.BaseURL+"profile", body)
	if err != nil {
		return fmt.Errorf("failed to create profile request: %w", err)
	}
	req.Header.Set("Authorization", s.Token)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send profile request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("SM.MS token verification failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		if err == io.EOF {
			return errors.New("SM.MS token verification failed: empty or incomplete response from server")
		}
		return fmt.Errorf("failed to decode profile response: %w", err)
	}

	if success, ok := result["success"].(bool); !ok || !success {
		message := "unknown SM.MS token verification error"
		if msg, ok := result["message"].(string); ok {
			message = msg
		}
		return fmt.Errorf("SM.MS token verification failed: %s", message)
	}
	return nil
}
