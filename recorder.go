package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func recordCamera(ctx context.Context, cam *Camera) {
	log.Printf("Starting recording for %s (retention: %d days)", cam.Name, cam.RetentionDays)
	for {
		select {
		case <-ctx.Done():
			log.Printf("Stopped recording for %s", cam.Name)
			return
		default:
			err := recordSegment(ctx, cam)
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

func recordSegment(ctx context.Context, cam *Camera) error {
	now := time.Now()
	filename := fmt.Sprintf("%s_%s.mp4",
		now.Format("20060102_150405"),
		sanitizeFilename(cam.Name))
	filePath := filepath.Join(cam.RecordDir, filename)

	tempPath := filePath + ".tmp"

	segmentCtx, cancel := context.WithTimeout(ctx, 10*time.Minute+5*time.Second)
	defer cancel()

	log.Printf("Starting segment for %s: %s", cam.Name, filename)

	cmd := exec.CommandContext(segmentCtx, "ffmpeg",
		"-rtsp_transport", "tcp",
		"-i", cam.RTSPUrl,
		"-t", "600",
		"-c", "copy",
		"-movflags", "+faststart",
		"-f", "mp4",
		"-y",
		tempPath,
	)

	output, err := cmd.CombinedOutput()

	if ctx.Err() == context.Canceled {
		os.Remove(tempPath)
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
