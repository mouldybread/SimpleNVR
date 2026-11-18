package main

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "os"
 "regexp"
 "strings"
 "time"
 "unicode/utf8"

 "golang.org/x/sys/unix"
)

var errRecoveryInspection=errors.New("recovery inspection incomplete")
var recoveryStagingName=regexp.MustCompile(`^event-[A-Za-z0-9_-]{1,80}$`)
var recoveryOutputName=regexp.MustCompile(`^[A-Za-z0-9_]+\.mp4$`)

// Detect duplicate keys before decoding structs; encoding/json alone accepts
// duplicate keys and case-insensitive struct field matches.
func recoveryJSONValue(dec *json.Decoder,depth int,budget *int) error {
 *budget = *budget - 1
 if depth>6 || *budget<0 {return errRecoveryInspection}
 token,err:=dec.Token();if err!=nil {return err}
 delim,compound:=token.(json.Delim);if !compound {return nil}
 switch delim {
 case '{':
  keys:=make(map[string]bool)
  for dec.More() {
   *budget = *budget - 1;if *budget<0 {return errRecoveryInspection}
   keyToken,err:=dec.Token();if err!=nil {return err}
   key,ok:=keyToken.(string);if !ok || keys[key] {return errRecoveryInspection};keys[key]=true
   if err:=recoveryJSONValue(dec,depth+1,budget);err!=nil {return err}
  }
  end,err:=dec.Token();if err!=nil || end!=json.Delim('}') {return errRecoveryInspection}
 case '[':
  for dec.More() {if err:=recoveryJSONValue(dec,depth+1,budget);err!=nil {return err}}
  end,err:=dec.Token();if err!=nil || end!=json.Delim(']') {return errRecoveryInspection}
 default:return errRecoveryInspection
 }
 return nil
}

func exactRecoveryKeys(data []byte,required,optional []string) error {
 var fields map[string]json.RawMessage
 if err:=json.Unmarshal(data,&fields);err!=nil || fields==nil {return errRecoveryInspection}
 allowed:=make(map[string]bool)
 for _,key:=range required {allowed[key]=true;if value,ok:=fields[key];!ok || bytes.Equal(bytes.TrimSpace(value),[]byte("null")) {return errRecoveryInspection}}
 for _,key:=range optional {allowed[key]=true}
 for key,value:=range fields {if !allowed[key] || bytes.Equal(bytes.TrimSpace(value),[]byte("null")) {return errRecoveryInspection}}
 return nil
}

func parseRecoveryManifest(data []byte,cameraID string) (*eventRecoveryManifest,error) {
 if len(data)==0 || len(data)>maxRecoveryManifestBytes || !utf8.Valid(data) {return nil,errRecoveryInspection}
 dec:=json.NewDecoder(bytes.NewReader(data));budget:=4096
 if err:=recoveryJSONValue(dec,0,&budget);err!=nil {return nil,errRecoveryInspection}
 if _,err:=dec.Token();err!=io.EOF {return nil,errRecoveryInspection}
 required:=[]string{"version","revision","camera_id","event_id","requested_start","requested_end","duration_seconds","created_at","observed_at","state","prebuffer_inputs","pinned_count","capture_name","partial"}
 if err:=exactRecoveryKeys(data,required,[]string{"failure_code","output_temporary","output_published","coverage_assessment"});err!=nil {return nil,err}
 var raw struct {Inputs []json.RawMessage `json:"prebuffer_inputs"`; Coverage json.RawMessage `json:"coverage_assessment"`}
 if err:=json.Unmarshal(data,&raw);err!=nil || len(raw.Inputs)>maxRecoveryInputs {return nil,errRecoveryInspection}
 for _,input:=range raw.Inputs {if err:=exactRecoveryKeys(input,[]string{"name","metadata_start","metadata_end"},nil);err!=nil {return nil,err}}
 var m eventRecoveryManifest
 dec=json.NewDecoder(bytes.NewReader(data));dec.DisallowUnknownFields()
 if err:=dec.Decode(&m);err!=nil {return nil,errRecoveryInspection}
 if m.Version!=1 || m.Revision<1 || m.Revision>1000000 || m.CameraID!=cameraID || m.CameraID=="" || len(m.CameraID)>256 || m.EventID=="" || len(m.EventID)>256 || strings.ContainsAny(m.EventID,"\x00\r\n") || m.DurationSeconds<1 || m.DurationSeconds>maxEventSeconds || m.Inputs==nil || m.PinnedCount<0 || m.PinnedCount>len(m.Inputs) || m.CaptureName!="event.mp4" {return nil,errRecoveryInspection}
 if m.RequestedStart.IsZero() || m.RequestedEnd.IsZero() || m.CreatedAt.IsZero() || m.ObservedAt.IsZero() || m.RequestedEnd.Sub(m.RequestedStart)!=time.Duration(m.DurationSeconds)*time.Second {return nil,errRecoveryInspection}
 for i,input:=range m.Inputs {if input.Name!=fmt.Sprintf("pre_%03d.mp4",i) || input.Start.IsZero() || input.End.IsZero() || !input.End.After(input.Start) {return nil,errRecoveryInspection}}
 failure:=""
 switch m.State {
 case "pinning":if m.PinnedCount!=0 {return nil,errRecoveryInspection}
 case "pin_failed":failure="link_failed";if m.PinnedCount>=len(m.Inputs) {return nil,errRecoveryInspection}
 case "capture_failed":failure="process_failed_or_canceled"
 case "publication_failed":failure="validation_or_publication_failed"
 case "pinned","capturing","captured","assembling","publishing","published":
 default:return nil,errRecoveryInspection
 }
 if m.FailureCode!=failure {return nil,errRecoveryInspection}
 if m.State!="pinning" && m.State!="pin_failed" && m.PinnedCount!=len(m.Inputs) {return nil,errRecoveryInspection}
 publication:=m.State=="publishing" || m.State=="published" || m.State=="publication_failed"
 if publication {
  if len(m.OutputTemporary)>240 || !strings.HasSuffix(m.OutputTemporary,".tmp") {return nil,errRecoveryInspection}
  normal:=strings.TrimSuffix(m.OutputTemporary,".tmp")
  if !recoveryOutputName.MatchString(normal) || !recoveryOutputName.MatchString(m.OutputPublished) {return nil,errRecoveryInspection}
  expected:=normal;if m.Partial {expected=partialRecordingName(normal)}
  if m.OutputPublished!=expected {return nil,errRecoveryInspection}
 } else if m.OutputTemporary!="" || m.OutputPublished!="" || m.Partial {return nil,errRecoveryInspection}
 if len(raw.Coverage)!=0 {if err:=validateEventCoverageJSON(raw.Coverage,&m);err!=nil {return nil,err}}
 return &m,nil
}

func sameRecoveryObject(a,b *unix.Stat_t) bool {return a.Dev==b.Dev && a.Ino==b.Ino && a.Mode==b.Mode}
func sameRecoverySnapshot(a,b *unix.Stat_t) bool {return sameRecoveryObject(a,b) && a.Size==b.Size && a.Mtim==b.Mtim && a.Ctim==b.Ctim}

func openRecoveryChild(parent *os.File,name string) (*os.File,unix.Stat_t,error) {
 var before,opened unix.Stat_t
 if err:=unix.Fstatat(int(parent.Fd()),name,&before,unix.AT_SYMLINK_NOFOLLOW);err!=nil || before.Mode&unix.S_IFMT!=unix.S_IFDIR {return nil,opened,errRecoveryInspection}
 fd,err:=unix.Openat(int(parent.Fd()),name,unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC,0);if err!=nil {return nil,opened,errRecoveryInspection}
 file:=os.NewFile(uintptr(fd),"recovery-directory")
 if err:=unix.Fstat(fd,&opened);err!=nil || !sameRecoveryObject(&before,&opened) {file.Close();return nil,opened,errRecoveryInspection}
 return file,opened,nil
}

func readRecoverySnapshot(ctx context.Context,dir *os.File) ([]byte,bool,error) {
 if ctx.Err()!=nil {return nil,false,errRecoveryInspection}
 var before,opened,after,named unix.Stat_t
 if err:=unix.Fstatat(int(dir.Fd()),"recovery.json",&before,unix.AT_SYMLINK_NOFOLLOW);err!=nil {
  if errors.Is(err,unix.ENOENT) {return nil,true,nil};return nil,false,errRecoveryInspection
 }
 if before.Mode&unix.S_IFMT!=unix.S_IFREG || before.Size<1 || before.Size>maxRecoveryManifestBytes {return nil,false,errRecoveryInspection}
 fd,err:=unix.Openat(int(dir.Fd()),"recovery.json",unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC,0);if err!=nil {return nil,false,errRecoveryInspection}
 identity:=os.NewFile(uintptr(fd),"recovery-manifest-identity")
 if err:=unix.Fstat(fd,&opened);err!=nil || !sameRecoverySnapshot(&before,&opened) {identity.Close();return nil,false,errRecoveryInspection}
 // O_PATH never activates a swapped device/FIFO. Reopen the verified regular
 // inode through our own live descriptor, not the mutable directory name.
 if ctx.Err()!=nil {identity.Close();return nil,false,errRecoveryInspection}
 readFD,err:=unix.Open(fmt.Sprintf("/proc/self/fd/%d",fd),unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC,0)
 if err!=nil {identity.Close();return nil,false,errRecoveryInspection}
 file:=os.NewFile(uintptr(readFD),"recovery-manifest")
 var reopened unix.Stat_t
 if err:=unix.Fstat(readFD,&reopened);err!=nil || !sameRecoverySnapshot(&opened,&reopened) {file.Close();identity.Close();return nil,false,errRecoveryInspection}
 data,readErr:=io.ReadAll(io.LimitReader(file,maxRecoveryManifestBytes+1))
 statErr:=unix.Fstat(readFD,&after);closeErr:=errors.Join(file.Close(),identity.Close())
 namedErr:=unix.Fstatat(int(dir.Fd()),"recovery.json",&named,unix.AT_SYMLINK_NOFOLLOW)
 if ctx.Err()!=nil || readErr!=nil || statErr!=nil || closeErr!=nil || namedErr!=nil || !sameRecoverySnapshot(&opened,&after) || !sameRecoverySnapshot(&after,&named) || len(data)>maxRecoveryManifestBytes || int64(len(data))!=after.Size {return nil,false,errRecoveryInspection}
 return data,false,nil
}

type recoveryArtifactObservation struct {
 Role string `json:"role"`
 Index *int `json:"index,omitempty"`
 Present bool `json:"present"`
 LogicalBytes *int64 `json:"logical_bytes,omitempty"`
}

type eventRecoveryInspection struct {
 CameraID string `json:"camera_id"`
 StagingID string `json:"staging_id"`
 ObservedAt time.Time `json:"observed_at"`
 Complete bool `json:"inspection_complete"`
 Classification string `json:"classification"`
 EventID string `json:"event_id,omitempty"`
 State string `json:"checkpoint_state,omitempty"`
 Revision int `json:"checkpoint_revision,omitempty"`
 Partial *bool `json:"checkpoint_partial,omitempty"`
 RequestedStart *time.Time `json:"requested_start,omitempty"`
 RequestedEnd *time.Time `json:"requested_end,omitempty"`
 DurationSeconds *int `json:"duration_seconds,omitempty"`
 Artifacts []recoveryArtifactObservation `json:"artifacts,omitempty"`
 Coverage *eventCoverageAssessment `json:"stored_coverage_assessment,omitempty"`
 Errors []string `json:"inspection_errors"`
}

func observeRecoveryArtifact(ctx context.Context,parent *os.File,name,role string,index *int) (recoveryArtifactObservation,error) {
 item:=recoveryArtifactObservation{Role:role,Index:index}
 if ctx.Err()!=nil {return item,errRecoveryInspection}
 var stat unix.Stat_t
 if err:=unix.Fstatat(int(parent.Fd()),name,&stat,unix.AT_SYMLINK_NOFOLLOW);err!=nil {
  if errors.Is(err,unix.ENOENT) {return item,nil};return item,errRecoveryInspection
 }
 if stat.Mode&unix.S_IFMT!=unix.S_IFREG || stat.Size<0 {return item,errRecoveryInspection}
 item.Present=true;size:=stat.Size;item.LogicalBytes=&size;return item,nil
}

func inspectEventRecovery(ctx context.Context,rootPath,cameraID,stagingID string) (report eventRecoveryInspection) {
 report=eventRecoveryInspection{CameraID:cameraID,StagingID:stagingID,Classification:"inspection_incomplete",Errors:[]string{}}
 defer func(){report.ObservedAt=time.Now().UTC();if !report.Complete && len(report.Errors)==0 {report.Errors=append(report.Errors,"inspection_incomplete")}}()
 if !recoveryStagingName.MatchString(stagingID) || ctx.Err()!=nil {return report}
 fd,err:=unix.Open(rootPath,unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC,0);if err!=nil {return report}
 root:=os.NewFile(uintptr(fd),"recovery-camera");defer func(){if err:=root.Close();err!=nil {report.Complete=false;report.Errors=append(report.Errors,"descriptor_close_failed")}}()
 temp,tempStat,err:=openRecoveryChild(root,"temp");if err!=nil {return report};defer func(){if err:=temp.Close();err!=nil {report.Complete=false;report.Errors=append(report.Errors,"descriptor_close_failed")}}()
 staging,stagingStat,err:=openRecoveryChild(temp,stagingID);if err!=nil {return report};defer func(){if err:=staging.Close();err!=nil {report.Complete=false;report.Errors=append(report.Errors,"descriptor_close_failed")}}()
 data,missing,err:=readRecoverySnapshot(ctx,staging)
 if err!=nil {report.Errors=append(report.Errors,"manifest_unavailable_or_changed");return report}
 if missing {report.Classification="legacy_or_missing_manifest"} else {
  m,err:=parseRecoveryManifest(data,cameraID)
  if err!=nil {report.Classification="invalid_or_unsupported_manifest";report.Errors=append(report.Errors,"manifest_rejected");return report}
  report.EventID=m.EventID;report.State=m.State;report.Revision=m.Revision
  report.RequestedStart=&m.RequestedStart;report.RequestedEnd=&m.RequestedEnd;report.DurationSeconds=&m.DurationSeconds
  partial:=m.Partial;report.Partial=&partial
  report.Coverage=m.Coverage // stored diagnostic, not independently verified media coverage
  switch m.State {
  case "pin_failed","capture_failed","publication_failed":report.Classification="observed_processing_failure"
  case "publishing":report.Classification="publication_outcome_ambiguous"
  case "published":report.Classification="published_checkpoint_unverified"
  default:report.Classification="inflight_or_interrupted"
  }
  type target struct {parent *os.File;name,role string;index *int}
  targets:=[]target{{parent:staging,name:"event.mp4",role:"capture"},{parent:staging,name:"concat.txt",role:"concat_list"}}
  for i,input:=range m.Inputs {index:=i;targets=append(targets,target{parent:staging,name:input.Name,role:"prebuffer",index:&index})}
  if m.OutputTemporary!="" {targets=append(targets,target{parent:root,name:m.OutputTemporary,role:"temporary_output"},target{parent:root,name:m.OutputPublished,role:"published_output"})}
  for _,target:=range targets {
   observation,err:=observeRecoveryArtifact(ctx,target.parent,target.name,target.role,target.index)
   if err!=nil {report.Errors=append(report.Errors,"artifact_observation_incomplete");return report}
   report.Artifacts=append(report.Artifacts,observation)
  }
 }
 // Recheck the manifest and directory names before claiming completed inspection.
 again,missingAgain,err:=readRecoverySnapshot(ctx,staging)
 var tempNow,stagingNow unix.Stat_t
 e1:=unix.Fstatat(int(root.Fd()),"temp",&tempNow,unix.AT_SYMLINK_NOFOLLOW)
 e2:=unix.Fstatat(int(temp.Fd()),stagingID,&stagingNow,unix.AT_SYMLINK_NOFOLLOW)
 if err!=nil || missingAgain!=missing || !bytes.Equal(data,again) || e1!=nil || e2!=nil || !sameRecoveryObject(&tempStat,&tempNow) || !sameRecoveryObject(&stagingStat,&stagingNow) || ctx.Err()!=nil {report.Errors=append(report.Errors,"inspection_changed_or_canceled");return report}
 report.Complete=true;return report
}
