package ipc

// Handler is the interface for command handlers
// This file provides additional handler utilities and middleware

import (
	"log"
	"time"
)

// RequestLogger logs incoming requests (for debugging)
func RequestLogger(req *Request) {
	log.Printf("Request: cmd=%s token=%s...", req.Cmd, truncateToken(req.Token))
}

// ResponseLogger logs outgoing responses (for debugging)
func ResponseLogger(resp *Response, duration time.Duration) {
	if resp.Success {
		log.Printf("Response: success=true duration=%v", duration)
	} else {
		log.Printf("Response: success=false error=%s duration=%v", resp.Error, duration)
	}
}

func truncateToken(token string) string {
	if len(token) > 8 {
		return token[:8]
	}
	return token
}
