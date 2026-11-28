package main

import (
 "net/http"
 "net/http/httptest"
 "testing"
)

func TestRetiredRecordingListEndpointIsGone(t *testing.T) {
 w:=httptest.NewRecorder();listRecordingsHandler(w,httptest.NewRequest("GET","/api/recordings/list?camera_id=x",nil))
 if w.Code!=http.StatusGone {t.Fatal(w.Code)}
}
