package migration

import "testing"

func TestLegacySourcesAcceptOnlyExplicitS3Locations(t *testing.T) {
	allowed := []string{"legacy-bucket"}
	for _, raw := range []string{
		"https://legacy-bucket.s3.amazonaws.com/question-images/photo.png",
		"https://legacy-bucket.s3.ap-southeast-1.amazonaws.com/answer-images/photo%20one.jpg",
		"https://legacy-bucket.s3-us-east-1.amazonaws.com/images/old/photo.gif",
	} {
		source, err := ParseLegacyURL(raw, allowed)
		if err != nil || source.Bucket != "legacy-bucket" || source.Key == "" {
			t.Errorf("valid legacy source rejected: %+v %v", source, err)
		}
	}
	for _, raw := range []string{
		"http://legacy-bucket.s3.amazonaws.com/question-images/photo.png",
		"https://evil.example/question-images/photo.png",
		"https://legacy-bucket.s3.amazonaws.com.evil.example/question-images/photo.png",
		"https://unapproved-bucket.s3.amazonaws.com/question-images/photo.png",
		"https://legacy-bucket.s3.amazonaws.com:443/question-images/photo.png",
		"https://user:secret@legacy-bucket.s3.amazonaws.com/question-images/photo.png",
		"https://legacy-bucket.s3.amazonaws.com/question-images/photo.png?secret=token",
		"https://legacy-bucket.s3.amazonaws.com/question-images/photo.png?",
		"https://legacy-bucket.s3.amazonaws.com/question-images/photo.png#secret",
		"https://legacy-bucket.s3.amazonaws.com/question-images/photo.png#",
		"https://s3.amazonaws.com/legacy-bucket/question-images/photo.png",
		"https://legacy-bucket.s3.amazonaws.com/private/photo.png",
		"https://legacy-bucket.s3.amazonaws.com/question-images/../private.png",
		"https://legacy-bucket.s3.amazonaws.com/question-images/%2e%2e/private.png",
		"https://legacy-bucket.s3.amazonaws.com/question-images%2Fprivate.png",
		"https://legacy-bucket.s3.amazonaws.com/question-images/%00photo.png",
		"https://legacy-bucket.s3.amazonaws.com/question-images/",
	} {
		if _, err := ParseLegacyURL(raw, allowed); err == nil {
			t.Errorf("untrusted source was accepted: %s", raw)
		}
	}
}
