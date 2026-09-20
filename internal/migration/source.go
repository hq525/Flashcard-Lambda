// Package migration moves approved legacy media into immutable managed keys.
// It never deletes source media, and dry runs never fetch or write media.
package migration

import (
	"errors"
	"net"
	"net/url"
	"path"
	"regexp"
	"strings"
	"unicode"
)

var ErrInvalidSource = errors.New("legacy source is not an approved S3 object URL")

var bucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
var legacyHostPattern = regexp.MustCompile(`^([a-z0-9][a-z0-9.-]{1,61}[a-z0-9])\.s3(?:[.-][a-z]{2}(?:-[a-z]+)+-[0-9]+)?\.amazonaws\.com$`)

// Source is an SDK bucket/key pair, never a URL to fetch directly.
type Source struct {
	Bucket string
	Key    string
}

func validBucket(bucket string) bool {
	if !bucketPattern.MatchString(bucket) || strings.Contains(bucket, "..") || net.ParseIP(bucket) != nil {
		return false
	}
	for _, label := range strings.Split(bucket, ".") {
		if label == "" || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
	}
	return true
}

func ParseLegacyURL(raw string, allowedBuckets []string) (Source, error) {
	if len(raw) > 4096 || strings.ContainsAny(raw, "?#\\") || strings.ContainsFunc(raw, unicode.IsSpace) {
		return Source{}, ErrInvalidSource
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Opaque != "" || u.User != nil || u.Host != u.Hostname() || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return Source{}, ErrInvalidSource
	}
	match := legacyHostPattern.FindStringSubmatch(u.Host)
	if len(match) != 2 || !validBucket(match[1]) {
		return Source{}, ErrInvalidSource
	}
	approved := false
	for _, bucket := range allowedBuckets {
		if bucket == match[1] {
			approved = true
			break
		}
	}
	if !approved {
		return Source{}, ErrInvalidSource
	}
	escaped := strings.ToLower(u.EscapedPath())
	if strings.Contains(escaped, "%2f") || strings.Contains(escaped, "%5c") {
		return Source{}, ErrInvalidSource
	}
	key := strings.TrimPrefix(u.Path, "/")
	if len(key) == 0 || len(key) > 1024 || path.Clean(key) != key || strings.HasSuffix(key, "/") || strings.ContainsAny(key, "\\") || strings.ContainsFunc(key, unicode.IsControl) {
		return Source{}, ErrInvalidSource
	}
	parts := strings.SplitN(key, "/", 2)
	if len(parts) != 2 || parts[1] == "" {
		return Source{}, ErrInvalidSource
	}
	switch parts[0] {
	case "question-images", "answer-images", "images":
	default:
		return Source{}, ErrInvalidSource
	}
	return Source{Bucket: match[1], Key: key}, nil
}
