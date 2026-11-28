package main

import (
 "bytes"
 "context"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func TestPresentationProbeEnvelopeAllowsOnlyKnownEmptyArrays(t *testing.T) {
 for _,extra:=range []string{"",`"programs":[],`,`"stream_groups":[ ],`,`"programs":[],"stream_groups":[],`} {
  raw:=[]byte("{"+extra+eventPresentationFixture[1:])
  observation,err:=decodeEventPresentationProbeJSON(context.Background(),raw)
  if err!=nil || !observation.continuity.atLeastSeconds(2) {t.Fatal(extra,observation,err)}
  if extra!="" {if _,err:=decodeEventVideoPresentation(context.Background(),raw);err==nil {t.Fatal("canonical contract weakened")}}
 }
 for _,extra:=range []string{`"programs":null,`,`"programs":{},`,`"programs":[{}],`,`"stream_groups":[0],`,`"unknown":[],`,`"programs":[],"programs":[],`,`"progr\u0061ms":[],"programs":[],`,`"frames":[],`} {
  observation,err:=decodeEventPresentationProbeJSON(context.Background(),[]byte("{"+extra+eventPresentationFixture[1:]))
  if err==nil || observation!=(eventVideoPresentation{}) {t.Fatal(extra,observation,err)}
 }
 for _,suffix:=range []string{"{}","x","null"} {if _,err:=decodeEventPresentationProbeJSON(context.Background(),[]byte(eventPresentationFixture+suffix));err==nil {t.Fatal("tail accepted")}}
}
func TestPresentationProbeBufferRejectsRatherThanAcceptingPrefix(t *testing.T) {
 b:=&eventPresentationProbeBuffer{limit:3}
 if n,err:=b.Write([]byte("123"));n!=3 || err!=nil {t.Fatal(n,err)}
 if n,err:=b.Write([]byte("4"));n!=0 || !errors.Is(err,errEventVideoPresentation) || !b.refused || b.String()!="123" {t.Fatal(n,err)}
 if n,err:=b.Write(nil);n!=0 || err==nil {t.Fatal("refused buffer revived")}
}
func presentationProbeFile(t *testing.T) *os.File {
 t.Helper();file,err:=os.CreateTemp(t.TempDir(),"held-capture")
 if err!=nil {t.Fatal(err)}
 t.Cleanup(func(){file.Close()})
 if _,err:=file.WriteString("held descriptor evidence");err!=nil {t.Fatal(err)}
 return file
}
func TestPresentationProbeReadsHeldDescriptorAndRetainsBorrowedOwnership(t *testing.T) {
 file:=presentationProbeFile(t);path:=file.Name()
 if err:=os.Rename(path,path+".saved");err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(path,[]byte("foreign replacement"),0600);err!=nil {t.Fatal(err)}
 installEventSpanProbe(t,"[ \"$(cat /proc/self/fd/3)\" = 'held descriptor evidence' ] || exit 9\ncase \"$*\" in *frame=media_type,stream_index,pts,duration*) ;; *) exit 8;; esac\nprintf '%s' '"+eventPresentationFixture+"'\n")
 lease,err:=recordingMediaOwnership.acquire(context.Background());if err!=nil {t.Fatal(err)};defer lease.release()
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second);defer cancel()
 observation,err:=probeEventVideoPresentation(ctx,lease,file)
 if err!=nil || !observation.continuity.atLeastSeconds(2) || !lease.held() {t.Fatal(observation,err)}
 assertPublicationBytes(t,path,"foreign replacement");assertPublicationBytes(t,path+".saved","held descriptor evidence")
 if _,err:=file.Stat();err!=nil {t.Fatal("borrowed descriptor closed",err)}
}
func TestPresentationProbeRefusesFailuresWithoutClosingLeaseOrFile(t *testing.T) {
 cases:=map[string]string{
  "exit failure":"printf '%s' '"+eventPresentationFixture+"'\nexit 1\n",
  "diagnostic":"printf 'decode error' >&2\nprintf '%s' '"+eventPresentationFixture+"'\n",
  "malformed":"printf garbage\n",
  "gap":"printf '%s' '"+strings.Replace(eventPresentationFixture,`"pts":1000`,`"pts":1001`,1)+"'\n",
  "mutated descriptor":"printf x >> /proc/self/fd/3\nprintf '%s' '"+eventPresentationFixture+"'\n",
  "stdout overflow":"head -c 4194305 /dev/zero\n",
  "stderr overflow":"head -c 16385 /dev/zero >&2\nprintf '%s' '"+eventPresentationFixture+"'\n",
 }
 for name,body:=range cases {t.Run(name,func(t *testing.T){
  installEventSpanProbe(t,body);file:=presentationProbeFile(t)
  lease,err:=recordingMediaOwnership.acquire(context.Background());if err!=nil {t.Fatal(err)};defer lease.release()
  observation,err:=probeEventVideoPresentation(context.Background(),lease,file)
  if err==nil || observation!=(eventVideoPresentation{}) || !lease.held() {t.Fatal(observation,err)}
  if _,err:=file.Stat();err!=nil {t.Fatal(err)}
 })}
}
func TestPresentationProbeAdmissionCancellationAndDescriptorSnapshots(t *testing.T) {
 file:=presentationProbeFile(t)
 marker:=filepath.Join(t.TempDir(),"started");t.Setenv("PRESENTATION_STARTED",marker)
 installEventSpanProbe(t,"printf x > \"$PRESENTATION_STARTED\"\nprintf '%s' '"+eventPresentationFixture+"'\n")
 if _,err:=probeEventVideoPresentation(context.Background(),nil,file);err==nil {t.Fatal("nil lease admitted")}
 foreign,err:=newMediaMutationGate().acquire(context.Background());if err!=nil {t.Fatal(err)};defer foreign.release()
 if _,err:=probeEventVideoPresentation(context.Background(),foreign,file);err==nil {t.Fatal("foreign lease admitted")}
 lease,err:=recordingMediaOwnership.acquire(context.Background());if err!=nil {t.Fatal(err)};defer lease.release()
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if _,err:=probeEventVideoPresentation(ctx,lease,file);!errors.Is(err,context.Canceled) {t.Fatal(err)}
 if _,err:=probeEventVideoPresentation(context.Background(),lease,nil);err==nil {t.Fatal("nil file admitted")}
 empty,err:=os.CreateTemp(t.TempDir(),"empty");if err!=nil {t.Fatal(err)};defer empty.Close()
 if _,err:=probeEventVideoPresentation(context.Background(),lease,empty);err==nil {t.Fatal("empty file admitted")}
 closed:=presentationProbeFile(t);closed.Close()
 if _,err:=probeEventVideoPresentation(context.Background(),lease,closed);err==nil {t.Fatal("closed file admitted")}
 directory,err:=os.Open(t.TempDir());if err!=nil {t.Fatal(err)};defer directory.Close()
 if _,err:=probeEventVideoPresentation(context.Background(),lease,directory);err==nil {t.Fatal("directory admitted")}
 if _,err:=os.Stat(marker);!os.IsNotExist(err) {t.Fatal("admission failure started a child",err)}
 before,err:=eventPresentationDescriptorStat(context.Background(),lease,file);if err!=nil {t.Fatal(err)}
 for _,field:=range []string{"dev","ino","mode","size","links","mtime","ctime"} {
  after:=before
  switch field {case "dev":after.Dev++;case "ino":after.Ino++;case "mode":after.Mode++;case "size":after.Size++;case "links":after.Nlink++;case "mtime":after.Mtim.Nsec++;case "ctime":after.Ctim.Nsec++}
  if eventPresentationDescriptorUnchanged(before,after) {t.Fatal("changed descriptor metadata accepted",field)}
 }
 if !eventPresentationDescriptorUnchanged(before,before) {t.Fatal("same descriptor refused")}
}
func TestPresentationProbeFormatFixtures(t *testing.T) {
 // Actual FFprobe 8.0.1 output captured from generated MPEG-4 MP4, NOT RTSP.
 raw,err:=os.ReadFile("tests/fixtures/event-presentation-ffprobe-8.json");if err!=nil {t.Fatal(err)}
 observation,err:=decodeEventPresentationProbeJSON(context.Background(),raw)
 if err!=nil || observation.continuity.frames!=5 || !observation.continuity.atLeastSeconds(1) || observation.continuity.atLeastSeconds(2) {t.Fatal(observation,err)}
 // Older/unsupported frame schemas do NOT get a packet-duration fallback.
 unsupported:=bytes.ReplaceAll(raw,[]byte(`"duration"`),[]byte(`"pkt_duration"`))
 if _,err:=decodeEventPresentationProbeJSON(context.Background(),unsupported);err==nil {t.Fatal("unsupported frame schema silently accepted")}
}
