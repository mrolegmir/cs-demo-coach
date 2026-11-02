// Package config reads process configuration shared by API and direct workers.
package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

func Quotas() (store.Limits, error) {
	read := func(name string) (int, error) {
		v := os.Getenv(name)
		if v == "" {
			return 15, nil
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("%s must be a nonnegative integer", name)
		}
		return n, nil
	}
	u, err := read("CSDC_QUOTA_UPLOADS")
	if err != nil {
		return store.Limits{}, err
	}
	c, err := read("CSDC_QUOTA_COACHING")
	return store.Limits{Uploads: u, Coaching: c}, err
}
