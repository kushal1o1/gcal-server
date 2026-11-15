// Package timing logs how long each stage of a request takes, so slow stages
// show up in the server logs (local `go run` and Vercel function logs alike).
package timing

import (
	"log"
	"time"
)

// Since logs one stage's duration. Usage:
//
//	t := time.Now()
//	... work ...
//	timing.Since("nlu", t)
func Since(stage string, start time.Time) {
	log.Printf("timing stage=%s ms=%d", stage, time.Since(start).Milliseconds())
}
