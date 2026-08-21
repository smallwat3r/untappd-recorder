package faceblur

import (
	"image"
	"testing"
)

// two distinct faces whose raw boxes overlap under the NMS threshold must
// both survive, even though the grown blur boxes overlap more heavily.
// Suppressing on grown boxes would leave one face unblurred.
func TestDecodeDetections_NMSRunsOnRawBoxes(t *testing.T) {
	// output layout [1, 5, n]: rows cx, cy, w, h, confidence
	// both boxes 100x100, centres 40px apart: raw IoU 0.43 (kept), grown
	// (1.2x) IoU 0.50 (would be suppressed)
	shape := []int64{1, 5, 2}
	data := []float32{
		200, 240, // cx
		200, 200, // cy
		100, 100, // w
		100, 100, // h
		0.9, 0.8, // confidence
	}

	faces, err := decodeDetections(
		shape, data, image.Rect(0, 0, 640, 640), 1.0, 0, 0, 0.5,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(faces) != 2 {
		t.Fatalf("expected both overlapping faces to be kept, got %d", len(faces))
	}

	// the returned boxes must be the grown ones (120px wide, not 100)
	if got := faces[0].Rect.Dx(); got != 120 {
		t.Errorf("expected grown box width 120, got %d", got)
	}
}

func TestDecodeDetections_FiltersLowConfidence(t *testing.T) {
	shape := []int64{1, 5, 2}
	data := []float32{
		100, 500, // cx
		100, 500, // cy
		50, 50, // w
		50, 50, // h
		0.9, 0.3, // confidence
	}

	faces, err := decodeDetections(
		shape, data, image.Rect(0, 0, 640, 640), 1.0, 0, 0, 0.5,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(faces) != 1 {
		t.Fatalf("expected only the confident detection, got %d", len(faces))
	}
	if faces[0].Q != 0.9 {
		t.Errorf("expected the confident face, got q=%f", faces[0].Q)
	}
}
