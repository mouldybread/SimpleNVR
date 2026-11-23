package main

import (
 "encoding/json"
 "net/http"
 "testing"
)

type exportHTTPList struct {
 Jobs []exportHTTPView `json:"jobs"`
}

func listHTTPExports(t *testing.T,h http.Handler,cookie *http.Cookie) exportHTTPList {
 t.Helper();w:=exportHTTPRequest(h,cookie,"GET",exportListPath,"")
 if w.Code!=200 {t.Fatalf("list %d %s",w.Code,w.Body.String())}
 var list exportHTTPList;if err:=json.Unmarshal(w.Body.Bytes(),&list);err!=nil {t.Fatal(err)}
 if list.Jobs==nil {t.Fatal("jobs must be an empty array, not null")}
 return list
}

func TestExportHTTPListAndClear(t *testing.T) {
 a,_,h,cookie:=exportHTTPFixture(t)
 if got:=exportHTTPRequest(h,nil,"GET",exportListPath,"").Code;got!=401 {t.Fatalf("anonymous list: %d",got)}
 if got:=exportHTTPRequest(h,nil,"POST",exportClearPath,"{}").Code;got!=401 {t.Fatalf("anonymous clear: %d",got)}
 if got:=exportHTTPRequest(h,cookie,"POST",exportListPath,"{}").Code;got!=405 {t.Fatalf("POST list: %d",got)}
 if got:=exportHTTPRequest(h,cookie,"GET",exportClearPath,"").Code;got!=405 {t.Fatalf("GET clear: %d",got)}
 if got:=exportHTTPRequest(h,cookie,"GET",exportListPath+"?x=1","").Code;got!=400 {t.Fatalf("list with a query: %d",got)}
 if got:=len(listHTTPExports(t,h,cookie).Jobs);got!=0 {t.Fatalf("new session already has %d jobs",got)}
 first:=createHTTPExport(t,h,cookie)
 second:=exportHTTPRequest(h,cookie,"POST",exportCreatePath,exportHTTPCreateBody)
 if second.Code!=202 {t.Fatalf("second create %d %s",second.Code,second.Body.String())}
 var created exportHTTPView;if err:=json.Unmarshal(second.Body.Bytes(),&created);err!=nil {t.Fatal(err)}
 list:=listHTTPExports(t,h,cookie)
 if len(list.Jobs)!=2 || list.Jobs[0].ID!=first.ID || list.Jobs[1].ID!=created.ID {t.Fatalf("list wrong: %+v",list)}
 if list.Jobs[0].State!="ready" || list.Jobs[0].DownloadURL!=exportDownloadPath+"?id="+first.ID || list.Jobs[0].Bytes!=10 {t.Fatalf("ready job fields wrong: %+v",list.Jobs[0])}
 otherSession:=exportHTTPLogin(t,a,h)
 if got:=len(listHTTPExports(t,h,otherSession).Jobs);got!=0 {t.Fatalf("second session sees %d jobs",got)}
 w:=exportHTTPRequest(h,otherSession,"POST",exportClearPath,"{}")
 if w.Code!=200 || w.Body.String()!="{\"cleared\":0}\n" {t.Fatalf("clear from another session: %d %q",w.Code,w.Body.String())}
 if got:=len(listHTTPExports(t,h,cookie).Jobs);got!=2 {t.Fatalf("another session's clear removed jobs: %d",got)}
 if got:=exportHTTPRequest(h,cookie,"POST",exportClearPath,`{"all":true}`).Code;got!=400 {t.Fatalf("clear with unknown fields: %d",got)}
 w=exportHTTPRequest(h,cookie,"POST",exportClearPath,"{}")
 if w.Code!=200 || w.Body.String()!="{\"cleared\":2}\n" {t.Fatalf("clear: %d %q",w.Code,w.Body.String())}
 if got:=len(listHTTPExports(t,h,cookie).Jobs);got!=0 {t.Fatalf("%d jobs left after clear",got)}
 for _,path:=range []string{exportStatusPath+"?id="+first.ID,exportDownloadPath+"?id="+first.ID,exportStatusPath+"?id="+created.ID} {
  if got:=exportHTTPRequest(h,cookie,"GET",path,"").Code;got!=404 {t.Fatalf("%s after clear: %d",path,got)}
 }
 if got:=exportHTTPRequest(h,cookie,"POST",exportCancelPath,`{"id":"`+first.ID+`"}`).Code;got!=404 {t.Fatalf("cancel after clear: %d",got)}
}
