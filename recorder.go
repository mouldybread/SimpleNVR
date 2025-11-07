package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// BufferManager manages the 30-second circular buffer for event-based recording
type BufferManager struct {
	camera      *Camera
	buffer      []BufferSegment
	bufferMutex sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	eventChan   chan Event
	recording   bool
	recordMutex sync.Mutex
}

type BufferSegment struct {
	StartTime time.Time
	FilePath  string
}

const (
	bufferDuration = 30 * time.Second
	segmentSize    = 5 * time.Second
)

// Continuous recording (existing functionality)
func recordCamera(ctx context.Context, cam *Camera) {
	log.Printf("Starting continuous recording for %s (retention: %d days)", cam.Name, cam.RetentionDays)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Stopped recording for %s", cam.Name)
			return
		default:
			err := recordSegment(ctx, cam, 600) // 10-minute segments
			if err != nil {
				log.Printf("Recording error for %s: %v. Retrying in 5s...", cam.Name, err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
					continue
				}
			}
		}
	}
}

// Event-based recording with 30-second pre-recording buffer
func recordCameraEventBased(ctx context.Context, cam *Camera) {
	log.Printf("Starting event-based recording for %s with 30s pre-buffer", cam.Name)

	bufferCtx, bufferCancel := context.WithCancel(ctx)
	bm := &BufferManager{
		camera:    cam,
		buffer:    make([]BufferSegment, 0),
		ctx:       bufferCtx,
		cancel:    bufferCancel,
		eventChan: make(chan Event, 10),
	}

	cam.bufferManager = bm

	// Start buffer maintenance goroutine
	go bm.maintainBuffer()

	// Handle incoming events
	for {
		select {
		case <-ctx.Done():
			bufferCancel()
			log.Printf("Stopped event-based recording for %s", cam.Name)
			return
		case event := <-bm.eventChan:
			go bm.handleEvent(event)
		}
	}
}

// maintainBuffer keeps a rolling 30-second buffer
func (bm *BufferManager) maintainBuffer() {
	log.Printf("Starting buffer maintenance for %s", bm.camera.Name)

	for {
		select {
		case <-bm.ctx.Done():
			bm.cleanupBuffer()
			return
		default:
			err := bm.recordBufferSegment()
			if err != nil {
				log.Printf("Buffer recording error for %s: %v. Retrying in 2s...", bm.camera.Name, err)
				time.Sleep(2 * time.Second)
				continue
			}
		}
	}
}

// recordBufferSegment records a small segment to the buffer
func (bm *BufferManager) recordBufferSegment() error {
	now := time.Now()
	filename := fmt.Sprintf("buffer_%s_%s.mp4",
		now.Format("20060102_150405"),
		sanitizeFilename(bm.camera.Name))

	bufferDir := filepath.Join(bm.camera.RecordDir, "buffer")
	os.MkdirAll(bufferDir, 0755)

	filePath := filepath.Join(bufferDir, filename)
	tempPath := filePath + ".tmp"

	segmentCtx, cancel := context.WithTimeout(bm.ctx, segmentSize+2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(segmentCtx, "ffmpeg",
		"-rtsp_transport", "tcp",
		"-i", bm.camera.RTSPUrl,
		"-t", fmt.Sprintf("%.0f", segmentSize.Seconds()),
		"-c", "copy",
		"-f", "mp4",
		"-movflags", "faststart",
		"-y",
		tempPath,
	)

	output, err := cmd.CombinedOutput()
	if bm.ctx.Err() == context.Canceled {
		log.Printf("Buffer recording cancelled for %s, keeping partial file", bm.camera.Name)
		return fmt.Errorf("recording cancelled")
	}

	if segmentCtx.Err() == context.DeadlineExceeded {
		os.Remove(tempPath)
		return fmt.Errorf("segment timeout")
	}

	if err != nil {
		log.Printf("FFmpeg buffer error for %s: %v\nOutput: %s", bm.camera.Name, err, string(output))
		os.Remove(tempPath)
		return fmt.Errorf("ffmpeg error: %w", err)
	}

	err = os.Rename(tempPath, filePath)
	if err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to finalize buffer segment: %w", err)
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to stat buffer segment: %w", err)
	}

	if info.Size() < 1000 {
		os.Remove(filePath)
		return fmt.Errorf("buffer segment too small")
	}

	// Add to buffer
	bm.bufferMutex.Lock()
	bm.buffer = append(bm.buffer, BufferSegment{
		StartTime: now,
		FilePath:  filePath,
	})

	// Remove segments older than 30 seconds
	cutoff := now.Add(-bufferDuration)
	newBuffer := make([]BufferSegment, 0)
	for _, seg := range bm.buffer {
		if seg.StartTime.After(cutoff) {
			newBuffer = append(newBuffer, seg)
		} else {
			os.Remove(seg.FilePath)
		}
	}
	bm.buffer = newBuffer
	bm.bufferMutex.Unlock()

	return nil
}

// TriggerEvent is called when an external event is received
func (bm *BufferManager) TriggerEvent(event Event) {
	select {
	case bm.eventChan <- event:
		log.Printf("Event queued for %s: %s", bm.camera.Name, event.Label)
	default:
		log.Printf("Event channel full for %s, dropping event", bm.camera.Name)
	}
}

// handleEvent processes an event and saves the recording
func (bm *BufferManager) handleEvent(event Event) {
	bm.recordMutex.Lock()
	defer bm.recordMutex.Unlock()

	log.Printf("Processing event for %s: %s (duration: %ds)", bm.camera.Name, event.Label, event.Duration)

	// Get buffer segments (pre-recording)
	bm.bufferMutex.RLock()
	preRecordingFiles := make([]string, len(bm.buffer))
	for i, seg := range bm.buffer {
		preRecordingFiles[i] = seg.FilePath
	}
	bm.bufferMutex.RUnlock()

	// Record additional duration for the event
	eventDuration := time.Duration(event.Duration) * time.Second
	eventFilePath := bm.recordEventDuration(event, eventDuration)

	if eventFilePath == "" {
		log.Printf("Failed to record event duration for %s", bm.camera.Name)
		return
	}

	// Concatenate pre-recording + event recording
	finalFilePath := bm.concatenateEventRecording(event, preRecordingFiles, eventFilePath)

	if finalFilePath != "" {
		log.Printf("Event recording saved: %s (%s - %ds with 30s pre-recording)",
			finalFilePath, event.Label, event.Duration)
	}

	// Cleanup temporary event recording
	os.Remove(eventFilePath)
}

// recordEventDuration records the actual event duration
func (bm *BufferManager) recordEventDuration(event Event, duration time.Duration) string {
	filename := fmt.Sprintf("event_%s_%s_%s.mp4",
		time.Now().Format("20060102_150405"),
		sanitizeFilename(event.Label),
		sanitizeFilename(bm.camera.Name))

	tempDir := filepath.Join(bm.camera.RecordDir, "temp")
	os.MkdirAll(tempDir, 0755)

	filePath := filepath.Join(tempDir, filename)

	ctx, cancel := context.WithTimeout(context.Background(), duration+10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-rtsp_transport", "tcp",
		"-i", bm.camera.RTSPUrl,
		"-t", fmt.Sprintf("%.0f", duration.Seconds()),
		"-c", "copy",
		"-f", "mp4",
		"-movflags", "faststart",
		"-y",
		filePath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("FFmpeg event recording error for %s: %v\nOutput: %s", bm.camera.Name, err, string(output))
		os.Remove(filePath)
		return ""
	}

	info, err := os.Stat(filePath)
	if err != nil || info.Size() < 1000 {
		os.Remove(filePath)
		return ""
	}

	return filePath
}

// concatenateEventRecording merges pre-recording buffer with event recording
func (bm *BufferManager) concatenateEventRecording(event Event, preRecordingFiles []string, eventFile string) string {
	// Create final filename
	finalFilename := fmt.Sprintf("%s_%s_%s.mp4",
		event.StartTime.Format("20060102_150405"),
		sanitizeFilename(event.Label),
		sanitizeFilename(bm.camera.Name))

	finalPath := filepath.Join(bm.camera.RecordDir, finalFilename)

	// Create concat list file
	concatFile, err := os.CreateTemp("", "event-concat-*.txt")
	if err != nil {
		log.Printf("Failed to create concat file: %v", err)
		return ""
	}
	defer os.Remove(concatFile.Name())

	// Write pre-recording files
	for _, file := range preRecordingFiles {
		absPath, _ := filepath.Abs(file)
		fmt.Fprintf(concatFile, "file '%s'\n", absPath)
	}

	// Write event file
	absEventPath, _ := filepath.Abs(eventFile)
	fmt.Fprintf(concatFile, "file '%s'\n", absEventPath)
	concatFile.Close()

	// Run FFmpeg concatenation
	cmd := exec.Command("ffmpeg",
		"-f", "concat",
		"-safe", "0",
		"-i", concatFile.Name(),
		"-c", "copy",
		"-f", "mp4",
		"-movflags", "faststart",
		"-y",
		finalPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("FFmpeg concat error for event %s: %v\nOutput: %s", event.Label, err, string(output))
		return ""
	}

	info, err := os.Stat(finalPath)
	if err != nil {
		log.Printf("Failed to stat final event recording: %v", err)
		return ""
	}

	log.Printf("Event recording complete: %s (%.2f MB)", finalFilename, float64(info.Size())/(1024*1024))
	return finalPath
}

// cleanupBuffer removes all buffer segments
func (bm *BufferManager) cleanupBuffer() {
	bm.bufferMutex.Lock()
	defer bm.bufferMutex.Unlock()

	for _, seg := range bm.buffer {
		os.Remove(seg.FilePath)
	}
	bm.buffer = nil

	// Cleanup buffer directory
	bufferDir := filepath.Join(bm.camera.RecordDir, "buffer")
	os.RemoveAll(bufferDir)

	log.Printf("Cleaned up buffer for %s", bm.camera.Name)
}

// recordSegment records a single segment (used for continuous recording)
func recordSegment(ctx context.Context, cam *Camera, durationSeconds int) error {
	now := time.Now()
	filename := fmt.Sprintf("%s_%s.mp4",
		now.Format("20060102_150405"),
		sanitizeFilename(cam.Name))

	filePath := filepath.Join(cam.RecordDir, filename)
	tempPath := filePath + ".tmp"

	segmentCtx, cancel := context.WithTimeout(ctx, time.Duration(durationSeconds)*time.Second+5*time.Second)
	defer cancel()

	log.Printf("Starting segment for %s: %s", cam.Name, filename)

	cmd := exec.CommandContext(segmentCtx, "ffmpeg",
		"-rtsp_transport", "tcp",
		"-fflags", "+genpts+igndts+ignidx+discardcorrupt",
		"-i", cam.RTSPUrl,
		"-t", fmt.Sprintf("%d", durationSeconds),
		"-c", "copy",
		"-movflags", "faststart",
		"-f", "mp4",
		"-y",
		tempPath,
	)

	output, err := cmd.CombinedOutput()

	if ctx.Err() == context.Canceled {
		log.Printf("Recording cancelled for %s, keeping partial file", cam.Name)
		return fmt.Errorf("recording cancelled")
	}

	if segmentCtx.Err() == context.DeadlineExceeded {
		log.Printf("FFmpeg timeout for %s (this shouldn't happen): %s", cam.Name, string(output))
		os.Remove(tempPath)
		return fmt.Errorf("segment timeout")
	}

	if err != nil {
		log.Printf("FFmpeg error for %s: %v\nOutput: %s", cam.Name, err, string(output))
		os.Remove(tempPath)
		return fmt.Errorf("ffmpeg error: %w", err)
	}

	err = os.Rename(tempPath, filePath)
	if err != nil {
		log.Printf("Failed to rename temp file for %s: %v", cam.Name, err)
		os.Remove(tempPath)
		return fmt.Errorf("failed to finalize recording: %w", err)
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to stat recording: %w", err)
	}

	if info.Size() < 1000 {
		log.Printf("Warning: Recording file for %s is suspiciously small: %d bytes", cam.Name, info.Size())
		os.Remove(filePath)
		return fmt.Errorf("recording file too small")
	}

	log.Printf("Completed segment for %s: %s (%.2f MB)", cam.Name, filename, float64(info.Size())/(1024*1024))
	return nil
}

func sanitizeFilename(name string) string {
	safe := ""
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			safe += string(r)
		} else {
			safe += "_"
		}
	}
	return safe
}

// recoverPartialRecordings - On startup, validate and recover playable .tmp files
func recoverPartialRecordings() {
	log.Println("Checking for partial recordings to recover...")

	nvr.mu.RLock()
	cameras := make([]*Camera, 0, len(nvr.cameras))
	for _, cam := range nvr.cameras {
		cameras = append(cameras, cam)
	}
	nvr.mu.RUnlock()

	totalRecovered := 0
	totalDeleted := 0

	for _, cam := range cameras {
		tmpFiles, err := filepath.Glob(filepath.Join(cam.RecordDir, "*.tmp"))
		if err != nil {
			continue
		}

		for _, tmpFile := range tmpFiles {
			finalFile := tmpFile[:len(tmpFile)-4] // Remove ".tmp"

			// Check if file is valid
			info, err := os.Stat(tmpFile)
			if err != nil || info.Size() < 10240 { // Less than 10KB
				os.Remove(tmpFile)
				log.Printf("Deleted tiny/corrupted partial recording: %s", filepath.Base(tmpFile))
				totalDeleted++
				continue
			}

			// Validate the MP4 file with ffprobe
			cmd := exec.Command("ffprobe",
				"-v", "error",
				"-select_streams", "v:0",
				"-show_entries", "stream=codec_name",
				"-of", "default=noprint_wrappers=1:nokey=1",
				tmpFile,
			)

			output, err := cmd.CombinedOutput()
			if err != nil || len(output) == 0 {
				// File is corrupted or unreadable
				sizeMB := float64(info.Size()) / (1024 * 1024)
				os.Remove(tmpFile)
				log.Printf("Deleted corrupted partial recording: %s (%.2f MB) - failed ffprobe validation", filepath.Base(tmpFile), sizeMB)
				totalDeleted++
				continue
			}

			// File is valid, rename it
			if err := os.Rename(tmpFile, finalFile); err != nil {
				log.Printf("Failed to recover partial recording %s: %v", filepath.Base(tmpFile), err)
				continue
			}

			sizeMB := float64(info.Size()) / (1024 * 1024)
			log.Printf("✓ Recovered partial recording: %s (%.2f MB)", filepath.Base(finalFile), sizeMB)
			totalRecovered++
		}
	}

	if totalRecovered > 0 || totalDeleted > 0 {
		log.Printf("Recovery complete: %d recovered, %d deleted (corrupted)", totalRecovered, totalDeleted)
	} else {
		log.Println("No partial recordings found")
	}
}
