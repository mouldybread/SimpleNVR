package main

import (
 "context"
 "encoding/json"
 "os/exec"
 "testing"
 "time"
)

type copyTestVideoPacket struct {
 DTS string `json:"dts_time"`
 Hash string `json:"data_hash"`
}

func copyTestVideoPackets(t *testing.T, ctx context.Context, path string) []copyTestVideoPacket {
 t.Helper()
 raw, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0", "-show_packets", "-show_data_hash", "sha256", "-show_entries", "packet=dts_time,data_hash", "-of", "json", path).Output()
 if err != nil { t.Fatal(err) }
 var data struct { Packets []copyTestVideoPacket `json:"packets"` }
 if err := json.Unmarshal(raw, &data); err != nil { t.Fatal(err) }
 if len(data.Packets) == 0 { t.Fatal("no video packets") }
 for _, packet := range data.Packets {
  if packet.Hash == "" { t.Fatal("missing packet hash") }
 }
 return data.Packets
}

// These comparisons run only in tests. Expected packets come from the source
// decode-time interval, not a second export produced by the same renderer.
func TestCopyBFrameVideoPacketPreservation(t *testing.T) {
 requireFFmpeg(t)
 for _, tc := range []struct { name string; open bool; wantPackets int }{
  {"closed GOP subsection", false, 60},
  {"open GOP expands to whole file", true, 120},
 } {
  t.Run(tc.name, func(t *testing.T) {
   ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
   defer cancel()
   source := makePreparedBFrameSource(t, ctx, tc.open)
   before := preparedMediaDigest(t, source)
   requested, pins := preparedMediaTestPlan(t, ctx, source, 1, 1500*time.Millisecond, 3500*time.Millisecond)
   actual, indices, err := planKeyframeCopy(ctx, pins, requested)
   if err != nil { t.Fatal(err) }
   wantStart, wantEnd := requested.MediaStart.Add(time.Second), requested.MediaStart.Add(4*time.Second)
   if tc.open { wantStart, wantEnd = requested.MediaStart, requested.MediaEnd }
   if !actual.Start.Equal(wantStart) || !actual.End.Equal(wantEnd) { t.Fatal("unexpected planned range") }
   if len(indices) != 1 { t.Fatal("expected one source") }
   index := indices[0]
   part := actual.Parts[0]
   startDTS := 0.0
   found := false
   for i, key := range index.keys {
    if key == part.In { startDTS = index.origin + index.ends[i].Seconds(); found = true; break }
   }
   if !found { t.Fatal("start is not a verified IDR") }
   endDTS := copyOutpoint(index, part)
   var expected []string
   for _, packet := range copyTestVideoPackets(t, ctx, source) {
    dts, err := preparedMediaSeconds(packet.DTS)
    if err != nil { t.Fatal(err) }
    if dts >= startDTS-0.0000001 && dts < endDTS-0.0000001 { expected = append(expected, packet.Hash) }
   }
   if len(expected) != tc.wantPackets { t.Fatalf("source interval: got %d packets, want %d", len(expected), tc.wantPackets) }
   output := preparedMediaOutput(t)
   if err := renderKeyframeCopy(ctx, pins, actual, indices, output, 16<<20); err != nil { t.Fatal(err) }
   decodePreparedMedia(t, ctx, output.Name())
   got := copyTestVideoPackets(t, ctx, output.Name())
   if len(got) != len(expected) { t.Fatalf("output has %d packets, want %d", len(got), len(expected)) }
   for i, packet := range got {
    if packet.Hash != expected[i] { t.Fatalf("compressed video packet %d changed or is out of order", i) }
   }
   if preparedMediaDigest(t, source) != before { t.Fatal("source changed") }
  })
 }
}
