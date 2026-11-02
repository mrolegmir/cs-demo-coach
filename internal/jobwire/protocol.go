// Package jobwire contains the v2 worker protocol, independent of HTTP handlers.
package jobwire

import (
	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

type ClaimRequest struct {
	StaleSeconds int `json:"stale_seconds"`
}
type DetectionRequest struct {
	Attempt int           `json:"attempt"`
	Match   *match.Match  `json:"match"`
	Report  detect.Report `json:"report"`
}
type AdviceRequest struct {
	Attempt int           `json:"attempt"`
	Advice  *coach.Advice `json:"advice"`
	Coach   string        `json:"coach"`
}
type FailureRequest struct {
	Attempt      int    `json:"attempt"`
	Retry        bool   `json:"retry"`
	AfterSeconds int    `json:"after_seconds"`
	Reason       string `json:"reason"`
}
