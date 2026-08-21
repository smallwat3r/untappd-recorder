package faceblur

import (
	"bytes"
	"os"
	"testing"
)

func TestBlur_FindsAndBlursFaces(t *testing.T) {
	// sample.jpg is a known face-containing test image
	jpg, err := os.ReadFile("testdata/sample.jpg")
	if err != nil {
		t.Fatalf("failed to read sample: %v", err)
	}

	out, n, err := Blur(jpg, DefaultMinQuality)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n == 0 {
		t.Fatal("expected at least one face to be detected")
	}
	if bytes.Equal(out, jpg) {
		t.Fatal("expected blurred output to differ from input")
	}
}

func TestBlur_NoFacesIsNoop(t *testing.T) {
	jpg, err := os.ReadFile("../../img/missing.jpg")
	if err != nil {
		t.Fatalf("failed to read placeholder: %v", err)
	}

	out, n, err := Blur(jpg, DefaultMinQuality)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected no faces in placeholder, got %d", n)
	}
	if !bytes.Equal(out, jpg) {
		t.Fatal("expected input bytes to be returned untouched")
	}
}
