package main

import (
 "path/filepath"
 "testing"
)

func TestEventPublishedPathRequiresExactStampedDayAndCamera(t *testing.T) {
 camera:=filepath.Join(t.TempDir(),"camera")
 name:="20261110_235500_E20261111_000500_event_id_x.mp4"
 cases:=[]struct{name,path string;want bool}{
  {"UTC start day",filepath.Join(camera,"20261110",name),true},
  {"end day is not start day",filepath.Join(camera,"20261111",name),false},
  {"stamped file in root",filepath.Join(camera,name),false},
  {"nested day",filepath.Join(camera,"extra","20261110",name),false},
  {"other camera",filepath.Join(filepath.Dir(camera),"other","20261110",name),false},
  {"staging",filepath.Join(camera,"temp",name),false},
  {"buffer",filepath.Join(camera,"buffer",name),false},
  {"invalid day",filepath.Join(camera,"20261310",name),false},
  {"relative path",filepath.Join("camera","20261110",name),false},
  {"unclean path",camera+"/extra/../20261110/"+name,false},
  {"existing unstamped contract",filepath.Join(camera,"output.mp4"),true},
  {"unstamped nested file",filepath.Join(camera,"20261110","output.mp4"),false},
 }
 for _,c:=range cases {t.Run(c.name,func(t *testing.T){if got:=eventPublishedPathInCamera(camera,c.path);got!=c.want {t.Fatal(c.path,got,c.want)}})}
 if eventPublishedPathInCamera("relative",filepath.Join(camera,"20261110",name)) {t.Fatal("relative camera accepted")}
}
