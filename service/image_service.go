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

var (
	randomImageUUIDs []string
	cacheMutex       sync.RWMutex
)

// Global task manager for batch jobs (in a real production app, this might be a Redis queue or similar)
var (
	tasks  = make(map[string]*Task)
	taskMu sync.Mutex
)

// ListImagesResponse is the new structure for paginated image lists.
type ListImagesResponse struct {
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"pageSize"`
	Images   []database.Image `json:"images"`
}

type Task struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Status    string    `json:"status"` // "running", "completed", "failed"
	Progress  int       `json:"progress"`
	Total     int       `json:"total"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

func InitAndStartCacheUpdater() {
	log.Println("Initializing random image cache...")
	if err := UpdateRandomImageCache(); err != nil {
		log.Printf("Initial random image cache update failed: %v", err)
	}

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			log.Println("Periodically updating random image cache...")
			if err := UpdateRandomImageCache(); err != nil {
				log.Printf("Periodic random image cache update failed: %v", err)
			}
		}
	}()
}

// UpdateRandomImageCache 从数据库加载所有允许随机访问的图片UUID到内存
func UpdateRandomImageCache() error {
	var uuids []string
	if err := database.DB.Model(&database.Image{}).Where("allow_random = ?", true).Pluck("uuid", &uuids).Error; err != nil {
		log.Printf("Error updating random image cache: %v", err)
		return err
	}

	cacheMutex.Lock()
	randomImageUUIDs = uuids
	cacheMutex.Unlock()

	log.Printf("Random image cache updated. Loaded %d image UUIDs.", len(uuids))
	return nil
}

// StartRandomImageCacheUpdater 启动一个后台任务，定期刷新随机图片缓存
func StartRandomImageCacheUpdater() {
	go UpdateRandomImageCache() // 程序启动时立即执行一次

	ticker := time.NewTicker(5 * time.Minute) // 每5分钟刷新一次
	go func() {
		for range ticker.C {
			UpdateRandomImageCache()
		}
	}()
}

// GetRandomImageUUID 从内存缓存中随机获取一个图片UUID
func GetRandomImageUUID() (string, error) {
	cacheMutex.RLock()
	defer cacheMutex.RUnlock()

	if len(randomImageUUIDs) == 0 {
		return "", errors.New("no images available in the random pool")
	}

	rand.Seed(time.Now().UnixNano())
	randomIndex := rand.Intn(len(randomImageUUIDs))
	return randomImageUUIDs[randomIndex], nil
}

func BatchSetRandomStatus(imageUUIDs []string, allowRandom bool) error {
	if len(imageUUIDs) == 0 {
		return nil // 无操作
	}
	result := database.DB.Model(&database.Image{}).Where("uuid IN ?", imageUUIDs).Update("allow_random", allowRandom)
	if result.Error != nil {
		return fmt.Errorf("failed to batch update random status: %w", result.Error)
	}
	log.Printf("Batch updated allow_random to %v for %d images. Rows affected: %d", allowRandom, len(imageUUIDs), result.RowsAffected)
	go UpdateRandomImageCache() // 异步更新缓存
	return nil
}

// ToggleImageRandomStatus 切换图片的AllowRandom状态
func ToggleImageRandomStatus(imageUUID string) (*database.Image, error) {
	var image database.Image
	if err := database.DB.Where("uuid = ?", imageUUID).First(&image).Error; err != nil {
		return nil, err
	}

	image.AllowRandom = !image.AllowRandom
	if err := database.DB.Save(&image).Error; err != nil {
		return nil, err
	}

	// 状态更新后，立即异步刷新缓存，以便即时生效
	go UpdateRandomImageCache()

	return &image, nil
}

// getImageDimensions extracts width and height from an image file.
func getImageDimensions(file *multipart.FileHeader) (int, int, error) {
	src, err := file.Open()
	if err != nil {
		return 0, 0, err
	}
	defer src.Close()

	// FIX: This is critical. We must seek to the beginning of the file
	// because it might have been read before (e.g., for MD5 calculation).
	if _, err := src.Seek(0, 0); err != nil {
		return 0, 0, fmt.Errorf("failed to seek file start: %w", err)
	}

	config, _, err := image.DecodeConfig(src)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to decode image config: %w", err)
	}
	return config.Width, config.Height, nil
}

func UploadImage(file *multipart.FileHeader, userID uint, targetBackendIDs []uint, storageManager *manager.StorageManager) (*database.Image, error) {
	// 1. Calculate MD5 for deduplication
	fileMD5, err := util.CalculateFileMD5(file)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate file MD5: %w", err)
	}

	// 2. Check if an image with the same MD5 already exists
	var existingImage database.Image
	err = database.DB.Preload("StorageLocations.Backend").Where("md5 = ?", fileMD5).First(&existingImage).Error

	// a) If image exists (MD5 match)
	if err == nil {
		log.Printf("Duplicate image detected (MD5: %s). Checking for backends to backfill.", fileMD5)
		return handleDuplicateImage(file, &existingImage, targetBackendIDs, storageManager)
	}

	// b) If there's a different database error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("database error during MD5 check: %w", err)
	}

	// c) If the image is new, now we get its dimensions
	log.Printf("New image detected (MD5: %s). Getting dimensions.", fileMD5)

	// FIX: Actually call the function to get dimensions
	width, height, err := getImageDimensions(file)
	if err != nil {
		// We can decide to fail or just log the error and continue with 0x0
		log.Printf("Could not get image dimensions for %s: %v. Proceeding with 0x0.", file.Filename, err)
		width = 0
		height = 0
	}

	log.Printf("Starting new upload process for image with dimensions %dx%d.", width, height)
	// FIX: Pass the acquired dimensions to the handler function
	return handleNewImage(file, userID, fileMD5, width, height, targetBackendIDs, storageManager)
}

// ** MODIFIED handleNewImage FUNCTION **
// It now accepts width and height and saves them.
func handleNewImage(file *multipart.FileHeader, userID uint, fileMD5 string, width, height int, targetBackendIDs []uint, storageManager *manager.StorageManager) (*database.Image, error) {
	// 1. Determine target backends
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

	// 2. Create image metadata record, now including dimensions
	imageUUID := uuid.New().String()
	image := &database.Image{
		UUID:             imageUUID,
		MD5:              fileMD5,
		OriginalFilename: file.Filename,
		FileSize:         file.Size,
		ContentType:      file.Header.Get("Content-Type"),
		Width:            width,  // Save width
		Height:           height, // Save height
		UserID:           userID,
	}
	if err := database.DB.Create(&image).Error; err != nil {
		return nil, fmt.Errorf("failed to create image record in database: %w", err)
	}

	// 3. Distribute the file to backends
	uniqueFilename := fmt.Sprintf("%s%s", imageUUID, filepath.Ext(file.Filename))
	distributeToBackends(file, uniqueFilename, image.ID, activeBackends, storageManager)

	// 4. Reload image info and return the result
	database.DB.Preload("StorageLocations.Backend").First(&image, image.ID)
	if len(image.StorageLocations) == 0 {
		database.DB.Delete(&image) // Rollback if all uploads failed
		return nil, errors.New("upload failed on all active backends")
	}

	return image, nil
}

// --- NO CHANGES TO THE FUNCTIONS BELOW ---

// handleDuplicateImage handles the logic for re-uploading an existing image to new backends.
func handleDuplicateImage(file *multipart.FileHeader, existingImage *database.Image, targetBackendIDs []uint, storageManager *manager.StorageManager) (*database.Image, error) {
	// 1. Find out which backends need backfilling
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

	// 2. If no backfill is needed, just return the existing info
	if len(backendsToBackfill) == 0 {
		log.Printf("Image fully duplicated on all target backends. No backfill needed.")
		return existingImage, nil
	}

	// 3. Otherwise, distribute the file to the missing backends
	log.Printf("Backfilling image to %d new backends.", len(backendsToBackfill))
	uniqueFilename := fmt.Sprintf("%s%s", existingImage.UUID, filepath.Ext(file.Filename))
	distributeToBackends(file, uniqueFilename, existingImage.ID, backendsToBackfill, storageManager)

	// 4. Reload and return image info
	database.DB.Preload("StorageLocations.Backend").First(&existingImage, existingImage.ID)
	return existingImage, nil
}

// distributeToBackends concurrently uploads a file to a list of specified backends.
func distributeToBackends(file *multipart.FileHeader, uniqueFilename string, imageID uint, backends []database.Backend, storageManager *manager.StorageManager) {
	var wg sync.WaitGroup

	for _, backend := range backends {
		wg.Add(1)
		go func(b database.Backend) {
			defer wg.Done()

			uploader, found := storageManager.Get(b.ID)
			if !found {
				log.Printf("Uploader not found for backend %s (ID: %d), skipping upload.", b.Name, b.ID)
				return
			}

			uploadResultURL, err := uploader.Upload(file, uniqueFilename)
			if err != nil {
				log.Printf("Failed to upload to %s (type: %s): %v", b.Name, uploader.Type(), err)
				return
			}

			finalURL := uploadResultURL
			deleteIdentifier := ""

			if uploader.Type() == "sm.ms" {
				parts := strings.Split(uploadResultURL, "@@@")
				if len(parts) == 2 {
					finalURL = parts[0]
					deleteIdentifier = parts[1]
				}
			}

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
	err := database.DB.Preload("StorageLocations").Where("uuid = ?", imageUUID).First(&image).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("image not found")
		}
		return err
	}

	if userRole != "admin" && image.UserID != userID {
		return errors.New("permission denied to delete this image")
	}

	var wg sync.WaitGroup
	for _, loc := range image.StorageLocations {
		wg.Add(1)
		go func(location database.StorageLocation) {
			defer wg.Done()
			uploader, found := storageManager.Get(location.BackendID)
			if !found {
				log.Printf("Uploader for BackendID %d not found, cannot delete physical file at %s", location.BackendID, location.URL)
				return
			}
			deleteID := location.DeleteIdentifier
			if location.StorageType == "local" {
				parsedURL, err := url.Parse(location.URL)
				if err == nil {
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

	return database.DB.Transaction(func(tx *gorm.DB) error {
		if len(image.StorageLocations) > 0 {
			if err := tx.Delete(&image.StorageLocations).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&image).Error
	})
}

// GetImageRedirectURL gets a publicly accessible URL for an image.
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
	// --- 2. 新增: 从内存缓存获取访问策略 ---
	accessPolicy := GetAccessPolicy()

	var availableLocations []database.StorageLocation
	for _, loc := range image.StorageLocations {
		// 筛选条件: 图片自身的存储位置是激活的 AND 其所属的后端是允许跳转的 AND 失败次数未达上限
		if loc.IsActive && loc.Backend.AllowRedirect && loc.FailureCount < maxFailures {
			availableLocations = append(availableLocations, loc)
		}
	}

	if len(availableLocations) == 0 {
		return nil, errors.New("no available storage locations for this image, all are inactive or have reached the failure limit")
	}

	// --- 3. 核心修改: 根据访问策略决定排序方式 ---
	log.Printf("Applying access policy: '%s'", accessPolicy)
	if accessPolicy == "priority" {
		// 优先级策略: 按照 Backend 的 Priority 字段从小到大排序
		sort.Slice(availableLocations, func(i, j int) bool {
			return availableLocations[i].Backend.Priority < availableLocations[j].Backend.Priority
		})
	} else {
		// 默认或随机策略: 随机打乱列表顺序
		rand.Seed(time.Now().UnixNano())
		rand.Shuffle(len(availableLocations), func(i, j int) {
			availableLocations[i], availableLocations[j] = availableLocations[j], availableLocations[i]
		})
	}
	// --- 策略应用结束 ---

	for i := range availableLocations {
		loc := &availableLocations[i]
		var isHealthy bool

		if loc.StorageType == "local" {
			parsedURL, err := url.Parse(loc.URL)
			if err != nil {
				log.Printf("[Health Check] Failed to parse local URL %s: %v", loc.URL, err)
				isHealthy = false
			} else {
				localPath := "." + parsedURL.Path
				if _, err := os.Stat(localPath); err == nil {
					isHealthy = true
				} else {
					isHealthy = false
				}
			}
		} else {
			isHealthy = checkURLHealth(loc.URL)
		}

		if isHealthy {
			// 2. --- 修改点：异步重置失败次数 ---
			if loc.FailureCount > 0 {
				go func(locationID uint) {
					database.DB.Model(&database.StorageLocation{}).Where("id = ?", locationID).Update("failure_count", 0)
					log.Printf("Asynchronously reset failure count for StorageLocation ID %d.", locationID)
				}(loc.ID)
			}
			// 立即返回健康的链接，不再等待数据库操作
			return loc, nil
		} else {
			// 3. --- 修改点：异步增加失败次数 ---
			go func(locationID uint) {
				database.DB.Model(&database.StorageLocation{}).Where("id = ?", locationID).Update("failure_count", gorm.Expr("failure_count + 1"))
				log.Printf("Asynchronously incremented failure count for StorageLocation ID %d.", locationID)
			}(loc.ID)
		}
	}

	return nil, errors.New("all available storage locations are currently unreachable")
}

// ListImages now supports pagination.
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

// BatchDeleteImages starts a background task to delete images.
func BatchDeleteImages(imageUUIDs []string, userID uint, userRole string, storageManager *manager.StorageManager) (string, error) {
	taskID := uuid.New().String()
	task := &Task{
		ID:        taskID,
		Type:      "Batch Delete",
		Status:    "running",
		Total:     len(imageUUIDs),
		CreatedAt: time.Now(),
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

// BatchBackfillToBackend starts a background task to re-upload images.
func BatchBackfillToBackend(imageUUIDs []string, backendID uint, storageManager *manager.StorageManager) (string, error) {
	taskID := uuid.New().String()
	task := &Task{
		ID:        taskID,
		Type:      "Batch Backfill (from Local)",
		Status:    "running",
		Total:     len(imageUUIDs),
		CreatedAt: time.Now(),
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
			log.Printf("[Task %s] Failed: Target backend ID %d not found", taskID, backendID)
			return
		}

		for i, uuid := range imageUUIDs {
			func() { // Use a closure to ensure DB connections are handled correctly in the loop
				var image database.Image
				if err := database.DB.Preload("StorageLocations").Where("uuid = ?", uuid).First(&image).Error; err != nil {
					log.Printf("[Task %s] Skipping %s: image record not found", taskID, uuid)
					return // to next iteration
				}

				existsOnTarget := false
				for _, loc := range image.StorageLocations {
					if loc.BackendID == backendID {
						existsOnTarget = true
						break
					}
				}

				if existsOnTarget {
					log.Printf("[Task %s] Skipping %s: already exists on target backend", taskID, uuid)
				} else {
					var localPath string
					for _, loc := range image.StorageLocations {
						if loc.StorageType == "local" {
							parsedURL, err := url.Parse(loc.URL)
							if err == nil {
								localPath = filepath.Join(".", parsedURL.Path)
							}
							break
						}
					}

					if localPath == "" {
						log.Printf("[Task %s] Skipping %s: no local storage record found", taskID, uuid)
					} else {
						// Use the new, correct helper function
						err := backfillFromLocalFile(&image, localPath, backendID, targetUploader)
						if err != nil {
							log.Printf("[Task %s] Backfill FAILED for %s from %s: %v", taskID, uuid, localPath, err)
						} else {
							log.Printf("[Task %s] Backfill SUCCESS for %s from %s", taskID, uuid, localPath)
						}
					}
				}
			}() // End of closure

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

// ** CORRECTED backfillFromLocalFile HELPER FUNCTION **
func backfillFromLocalFile(image *database.Image, localPath string, targetBackendID uint, targetUploader storage.Uploader) error {
	// 1. Check if the local file actually exists
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		return fmt.Errorf("local source file not found at %s", localPath)
	}

	// 2. Execute upload using the new interface method
	uniqueFilename := fmt.Sprintf("%s%s", image.UUID, filepath.Ext(image.OriginalFilename))
	uploadResultURL, err := targetUploader.UploadFromFile(localPath, uniqueFilename)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	// 3. Save the new storage location record to the database
	finalURL := uploadResultURL
	deleteIdentifier := ""
	if targetUploader.Type() == "sm.ms" {
		parts := strings.Split(uploadResultURL, "@@@")
		if len(parts) == 2 {
			finalURL = parts[0]
			deleteIdentifier = parts[1]
		}
	}

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

// GetTasks returns the list of all tasks.
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
	client := http.Client{
		Timeout: 5 * time.Second, // 设置5秒超时
	}
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

	// 认为 2xx 或 3xx 的状态码是健康的
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return true
	}

	log.Printf("Health check for %s returned non-success status: %s", url, resp.Status)
	return false
}
