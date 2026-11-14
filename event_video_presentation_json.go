package main

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "io"
 "strconv"
 "strings"
)

var errEventVideoPresentation = errors.New("strict event video presentation JSON unavailable")

// An observation-size ceiling, not permission to accept a prefix.
const maxEventVideoPresentationBytes = 4 << 20

// Parsed arithmetic only. This is NOT a descriptor-bound observation receipt.
// No production caller, process success witness, lease or media identity is added.
type eventVideoPresentation struct {
 streamIndex int64
 continuity eventVideoContinuity
}

// Exact, case-sensitive object keys; duplicate and unknown fields refuse instead
// of encoding/json's last-wins behavior. All named fields must occur exactly once.
func eventPresentationObject(ctx context.Context, d *json.Decoder, names ...string) (map[string]json.RawMessage, error) {
 if err := ctx.Err(); err != nil { return nil, err }
 token, err := d.Token()
 if err != nil || token != json.Delim('{') { return nil, errEventVideoPresentation }
 fields := make(map[string]json.RawMessage, len(names))
 for d.More() {
  if err := ctx.Err(); err != nil { return nil, err }
  token, err := d.Token()
  if err != nil { return nil, errEventVideoPresentation }
  key, ok := token.(string)
  if !ok { return nil, errEventVideoPresentation }
  allowed := false
  for _, name := range names { if name == key { allowed = true; break } }
  if _, exists := fields[key]; !allowed || exists { return nil, errEventVideoPresentation }
  var value json.RawMessage
  if err := d.Decode(&value); err != nil { return nil, errEventVideoPresentation }
  fields[key] = value
 }
 token, err = d.Token()
 if err != nil || token != json.Delim('}') || len(fields) != len(names) { return nil, errEventVideoPresentation }
 if err := ctx.Err(); err != nil { return nil, err }
 return fields, nil
}

func eventPresentationEOF(d *json.Decoder) bool {
 var extra json.RawMessage
 return d.Decode(&extra) == io.EOF
}

// Only JSON integer tokens fit this wire contract. Strings, null, booleans,
// decimal/exponent notation, whitespace inside a number and overflow all refuse.
func eventPresentationInteger(raw json.RawMessage) (int64, bool) {
 text := string(bytes.TrimSpace(raw))
 if text == "" { return 0, false }
 value, err := strconv.ParseInt(text, 10, 64)
 return value, err == nil && json.Valid([]byte(text))
}

func eventPresentationString(raw json.RawMessage, expected string) bool {
 var value string
 return json.Unmarshal(raw, &value) == nil && value == expected
}

func eventPresentationTimeBase(raw json.RawMessage) (int64, int64, bool) {
 var value string
 if json.Unmarshal(raw, &value) != nil { return 0, 0, false }
 parts := strings.Split(value, "/")
 if len(parts) != 2 { return 0, 0, false }
 for _, part := range parts {
  if part == "" { return 0, 0, false }
  for _, char := range part { if char < '0' || char > '9' { return 0, 0, false } }
 }
 numerator, err1 := strconv.ParseInt(parts[0], 10, 64)
 denominator, err2 := strconv.ParseInt(parts[1], 10, 64)
 return numerator, denominator, err1 == nil && err2 == nil && numerator > 0 && denominator > 0
}

// Explicit wire contract: one video stream (index, codec_type, time_base), and
// presentation-ordered frames (media_type, stream_index, pts, duration).
// No aliases, best-effort timestamps, packet-order sorting, format duration or
// inferred final-frame duration. A future reader must establish completeness and
// successful process/EOF provenance; valid JSON alone cannot establish either.
func decodeEventVideoPresentation(ctx context.Context, raw []byte) (eventVideoPresentation, error) {
 var zero eventVideoPresentation
 if err := ctx.Err(); err != nil { return zero, err }
 if len(raw) == 0 || len(raw) > maxEventVideoPresentationBytes { return zero, errEventVideoPresentation }
 d := json.NewDecoder(bytes.NewReader(raw))
 root, err := eventPresentationObject(ctx, d, "streams", "frames")
 if err != nil { return zero, err }
 if !eventPresentationEOF(d) { return zero, errEventVideoPresentation }
 streams := json.NewDecoder(bytes.NewReader(root["streams"]))
 token, err := streams.Token()
 if err != nil || token != json.Delim('[') || !streams.More() { return zero, errEventVideoPresentation }
 stream, err := eventPresentationObject(ctx, streams, "index", "codec_type", "time_base")
 if err != nil { return zero, err }
 if streams.More() { return zero, errEventVideoPresentation }
 token, err = streams.Token()
 if err != nil || token != json.Delim(']') || !eventPresentationEOF(streams) { return zero, errEventVideoPresentation }
 index, ok := eventPresentationInteger(stream["index"])
 if !ok || index < 0 || !eventPresentationString(stream["codec_type"], "video") { return zero, errEventVideoPresentation }
 numerator, denominator, ok := eventPresentationTimeBase(stream["time_base"])
 if !ok { return zero, errEventVideoPresentation }
 frames := json.NewDecoder(bytes.NewReader(root["frames"]))
 token, err = frames.Token()
 if err != nil || token != json.Delim('[') { return zero, errEventVideoPresentation }
 var intervals []eventVideoFrameInterval
 for frames.More() {
  if err := ctx.Err(); err != nil { return zero, err }
  if len(intervals) >= maxEventVideoContinuityFrames { return zero, errEventVideoPresentation }
  frame, err := eventPresentationObject(ctx, frames, "media_type", "stream_index", "pts", "duration")
  if err != nil { return zero, err }
  frameIndex, indexOK := eventPresentationInteger(frame["stream_index"])
  pts, ptsOK := eventPresentationInteger(frame["pts"])
  duration, durationOK := eventPresentationInteger(frame["duration"])
  if !indexOK || frameIndex != index || !ptsOK || !durationOK || duration <= 0 || !eventPresentationString(frame["media_type"], "video") { return zero, errEventVideoPresentation }
  intervals = append(intervals, eventVideoFrameInterval{pts:pts, duration:duration})
 }
 token, err = frames.Token()
 if err != nil || token != json.Delim(']') || !eventPresentationEOF(frames) { return zero, errEventVideoPresentation }
 continuity, err := validateEventVideoContinuity(ctx, intervals, numerator, denominator)
 if err != nil { return zero, err }
 if err := ctx.Err(); err != nil { return zero, err }
 return eventVideoPresentation{streamIndex:index, continuity:continuity}, nil
}
