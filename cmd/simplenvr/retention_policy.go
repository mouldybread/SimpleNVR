package main

import (
 "errors"
 "time"
)

const maxCameraRetentionDays=365
var errCameraRetention=errors.New("retention days must be 1 to 365, or legacy 0 for the seven-day default")

func cameraRetentionDuration(days int) (time.Duration,error) {
 if days==0 {days=7}
 if days<1 || days>maxCameraRetentionDays {return 0,errCameraRetention}
 // Validation must precede conversion and multiplication.
 return time.Duration(days)*24*time.Hour,nil
}
