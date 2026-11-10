package main

import (
 "errors"
 "os"
 "strconv"
 "syscall"
)

var (
 errStorageReserve = errors.New("insufficient free recording space for configured reserve")
 errStorageUnavailable = errors.New("unable to verify recording storage capacity")
 errStoragePolicy = errors.New("invalid SIMPLENVR_MIN_FREE_BYTES configuration")
)

// A missing value or zero explicitly leaves admission checking disabled.
func recordingStorageReserve() (uint64,error) {
 raw:=os.Getenv("SIMPLENVR_MIN_FREE_BYTES")
 if raw=="" {return 0,nil}
 reserve,err:=strconv.ParseUint(raw,10,64)
 if err!=nil {return 0,errStoragePolicy}
 return reserve,nil
}

// Compare block counts using ceiling division, avoiding byte multiplication
// overflow and using blocks available to the process rather than total free.
func storageReserveFits(availableBlocks uint64,blockSize int64,reserve uint64) (bool,error) {
 if reserve==0 {return true,nil}
 if blockSize<=0 {return false,errStorageUnavailable}
 size:=uint64(blockSize)
 required:=reserve/size
 if reserve%size!=0 {required++}
 return availableBlocks>=required,nil
}

func checkRecordingStorage(dir string) error {
 reserve,err:=recordingStorageReserve()
 if err!=nil || reserve==0 {return err}
 var stat syscall.Statfs_t
 // Inspect the actual existing camera directory: falling back to a parent
 // can accidentally verify a different mount. Never create or delete here.
 if err:=syscall.Statfs(dir,&stat);err!=nil {return errStorageUnavailable}
 fits,err:=storageReserveFits(uint64(stat.Bavail),int64(stat.Bsize),reserve)
 if err!=nil {return err}
 if !fits {return errStorageReserve}
 return nil
}
