package match

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// LoadFile reads a match written by SaveFile.
func LoadFile(path string) (*Match, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("load match %s: %w", path, err)
	}
	defer gz.Close()
	var m Match
	if err := json.NewDecoder(gz).Decode(&m); err != nil {
		return nil, fmt.Errorf("load match %s: %w", path, err)
	}
	return &m, nil
}

// SaveFile writes a match as gzip-compressed JSON.
func SaveFile(path string, m *Match) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	gz := gzip.NewWriter(f)
	enc := json.NewEncoder(gz)
	enc.SetIndent("", " ")
	if err := enc.Encode(m); err != nil {
		return err
	}
	return gz.Close()
}
