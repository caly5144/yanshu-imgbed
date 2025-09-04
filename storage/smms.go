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
	BaseURL string // SM.MS API 基础 URL，例如 "https://sm.ms/api/v2/"
	Token   string // SM.MS API Token
}

// NewSmmsUploader 创建一个新的 SM.MS 存储实例
func NewSmmsUploader(baseURL, token string) *SmmsUploader {
	return &SmmsUploader{BaseURL: baseURL, Token: token}
}

func (s *SmmsUploader) Upload(file *multipart.FileHeader, uniqueFilename string) (string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("smfile", uniqueFilename)
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}

	fileReader, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer fileReader.Close()

	_, err = io.Copy(part, fileReader)
	if err != nil {
		return "", fmt.Errorf("failed to copy file data: %w", err)
	}
	writer.Close() // 必须关闭 writer 才能完成 body 的写入

	req, err := http.NewRequest("POST", s.BaseURL+"upload", body)
	if err != nil {
		return "", fmt.Errorf("failed to create upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType()) // Content-Type 必须是 writer 提供的
	req.Header.Set("Authorization", s.Token)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send upload request: %w", err)
	}
	defer resp.Body.Close()

	// 检查非 2xx 状态码
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body) // 读取错误响应体
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

	// --- NEW: 存储删除图片的 hash 值 ---
	deleteHash, ok := data["hash"].(string) // SM.MS 的删除 hash
	if ok && deleteHash != "" {
		// 返回一个包含 URL 和删除哈希的特殊格式，以便 ImageService 保存
		// 格式： "url_value@@@delete_hash_value"
		return fmt.Sprintf("%s@@@%s", imageUrl, deleteHash), nil
	}
	// --- END NEW ---

	return imageUrl, nil
}

func (s *SmmsUploader) UploadFromFile(localPath string, uniqueFilename string) (string, error) {
	// 1. Open the local file
	file, err := os.Open(localPath)
	if err != nil {
		return "", fmt.Errorf("failed to open local file %s: %w", localPath, err)
	}
	defer file.Close()

	// 2. Create a multipart writer to build the request body
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 3. Create a form file part
	// Use the original filename from the database if available, otherwise use the unique filename
	part, err := writer.CreateFormFile("smfile", filepath.Base(localPath)) // Use the actual filename for the form
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}

	// 4. Copy the file content into the part
	_, err = io.Copy(part, file)
	if err != nil {
		return "", fmt.Errorf("failed to copy file data: %w", err)
	}
	writer.Close() // Finalize the body

	// 5. Create and send the HTTP request (this part is the same as the original Upload method)
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
		message := result["message"].(string)
		return "", fmt.Errorf("SM.MS upload failed: %s", message)
	}

	data, _ := result["data"].(map[string]interface{})
	imageUrl, _ := data["url"].(string)
	deleteHash, _ := data["hash"].(string)

	if deleteHash != "" {
		return fmt.Sprintf("%s@@@%s", imageUrl, deleteHash), nil
	}

	return imageUrl, nil
}

// Type 返回存储驱动的类型名
func (s *SmmsUploader) Type() string {
	return "sm.ms"
}

// Delete 从 SM.MS 删除图片
// 注意：需要知道 SM.MS 返回的 hash 值
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
	// 对于 Content-Type: multipart/form-data 的 POST 请求，即使没有数据，也需要构造一个 multipart body
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.Close() // 立即关闭 writer，表示没有文件或字段

	req, err := http.NewRequest("POST", s.BaseURL+"profile", body) // NEW: 使用构造的 body
	if err != nil {
		return fmt.Errorf("failed to create profile request: %w", err)
	}
	req.Header.Set("Authorization", s.Token)
	req.Header.Set("Content-Type", writer.FormDataContentType()) // NEW: 使用 writer 提供的 Content-Type

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send profile request: %w", err)
	}
	defer resp.Body.Close()

	// NEW: 检查非 2xx 状态码
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body) // 读取错误响应体
		return fmt.Errorf("SM.MS token verification failed with status %d: %s", resp.StatusCode, string(respBody))
	}
	// --- END NEW ---

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// 如果是 EOF 错误，可能是响应为空或不完整
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
