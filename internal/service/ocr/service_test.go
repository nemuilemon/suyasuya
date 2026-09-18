package ocr

import (
	"testing"

	"suyasuya/internal/domain"
)

func ptr[T any](v T) *T { return &v }
func TestOCRUnknownAndBadTime(t *testing.T) {
	out := domain.OCRResult{Records: []domain.OCRRecord{{Start: ptr("9:05"), Duration: ptr(20)}}}
	if e := validateOCR(&out); e != nil {
		t.Fatal(e)
	}
	if *out.Records[0].Start != "09:05" || out.Records[0].Kind != nil {
		t.Fatal("unknown kind must stay unknown")
	}
	out.Records[0].End = ptr("25:00")
	if validateOCR(&out) == nil {
		t.Fatal("bad time accepted")
	}
}
