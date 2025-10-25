package main

import (
    "context"
    "fmt"
    "log"
    "net/url"
    "os"
    "path/filepath"
    "time"

    "github.com/bluenviron/gortsplib/v4"
    "github.com/bluenviron/gortsplib/v4/pkg/base"
    "github.com/bluenviron/gortsplib/v4/pkg/format"
    "github.com/bluenviron/mediacommon/pkg/codecs/h264"
    "github.com/deepch/vdk/av"
    "github.com/deepch/vdk/codec/h264parser"
    "github.com/deepch/vdk/format/mp4"
    "github.com/pion/rtp"
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
    u, err := url.Parse(cam.RTSPUrl)
    if err != nil {
        return fmt.Errorf("invalid RTSP URL: %w", err)
    }

    client := &gortsplib.Client{}

    err = client.Start(u.Scheme, u.Host)
    if err != nil {
        return fmt.Errorf("failed to connect: %w", err)
    }
    defer client.Close()

    baseURL := &base.URL{
        Scheme: u.Scheme,
        Host:   u.Host,
        Path:   u.Path,
    }

    session, _, err := client.Describe(baseURL)
    if err != nil {
        return fmt.Errorf("failed to describe: %w", err)
    }

    var h264Format *format.H264
    videoMedia := session.FindFormat(&h264Format)
    if videoMedia == nil {
        return fmt.Errorf("no H264 track found")
    }

    _, err = client.Setup(session.BaseURL, videoMedia, 0, 0)
    if err != nil {
        return fmt.Errorf("failed to setup: %w", err)
    }

    _, err = client.Play(nil)
    if err != nil {
        return fmt.Errorf("failed to play: %w", err)
    }

    now := time.Now()
    filename := fmt.Sprintf("%s_%s.mp4",
        now.Format("20060102_150405"),
        sanitizeFilename(cam.Name))
    filePath := filepath.Join(cam.RecordDir, filename)

    file, err := os.Create(filePath)
    if err != nil {
        return fmt.Errorf("failed to create file: %w", err)
    }
    defer file.Close()

    muxer := mp4.NewMuxer(file)
    var videoTrackID int8 = -1
    var sps, pps []byte
    var codecData h264parser.CodecData

    segmentDuration := 10 * time.Minute
    startTime := time.Now()
    firstPacketTime := time.Time{}

    // Use RTPDepacketizer
    rtpDec, err := h264Format.CreateDecoder()
    if err != nil {
        return fmt.Errorf("failed to create decoder: %w", err)
    }

    client.OnPacketRTP(videoMedia, h264Format, func(pkt *rtp.Packet) {
        if time.Since(startTime) >= segmentDuration {
            return
        }

        select {
        case <-ctx.Done():
            return
        default:
        }

        // Decode packet
        nalus, err := rtpDec.Decode(pkt)
        if err != nil {
            return
        }

        for _, nalu := range nalus {
            typ := h264.NALUType(nalu[0] & 0x1F)
            switch typ {
            case h264.NALUTypeSPS:
                sps = append([]byte{}, nalu...)
            case h264.NALUTypePPS:
                pps = append([]byte{}, nalu...)
            }
        }

        if videoTrackID == -1 && len(sps) > 0 && len(pps) > 0 {
            codecData, err = h264parser.NewCodecDataFromSPSAndPPS(sps, pps)
            if err != nil {
                log.Printf("Failed to parse codec data: %v", err)
                return
            }

            streams := []av.CodecData{codecData}
            err = muxer.WriteHeader(streams)
            if err != nil {
                log.Printf("Failed to write header: %v", err)
                return
            }
            videoTrackID = 0
            firstPacketTime = time.Now()
        }

        if videoTrackID != -1 {
            timestamp := time.Since(firstPacketTime)

            for _, nalu := range nalus {
                packet := av.Packet{
                    Idx:        videoTrackID,
                    Data:       nalu,
                    Time:       timestamp,
                    IsKeyFrame: h264.NALUType(nalu[0]&0x1F) == h264.NALUTypeIDR,
                }

                err = muxer.WritePacket(packet)
                if err != nil {
                    log.Printf("Write packet error: %v", err)
                    return
                }
            }
        }
    })

    select {
    case <-ctx.Done():
        if videoTrackID != -1 {
            muxer.WriteTrailer()
        }
        return nil
    case <-time.After(segmentDuration):
        if videoTrackID != -1 {
            muxer.WriteTrailer()
        }
        log.Printf("Completed 10-minute segment for %s: %s", cam.Name, filename)
        return nil
    }
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

func cleanupRoutine() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        nvr.mu.RLock()
        for _, cam := range nvr.cameras {
            retentionDays := cam.RetentionDays
            if retentionDays == 0 {
                retentionDays = 7
            }
            retention := time.Duration(retentionDays) * 24 * time.Hour
            cleanupOldRecordings(cam.RecordDir, retention)
        }
        nvr.mu.RUnlock()
    }
}

func cleanupOldRecordings(dir string, retention time.Duration) {
    files, err := filepath.Glob(filepath.Join(dir, "*.mp4"))
    if err != nil {
        return
    }

    cutoff := time.Now().Add(-retention)
    for _, file := range files {
        info, err := os.Stat(file)
        if err != nil {
            continue
        }
        if info.ModTime().Before(cutoff) {
            os.Remove(file)
            log.Printf("Cleaned up old recording: %s (older than %v)", file, retention)
        }
    }
}
