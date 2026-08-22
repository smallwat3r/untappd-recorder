// Package faceblur detects faces with a YOLOv8-face model (via ONNX Runtime)
// and blurs them with libvips before photos are stored.
package faceblur

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	_ "image/jpeg"
	"os"
	"sort"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
	xdraw "golang.org/x/image/draw"

	"github.com/smallwat3r/untappd-recorder/internal/vips"
)

//go:embed yolov8n-face.onnx
var modelData []byte

// DefaultMinQuality is the detection confidence cutoff (0 to 1): higher
// means fewer false positives, lower means fewer missed faces.
const DefaultMinQuality = 0.35

const (
	inputSize    = 640  // YOLO letterbox size
	iouThreshold = 0.45 // NMS overlap cutoff for duplicate detections
	padGray      = 114  // letterbox padding colour used at training time
)

// Face is a detected face region with its detection confidence (0 to 1).
type Face struct {
	Rect image.Rectangle
	Q    float32

	// idx is the raw model output column, used internally to rebuild the
	// grown box after non-maximum suppression
	idx int
}

// session state is lazily initialised and only cached on success, so a
// transient failure (library briefly unavailable, memory pressure) is
// retried on the next call instead of poisoning the whole process.
var (
	sessionMu sync.Mutex
	session   *ort.DynamicAdvancedSession
	ortEnvUp  bool
)

func getSession() (*ort.DynamicAdvancedSession, error) {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	if session != nil {
		return session, nil
	}
	if !ortEnvUp {
		ort.SetSharedLibraryPath(libraryPath())
		if err := ort.InitializeEnvironment(); err != nil {
			return nil, fmt.Errorf("failed to initialise onnxruntime: %w", err)
		}
		ortEnvUp = true
	}
	s, err := ort.NewDynamicAdvancedSessionWithONNXData(
		modelData, []string{"images"}, []string{"output0"}, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create inference session: %w", err)
	}
	session = s
	return s, nil
}

// one inference at a time keeps peak memory flat on small instances;
// parallelise Run() only if detection ever dominates wall clock
var inferMu sync.Mutex

func libraryPath() string {
	if p := os.Getenv("ONNXRUNTIME_LIB"); p != "" {
		return p
	}
	for _, p := range []string{
		// bundled copy, found when run from the repo root
		"bin/onnxruntime-linux-x64-1.29.0/lib/libonnxruntime.so",
		"/usr/lib/libonnxruntime.so",
		"/usr/local/lib/libonnxruntime.so",
		"/opt/homebrew/lib/libonnxruntime.dylib",
		"/usr/local/lib/libonnxruntime.dylib",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// fall back to the dynamic loader's default search path
	return "onnxruntime.so"
}

// Blur returns the JPG with every detected face gaussian-blurred, along with
// the number of faces found. When no face is detected the input bytes are
// returned untouched, so callers can skip re-uploading.
func Blur(jpg []byte, minQuality float32) ([]byte, int, error) {
	faces, err := Detect(jpg, minQuality)
	if err != nil {
		return nil, 0, fmt.Errorf("face detection: %w", err)
	}
	if len(faces) == 0 {
		return jpg, 0, nil
	}

	out, err := BlurFaces(jpg, faces)
	if err != nil {
		return nil, 0, err
	}
	return out, len(faces), nil
}

// BlurFaces gaussian-blurs the given face regions in a JPG. Use it with the
// result of Detect to avoid running detection twice.
func BlurFaces(jpg []byte, faces []Face) ([]byte, error) {
	img, err := vips.NewJpegloadBuffer(jpg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to load jpg: %w", err)
	}
	defer img.Close()

	for _, f := range faces {
		if err := blurRegion(img, f.Rect); err != nil {
			return nil, err
		}
	}

	out, err := img.JpegsaveBuffer(&vips.JpegsaveBufferOptions{Q: 90})
	if err != nil {
		return nil, fmt.Errorf("failed to save jpg: %w", err)
	}
	return out, nil
}

func blurRegion(img *vips.Image, r image.Rectangle) error {
	face, err := img.Copy(nil)
	if err != nil {
		return fmt.Errorf("failed to copy image: %w", err)
	}
	defer face.Close()

	if err := face.ExtractArea(r.Min.X, r.Min.Y, r.Dx(), r.Dy()); err != nil {
		return fmt.Errorf("failed to extract face region: %w", err)
	}

	// scale blur strength with face size so large faces are unrecognisable
	sigma := float64(r.Dx()) / 8
	if sigma < 4 {
		sigma = 4
	}
	if err := face.Gaussblur(sigma, nil); err != nil {
		return fmt.Errorf("failed to blur face region: %w", err)
	}

	if err := img.Insert(face, r.Min.X, r.Min.Y, nil); err != nil {
		return fmt.Errorf("failed to insert blurred region: %w", err)
	}
	return nil
}

// Detect returns the face regions found in a JPG, with confidence scores,
// keeping only detections at or above minQuality.
func Detect(jpg []byte, minQuality float32) ([]Face, error) {
	session, err := getSession()
	if err != nil {
		return nil, err
	}

	src, _, err := image.Decode(bytes.NewReader(jpg))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	input, scale, xOff, yOff := letterbox(src)

	inputTensor, err := ort.NewTensor(ort.NewShape(1, 3, inputSize, inputSize), input)
	if err != nil {
		return nil, fmt.Errorf("failed to create input tensor: %w", err)
	}
	defer inputTensor.Destroy()

	outputs := []ort.Value{nil}
	inferMu.Lock()
	err = session.Run([]ort.Value{inputTensor}, outputs)
	inferMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("inference failed: %w", err)
	}
	outTensor, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("unexpected output tensor type %T", outputs[0])
	}
	defer outTensor.Destroy()

	return decodeDetections(
		outTensor.GetShape(), outTensor.GetData(),
		src.Bounds(), scale, xOff, yOff, minQuality,
	)
}

// letterbox scales the image to fit inputSize x inputSize, centred on grey
// padding, and returns the normalised NCHW tensor data plus the transform
// needed to map detections back to source pixels.
func letterbox(src image.Image) (data []float32, scale float64, xOff, yOff int) {
	b := src.Bounds()
	scale = float64(inputSize) / float64(b.Dx())
	if s := float64(inputSize) / float64(b.Dy()); s < scale {
		scale = s
	}
	w := int(float64(b.Dx()) * scale)
	h := int(float64(b.Dy()) * scale)
	xOff = (inputSize - w) / 2
	yOff = (inputSize - h) / 2

	dst := image.NewRGBA(image.Rect(0, 0, inputSize, inputSize))
	for i := range dst.Pix {
		dst.Pix[i] = padGray
	}
	xdraw.ApproxBiLinear.Scale(
		dst, image.Rect(xOff, yOff, xOff+w, yOff+h), src, b, xdraw.Src, nil,
	)

	data = make([]float32, 3*inputSize*inputSize)
	plane := inputSize * inputSize
	for y := 0; y < inputSize; y++ {
		row := dst.Pix[y*dst.Stride : y*dst.Stride+inputSize*4]
		for x := 0; x < inputSize; x++ {
			i := y*inputSize + x
			data[i] = float32(row[x*4]) / 255
			data[plane+i] = float32(row[x*4+1]) / 255
			data[2*plane+i] = float32(row[x*4+2]) / 255
		}
	}
	return data, scale, xOff, yOff
}

// decodeDetections converts the raw [1, 5, N] YOLO output (cx, cy, w, h,
// confidence per column) into deduplicated face rectangles in source pixels.
func decodeDetections(
	shape []int64,
	data []float32,
	bounds image.Rectangle,
	scale float64,
	xOff, yOff int,
	minQuality float32,
) ([]Face, error) {
	if len(shape) != 3 || shape[1] < 5 {
		return nil, fmt.Errorf("unexpected model output shape %v", shape)
	}
	n := int(shape[2])

	toRect := func(i int, grow float64) image.Rectangle {
		cx, cy := float64(data[i]), float64(data[n+i])
		w := float64(data[2*n+i]) * grow
		h := float64(data[3*n+i]) * grow
		x0 := int((cx - w/2 - float64(xOff)) / scale)
		y0 := int((cy - h/2 - float64(yOff)) / scale)
		x1 := int((cx + w/2 - float64(xOff)) / scale)
		y1 := int((cy + h/2 - float64(yOff)) / scale)
		return image.Rect(x0, y0, x1, y1)
	}

	// suppress duplicates on the raw detector boxes, so two genuinely
	// distinct but close faces are not merged by the blur padding
	var candidates []Face
	for i := 0; i < n; i++ {
		conf := data[4*n+i]
		if conf < minQuality {
			continue
		}
		candidates = append(candidates, Face{Rect: toRect(i, 1.0), Q: conf, idx: i})
	}
	kept := nms(candidates)

	// grow the surviving boxes slightly so hairline and chin are covered
	// too, and clamp to the image
	var faces []Face
	for _, f := range kept {
		r := toRect(f.idx, 1.2).Intersect(bounds)
		if !r.Empty() {
			faces = append(faces, Face{Rect: r, Q: f.Q})
		}
	}
	return faces, nil
}

// nms drops overlapping detections of the same face, keeping the most
// confident one.
func nms(candidates []Face) []Face {
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Q > candidates[j].Q
	})

	var faces []Face
	for _, c := range candidates {
		keep := true
		for _, f := range faces {
			if iou(c.Rect, f.Rect) > iouThreshold {
				keep = false
				break
			}
		}
		if keep {
			faces = append(faces, c)
		}
	}
	return faces
}

func iou(a, b image.Rectangle) float64 {
	inter := a.Intersect(b)
	if inter.Empty() {
		return 0
	}
	interArea := inter.Dx() * inter.Dy()
	union := a.Dx()*a.Dy() + b.Dx()*b.Dy() - interArea
	return float64(interArea) / float64(union)
}
