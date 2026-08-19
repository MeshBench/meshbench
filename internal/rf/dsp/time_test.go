package dsp

import "time"

func timeNow() int64 { return time.Now().UnixNano() }
