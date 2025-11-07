package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// cleanupOrphanedRecordingsHandler - Cleanup orphaned recordings from deleted cameras
func cleanupOrphanedRecordingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	nvr.mu.RLock()
	validDirs := make(map[string]bool)
	for _, cam := range nvr.cameras {
		validDirs[filepath.Base(cam.RecordDir)] = true
	}
	nvr.mu.RUnlock()

	totalDeleted := 0
	var totalSize int64

	entries, err := os.ReadDir("/recordings")
	if err != nil {
		log.Printf("Failed to read recordings directory: %v", err)
		http.Error(w, "Failed to read recordings directory", http.StatusInternalServerError)
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirPath := filepath.Join("/recordings", entry.Name())

		// Clean up orphaned directories (from deleted cameras)
		if !validDirs[entry.Name()] {
			pattern := filepath.Join(dirPath, "*.mp4")
			files, err := filepath.Glob(pattern)
			if err != nil {
				continue
			}

			for _, file := range files {
				info, err := os.Stat(file)
				if err != nil {
					continue
				}
				totalSize += info.Size()

				if err := os.Remove(file); err != nil {
					log.Printf("Failed to delete orphaned recording %s: %v", file, err)
				} else {
					totalDeleted++
				}
			}

			os.Remove(dirPath)
		} else {
			// For valid camera directories, clean up stale .tmp files
			tmpFiles, err := filepath.Glob(filepath.Join(dirPath, "*.tmp"))
			if err != nil {
				continue
			}

			// Sort by modification time to identify the most recent one
			type fileInfo struct {
				path    string
				modTime time.Time
				size    int64
			}

			var tmpFileInfos []fileInfo
			for _, file := range tmpFiles {
				info, err := os.Stat(file)
				if err != nil {
					continue
				}
				tmpFileInfos = append(tmpFileInfos, fileInfo{
					path:    file,
					modTime: info.ModTime(),
					size:    info.Size(),
				})
			}

			// Sort by modification time descending (most recent first)
			sort.Slice(tmpFileInfos, func(i, j int) bool {
				return tmpFileInfos[i].modTime.After(tmpFileInfos[j].modTime)
			})

			// Delete all but the most recent .tmp file (which might be in-progress)
			for i, tmpFile := range tmpFileInfos {
				if i == 0 {
					// Skip the most recent .tmp file
					log.Printf("Skipping most recent .tmp file (in-progress): %s", filepath.Base(tmpFile.path))
					continue
				}

				totalSize += tmpFile.size
				if err := os.Remove(tmpFile.path); err != nil {
					log.Printf("Failed to delete stale .tmp file %s: %v", tmpFile.path, err)
				} else {
					totalDeleted++
					log.Printf("Deleted stale .tmp file: %s (%.2f MB)", 
						filepath.Base(tmpFile.path), float64(tmpFile.size)/(1024*1024))
				}
			}
		}
	}

	sizeMB := float64(totalSize) / (1024 * 1024)
	log.Printf("Cleaned up %d orphaned files (%.2f MB)", totalDeleted, sizeMB)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"count":   totalDeleted,
		"size_mb": sizeMB,
	})
}

// cleanupRoutine - Background task that runs every 5 minutes to clean old recordings
func cleanupRoutine() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	log.Println("Cleanup routine started")
	for range ticker.C {
		nvr.mu.RLock()
		for _, cam := range nvr.cameras {
			retentionDays := cam.RetentionDays
			if retentionDays == 0 {
				retentionDays = 7
			}
			retention := time.Duration(retentionDays) * 24 * time.Hour
			cleanupOldRecordings(cam.RecordDir, retention)
			cleanupTempFiles(cam.RecordDir) // Clean up stale .tmp files
		}
		nvr.mu.RUnlock()
	}
}

// cleanupOldRecordings - Delete recordings older than retention period
func cleanupOldRecordings(dir string, retention time.Duration) {
	files, err := filepath.Glob(filepath.Join(dir, "*.mp4"))
	if err != nil {
		return
	}

	// FIXED: Calculate cutoff in UTC
	cutoff := time.Now().UTC().Add(-retention)
	for _, file := range files {
		filename := filepath.Base(file)
		if len(filename) < 15 {
			continue
		}

		// Extract timestamp from filename (e.g., "20241105_120000_camera.mp4")
		timeStr := filename[0:15]
		// FIXED: Parse filenames in UTC
		recordingStart, err := time.Parse("20060102_150405", timeStr)
		if err != nil {
			// Fallback to ModTime for files with non-standard names
			info, err := os.Stat(file)
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				size := float64(info.Size()) / (1024 * 1024)
				if err := os.Remove(file); err != nil {
					log.Printf("Failed to delete old recording %s: %v", file, err)
				} else {
					log.Printf("Cleaned up old recording: %s (%.2f MB, age: %.1f days)",
						filename, size, time.Since(info.ModTime()).Hours()/24)
				}
			}
			continue
		}

		// FIXED: Delete if recording started before cutoff (in UTC)
		if recordingStart.Before(cutoff) {
			info, _ := os.Stat(file)
			size := float64(info.Size()) / (1024 * 1024)
			if err := os.Remove(file); err != nil {
				log.Printf("Failed to delete old recording %s: %v", file, err)
			} else {
				log.Printf("Cleaned up old recording: %s (%.2f MB, started at %s UTC)",
					filename, size, recordingStart.Format("2006-01-02 15:04:05"))
			}
		}
	}
}

// cleanupTempFiles - Delete stale .tmp files (older than 15 minutes)
func cleanupTempFiles(dir string) {
	files, err := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if err != nil {
		return
	}

	// .tmp files older than 15 minutes are considered stale
	cutoff := time.Now().Add(-15 * time.Minute)
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			size := float64(info.Size()) / (1024 * 1024)
			err := os.Remove(file)
			if err != nil {
				log.Printf("Failed to delete stale temp file %s: %v", file, err)
			} else {
				log.Printf("Cleaned up stale temp file: %s (%.2f MB, age: %.1f minutes)",
					filepath.Base(file), size, time.Since(info.ModTime()).Minutes())
			}
		}
	}
}
