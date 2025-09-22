package service

import (
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"yanshu-imgbed/database"
	"yanshu-imgbed/manager"
	"yanshu-imgbed/storage"
	"yanshu-imgbed/util"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ListImagesResponse is the new structure for paginated image lists.
type ListImagesResponse struct {
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"pageSize"`
	Images   []database.Image `json:"images"`
}

var (
	tasks            = make(map[string]*Task)
	taskMu           sync.Mutex
	randomImageUUIDs []string
	cacheMutex       sync.RWMutex
)

type Task struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Status    string    `json:"status"` // "running", "completed", "failed"
	Progress  int       `json:"progress"`
	Total     int       `json:"total"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// ToggleImageRandomStatus toggles the AllowRandom status for a single image.
func ToggleImageRandomStatus(imageUUID string) (*database.Image, error) {
	var image database.Image
	if err := database.DB.Where("uuid = ?", imageUUID).First(&image).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("image not found")
		}
		return nil, err
	}

	image.AllowRandom = !image.AllowRandom
	if err := database.DB.Save(&image).Error; err != nil {
		return nil, err
	}

	// Trigger cache update in the background
	go UpdateRandomImageCache()

	return &image, nil
}

func getImageDimensions(file *multipart.FileHeader) (int, int, error) {
	src, err := file.Open()
	if err != nil {
		return 0, 0, err
	}
	defer src.Close()

	if _, err := src.Seek(0, 0); err != nil {
		return 0, 0, fmt.Errorf("failed to seek file start: %w", err)
	}

	config, _, err := image.DecodeConfig(src)
	if err != nil {
		log.Printf("Could not decode image config for %s: %v. This might be an SVG or other format.", file.Filename, err)
		return 0, 0, nil
	}
	return config.Width, config.Height, nil
}

// UploadImage handles the entire image upload flow, including deduplication.
func UploadImage(file *multipart.FileHeader, userID uint, targetBackendIDs []uint, storageManager *manager.StorageManager) (*database.Image, error) {
	fileMD5, err := util.CalculateFileMD5(file)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate file MD5: %w", err)
	}

	var existingImageForUser database.Image
	err = database.DB.Preload("StorageLocations.Backend").
		Where("md5 = ? AND user_id = ?", fileMD5, userID).
		First(&existingImageForUser).Error

	if err == nil {
		log.Printf("Duplicate image for user %d (MD5: %s). Backfilling.", userID, fileMD5)
		return handleDuplicateImage(&existingImageForUser, file, targetBackendIDs, storageManager)
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("database error during user duplicate check: %w", err)
	}

	var existingImageForOtherUser database.Image
	err = database.DB.Preload("StorageLocations.Backend").
		Where("md5 = ?", fileMD5).
		First(&existingImageForOtherUser).Error

	if err == nil {
		log.Printf("Image exists from another user (MD5: %s). Creating new metadata reference for user %d.", fileMD5, userID)
		return handleSharedImage(file, userID, fileMD5, &existingImageForOtherUser)
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("database error during global duplicate check: %w", err)
	}

	log.Printf("New image for the system (MD5: %s). Starting fresh upload for user %d.", fileMD5, userID)
	return handleNewImage(file, userID, fileMD5, targetBackendIDs, storageManager)
}

// handleNewImage uploads a completely new file and creates all records.
func handleNewImage(file *multipart.FileHeader, userID uint, fileMD5 string, targetBackendIDs []uint, storageManager *manager.StorageManager) (*database.Image, error) {
	width, height, err := getImageDimensions(file)
	if err != nil {
		log.Printf("Could not get image dimensions for %s: %v. Proceeding with 0x0.", file.Filename, err)
	}

	var activeBackends []database.Backend
	query := database.DB.Where("allow_upload = ?", true)
	if len(targetBackendIDs) > 0 {
		query = query.Where("id IN (?)", targetBackendIDs)
	}
	if err := query.Find(&activeBackends).Error; err != nil {
		return nil, fmt.Errorf("failed to load active backends: %w", err)
	}
	if len(activeBackends) == 0 {
		return nil, errors.New("no active storage backends configured or selected")
	}

	image := &database.Image{
		UUID:             uuid.New().String(),
		MD5:              fileMD5,
		OriginalFilename: file.Filename,
		FileSize:         file.Size,
		ContentType:      file.Header.Get("Content-Type"),
		Width:            width,
		Height:           height,
		UserID:           userID,
	}
	if err := database.DB.Create(&image).Error; err != nil {
		return nil, fmt.Errorf("failed to create image record: %w", err)
	}

	uniqueFilename := fmt.Sprintf("%s%s", image.UUID, filepath.Ext(file.Filename))
	distributeToBackends(file, uniqueFilename, image.ID, activeBackends, storageManager)

	database.DB.Preload("StorageLocations.Backend").First(&image, image.ID)
	if len(image.StorageLocations) == 0 {
		database.DB.Delete(&image)
		return nil, errors.New("upload failed on all active backends")
	}

	return image, nil
}

// handleDuplicateImage is for when the SAME user uploads the same file again.
func handleDuplicateImage(existingImage *database.Image, file *multipart.FileHeader, targetBackendIDs []uint, storageManager *manager.StorageManager) (*database.Image, error) {
	var backendsToBackfill []database.Backend
	var allPossibleBackends []database.Backend

	query := database.DB.Where("allow_upload = ?", true)
	if len(targetBackendIDs) > 0 {
		query = query.Where("id IN (?)", targetBackendIDs)
	}
	query.Find(&allPossibleBackends)

	existingBackendIDs := make(map[uint]bool)
	for _, loc := range existingImage.StorageLocations {
		existingBackendIDs[loc.BackendID] = true
	}

	for _, backend := range allPossibleBackends {
		if !existingBackendIDs[backend.ID] {
			backendsToBackfill = append(backendsToBackfill, backend)
		}
	}

	if len(backendsToBackfill) == 0 {
		return existingImage, nil
	}

	uniqueFilename := fmt.Sprintf("%s%s", existingImage.UUID, filepath.Ext(file.Filename))
	distributeToBackends(file, uniqueFilename, existingImage.ID, backendsToBackfill, storageManager)

	database.DB.Preload("StorageLocations.Backend").First(&existingImage, existingImage.ID)
	return existingImage, nil
}

// handleSharedImage creates a new Image metadata record for a user, linking to existing physical files.
func handleSharedImage(file *multipart.FileHeader, userID uint, fileMD5 string, existingImage *database.Image) (*database.Image, error) {
	width, height, err := getImageDimensions(file)
	if err != nil {
		log.Printf("Could not get image dimensions for shared image %s: %v. Using existing.", file.Filename, err)
		width = existingImage.Width
		height = existingImage.Height
	}

	// Create a new image record for the new user. This will now succeed due to the composite unique index.
	image := &database.Image{
		UUID:             uuid.New().String(),
		MD5:              fileMD5,
		OriginalFilename: file.Filename,
		FileSize:         file.Size,
		ContentType:      file.Header.Get("Content-Type"),
		Width:            width,
		Height:           height,
		UserID:           userID,
	}
	if err := database.DB.Create(&image).Error; err != nil {
		return nil, fmt.Errorf("failed to create shared image record: %w", err)
	}

	// Create new storage location records pointing to the OLD physical files.
	var newLocations []database.StorageLocation
	for _, loc := range existingImage.StorageLocations {
		if loc.IsActive { // Only copy active locations
			newLocations = append(newLocations, database.StorageLocation{
				ImageID:          image.ID,
				BackendID:        loc.BackendID,
				StorageType:      loc.StorageType,
				URL:              loc.URL,
				DeleteIdentifier: loc.DeleteIdentifier,
				IsActive:         true,
			})
		}
	}

	if len(newLocations) > 0 {
		if err := database.DB.Create(&newLocations).Error; err != nil {
			database.DB.Delete(&image)
			return nil, fmt.Errorf("failed to link shared storage locations: %w", err)
		}
	}

	database.DB.Preload("StorageLocations.Backend").First(&image, image.ID)
	return image, nil
}

func distributeToBackends(file *multipart.FileHeader, uniqueFilename string, imageID uint, backends []database.Backend, storageManager *manager.StorageManager) {
	var wg sync.WaitGroup
	for _, backend := range backends {
		wg.Add(1)
		go func(b database.Backend) {
			defer wg.Done()
			uploader, found := storageManager.Get(b.ID)
			if !found {
				log.Printf("Uploader not found for backend %s (ID: %d), skipping.", b.Name, b.ID)
				return
			}

			fileReader, err := file.Open()
			if err != nil {
				log.Printf("Failed to open file for backend %s: %v", b.Name, err)
				return
			}
			defer fileReader.Close()

			uploadResultURL, err := uploader.Upload(file, uniqueFilename, fileReader)
			if err != nil {
				log.Printf("Failed to upload to %s (type: %s): %v", b.Name, uploader.Type(), err)
				return
			}

			finalURL, deleteIdentifier := parseUploadResult(uploadResultURL, uploader.Type())
			location := database.StorageLocation{
				ImageID:          imageID,
				BackendID:        b.ID,
				StorageType:      uploader.Type(),
				URL:              finalURL,
				DeleteIdentifier: deleteIdentifier,
				IsActive:         true,
			}
			database.DB.Create(&location)
			log.Printf("Successfully uploaded to backend: %s, URL: %s", b.Name, finalURL)
		}(backend)
	}
	wg.Wait()
}

// DeleteImage deletes an image and its stored files from all backends.
func DeleteImage(imageUUID string, userID uint, userRole string, storageManager *manager.StorageManager) error {
	var image database.Image
	query := database.DB.Preload("StorageLocations").Where("uuid = ?", imageUUID)
	if userRole != "admin" {
		query = query.Where("user_id = ?", userID)
	}
	err := query.First(&image).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("image not found or permission denied")
		}
		return err
	}

	var count int64
	database.DB.Model(&database.Image{}).Where("md5 = ? AND id != ?", image.MD5, image.ID).Count(&count)

	if count == 0 {
		var wg sync.WaitGroup
		for _, loc := range image.StorageLocations {
			wg.Add(1)
			go func(location database.StorageLocation) {
				defer wg.Done()
				uploader, found := storageManager.Get(location.BackendID)
				if !found {
					log.Printf("Uploader for BackendID %d not found, cannot delete file at %s", location.BackendID, location.URL)
					return
				}
				deleteID := location.DeleteIdentifier
				if location.StorageType == "local" {
					if parsedURL, err := url.Parse(location.URL); err == nil {
						deleteID = path.Base(parsedURL.Path)
					}
				}
				if err := uploader.Delete(deleteID); err != nil {
					log.Printf("Failed to delete file from %s (URL: %s): %v", location.StorageType, location.URL, err)
				} else {
					log.Printf("Successfully deleted file from %s (URL: %s)", location.StorageType, location.URL)
				}
			}(loc)
		}
		wg.Wait()
	} else {
		log.Printf("Skipping physical file deletion for MD5 %s as it is referenced by other records.", image.MD5)
	}

	return database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&database.StorageLocation{}, "image_id = ?", image.ID).Error; err != nil {
			return err
		}
		return tx.Delete(&image).Error
	})
}

func GetHealthyStorageLocation(imageUUID string) (*database.StorageLocation, error) {
	var image database.Image
	err := database.DB.Preload("StorageLocations.Backend").Where("uuid = ?", imageUUID).First(&image).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("image not found")
		}
		return nil, err
	}

	maxFailures := GetRetryCount()
	accessPolicy := GetAccessPolicy()

	var availableLocations []database.StorageLocation
	for _, loc := range image.StorageLocations {
		failureCheckPassed := (maxFailures == 0) || (loc.FailureCount < maxFailures)
		if loc.IsActive && loc.Backend.AllowRedirect && failureCheckPassed {
			availableLocations = append(availableLocations, loc)
		}
	}

	if len(availableLocations) == 0 {
		return nil, errors.New("no available storage locations for this image")
	}

	if accessPolicy == "priority" {
		sort.Slice(availableLocations, func(i, j int) bool {
			return availableLocations[i].Backend.Priority < availableLocations[j].Backend.Priority
		})
	} else {
		rand.Seed(time.Now().UnixNano())
		rand.Shuffle(len(availableLocations), func(i, j int) {
			availableLocations[i], availableLocations[j] = availableLocations[j], availableLocations[i]
		})
	}

	// --- 已修改：为无限重试模式增加特殊处理 ---
	if maxFailures == 0 {
		// 在无限重试模式下，我们信任链接，不进行健康检查，直接返回第一个
		if len(availableLocations) > 0 {
			return &availableLocations[0], nil
		}
		// 如果没有可用的（比如都被手动禁用了），则继续执行到最后的错误返回
	}

	// 对于有限重试模式，执行健康检查
	for i := range availableLocations {
		loc := &availableLocations[i]
		isHealthy := false
		if loc.StorageType == "local" {
			if parsedURL, err := url.Parse(loc.URL); err == nil {
				if _, err := os.Stat("." + parsedURL.Path); err == nil {
					isHealthy = true
				}
			}
		} else {
			isHealthy = checkURLHealth(loc.URL)
		}

		if isHealthy {
			if loc.FailureCount > 0 {
				go func(locationID uint) {
					database.DB.Model(&database.StorageLocation{}).Where("id = ?", locationID).Update("failure_count", 0)
				}(loc.ID)
			}
			return loc, nil
		} else {
			go func(locationID uint) {
				database.DB.Model(&database.StorageLocation{}).Where("id = ?", locationID).Update("failure_count", gorm.Expr("failure_count + 1"))
			}(loc.ID)
		}
	}

	return nil, errors.New("all available storage locations are currently unreachable")
}

func ListImages(userID uint, userRole string, keyword string, page int, pageSize int) (*ListImagesResponse, error) {
	var images []database.Image
	var total int64

	query := database.DB.Model(&database.Image{}).Preload("StorageLocations").Order("created_at desc")

	if userRole != "admin" {
		query = query.Where("user_id = ?", userID)
	}

	if keyword != "" {
		query = query.Where("original_filename LIKE ?", "%"+keyword+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	offset := (page - 1) * pageSize
	if err := query.Limit(pageSize).Offset(offset).Find(&images).Error; err != nil {
		return nil, err
	}

	return &ListImagesResponse{
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Images:   images,
	}, nil
}

// BatchBackfillImagesForUser starts a backfill task, ensuring the user owns all images.
func BatchBackfillImagesForUser(imageUUIDs []string, backendID uint, userID uint, storageManager *manager.StorageManager) (string, error) {
	var count int64
	database.DB.Model(&database.Image{}).Where("uuid IN ? AND user_id = ?", imageUUIDs, userID).Count(&count)
	if count != int64(len(imageUUIDs)) {
		return "", errors.New("permission denied: you do not own all the selected images")
	}

	return BatchBackfillToBackend(imageUUIDs, backendID, storageManager)
}

func BatchSetRandomStatus(imageUUIDs []string, allowRandom bool) error {
	if err := database.DB.Model(&database.Image{}).Where("uuid IN ?", imageUUIDs).Update("allow_random", allowRandom).Error; err != nil {
		return err
	}
	go UpdateRandomImageCache()
	return nil
}

func BatchDeleteImagesForUser(imageUUIDs []string, userID uint, storageManager *manager.StorageManager) (string, error) {
	var count int64
	database.DB.Model(&database.Image{}).Where("uuid IN ? AND user_id = ?", imageUUIDs, userID).Count(&count)
	if count != int64(len(imageUUIDs)) {
		return "", errors.New("permission denied: you do not own all the selected images")
	}

	// Pass "user" role to ensure underlying functions respect user-level constraints
	return BatchDeleteImages(imageUUIDs, userID, "user", storageManager)
}

func BatchDeleteImages(imageUUIDs []string, userID uint, userRole string, storageManager *manager.StorageManager) (string, error) {
	taskID := uuid.New().String()
	task := &Task{
		ID: taskID, Type: "Batch Delete", Status: "running",
		Total: len(imageUUIDs), CreatedAt: time.Now(),
	}
	taskMu.Lock()
	tasks[taskID] = task
	taskMu.Unlock()

	go func() {
		for i, uuid := range imageUUIDs {
			if err := DeleteImage(uuid, userID, userRole, storageManager); err != nil {
				log.Printf("Batch delete error for UUID %s: %v", uuid, err)
			}
			taskMu.Lock()
			tasks[taskID].Progress = i + 1
			taskMu.Unlock()
		}
		taskMu.Lock()
		tasks[taskID].Status = "completed"
		taskMu.Unlock()
	}()

	return taskID, nil
}

func BatchBackfillToBackend(imageUUIDs []string, backendID uint, storageManager *manager.StorageManager) (string, error) {
	taskID := uuid.New().String()
	task := &Task{
		ID: taskID, Type: "Batch Backfill", Status: "running",
		Total: len(imageUUIDs), CreatedAt: time.Now(),
	}
	taskMu.Lock()
	tasks[taskID] = task
	taskMu.Unlock()

	go func() {
		targetUploader, found := storageManager.Get(backendID)
		if !found {
			taskMu.Lock()
			task.Status = "failed"
			task.Message = "Target backend not found"
			taskMu.Unlock()
			return
		}

		for i, uuid := range imageUUIDs {
			func() {
				var image database.Image
				if err := database.DB.Preload("StorageLocations").Where("uuid = ?", uuid).First(&image).Error; err != nil {
					return
				}

				existsOnTarget := false
				for _, loc := range image.StorageLocations {
					if loc.BackendID == backendID {
						existsOnTarget = true
						break
					}
				}

				if !existsOnTarget {
					var localPath string
					for _, loc := range image.StorageLocations {
						if loc.StorageType == "local" {
							if parsedURL, err := url.Parse(loc.URL); err == nil {
								localPath = filepath.Join(".", parsedURL.Path)
							}
							break
						}
					}
					if localPath != "" {
						if err := backfillFromLocalFile(&image, localPath, backendID, targetUploader); err != nil {
							log.Printf("[Task %s] Backfill FAILED for %s: %v", taskID, uuid, err)
						}
					}
				}
			}()

			taskMu.Lock()
			tasks[taskID].Progress = i + 1
			taskMu.Unlock()
		}

		taskMu.Lock()
		tasks[taskID].Status = "completed"
		taskMu.Unlock()
	}()

	return taskID, nil
}

func backfillFromLocalFile(image *database.Image, localPath string, targetBackendID uint, targetUploader storage.Uploader) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file %s: %w", localPath, err)
	}
	defer file.Close()

	fileInfo, _ := file.Stat()

	tempHeader := &multipart.FileHeader{
		Filename: image.OriginalFilename,
		Size:     fileInfo.Size(),
	}

	uniqueFilename := fmt.Sprintf("%s%s", image.UUID, filepath.Ext(image.OriginalFilename))
	uploadResultURL, err := targetUploader.Upload(tempHeader, uniqueFilename, file)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	finalURL, deleteIdentifier := parseUploadResult(uploadResultURL, targetUploader.Type())
	location := database.StorageLocation{
		ImageID:          image.ID,
		BackendID:        targetBackendID,
		StorageType:      targetUploader.Type(),
		URL:              finalURL,
		DeleteIdentifier: deleteIdentifier,
		IsActive:         true,
	}
	return database.DB.Create(&location).Error
}

func parseUploadResult(result, uploaderType string) (string, string) {
	finalURL := result
	deleteIdentifier := ""

	if uploaderType == "sm.ms" || uploaderType == "oss" {
		parts := strings.Split(result, "@@@")
		if len(parts) == 2 {
			finalURL = parts[0]
			deleteIdentifier = parts[1]
			return finalURL, deleteIdentifier // Success, return early
		}
	}

	// Fallback for OSS if '@@@' is missing
	if uploaderType == "oss" {
		if parsedURL, err := url.Parse(result); err == nil {
			deleteIdentifier = strings.TrimPrefix(parsedURL.Path, "/")
		}
	}

	return finalURL, deleteIdentifier
}

func GetTasks() []*Task {
	taskMu.Lock()
	defer taskMu.Unlock()
	taskList := make([]*Task, 0, len(tasks))
	for _, task := range tasks {
		taskList = append(taskList, task)
	}
	return taskList
}

func checkURLHealth(url string) bool {
	client := http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		log.Printf("Failed to create HEAD request for %s: %v", url, err)
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Health check failed for %s: %v", url, err)
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

func InitRandomImageCache() {
	log.Println("Initializing random image cache...")
	UpdateRandomImageCache()

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			log.Println("Updating random image cache on schedule...")
			UpdateRandomImageCache()
		}
	}()
}

func UpdateRandomImageCache() {
	var uuids []string
	database.DB.Model(&database.Image{}).Where("allow_random = ?", true).Pluck("uuid", &uuids)
	cacheMutex.Lock()
	randomImageUUIDs = uuids
	cacheMutex.Unlock()
	log.Printf("Random image cache updated. Total images in pool: %d", len(uuids))
}

func GetRandomImageUUID() (string, error) {
	cacheMutex.RLock()
	defer cacheMutex.RUnlock()
	if len(randomImageUUIDs) == 0 {
		return "", errors.New("no images available in the random pool")
	}
	return randomImageUUIDs[rand.Intn(len(randomImageUUIDs))], nil
}
