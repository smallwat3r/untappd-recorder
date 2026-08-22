package main

import (
	"context"
	"sync"
	"testing"

	"github.com/smallwat3r/untappd-recorder/internal/faceblur"
)

type mockStore struct {
	mu       sync.Mutex
	keys     []string
	objects  map[string][]byte
	metadata map[string]map[string]string
	replaced map[string][]byte
	order    []string
}

func (m *mockStore) ListJPGKeys(ctx context.Context) ([]string, error) {
	return m.keys, nil
}

func (m *mockStore) DownloadWithMetadata(
	ctx context.Context,
	key string,
) ([]byte, map[string]string, error) {
	return m.objects[key], m.metadata[key], nil
}

func (m *mockStore) Replace(
	ctx context.Context,
	key string,
	b []byte,
	md map[string]string,
	contentType string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.replaced[key] = b
	m.order = append(m.order, key)
	return nil
}

func setEnv(t *testing.T) {
	t.Setenv("R2_ACCOUNT_ID", "test-account-id")
	t.Setenv("R2_ACCESS_KEY_ID", "test-key-id")
	t.Setenv("R2_SECRET_ACCESS_KEY", "test-secret")
	t.Setenv("BUCKET_NAME", "test-bucket")
	t.Setenv("NUM_WORKERS", "1")
}

// stubs: photos containing "face" get one detection, others none
func fakeDetect(b []byte) ([]faceblur.Face, error) {
	if string(b) == "face" {
		return []faceblur.Face{{Q: 0.9}}, nil
	}
	return nil, nil
}

func fakeBlur(b []byte, faces []faceblur.Face) ([]byte, error) {
	return []byte("blurred"), nil
}

func fakeToWEBP(b []byte) ([]byte, error) {
	return append([]byte("webp:"), b...), nil
}

func TestRun_ReplacesInPlace(t *testing.T) {
	setEnv(t)

	st := &mockStore{
		keys: []string{"2019/08/18/111.jpg", "2019/08/18/222.jpg", "latest.jpg"},
		objects: map[string][]byte{
			"2019/08/18/111.jpg": []byte("face"),
			"2019/08/18/222.jpg": []byte("no-face"),
			"latest.jpg":         []byte("face"),
		},
		metadata: map[string]map[string]string{
			"2019/08/18/111.jpg": {"id": "111"},
		},
		replaced: map[string][]byte{},
	}

	if err := run(context.Background(), false, nil, st, fakeDetect, fakeBlur, fakeToWEBP); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := string(st.replaced["2019/08/18/111.jpg"]); got != "blurred" {
		t.Errorf("expected jpg with face to be replaced, got %q", got)
	}
	if got := string(st.replaced["2019/08/18/WEBP/111.webp"]); got != "webp:blurred" {
		t.Errorf("expected webp sibling to be regenerated, got %q", got)
	}
	if _, ok := st.replaced["2019/08/18/222.jpg"]; ok {
		t.Error("photo without faces must not be rewritten")
	}
	if got := string(st.replaced["latest.jpg"]); got != "blurred" {
		t.Errorf("expected latest.jpg to be replaced, got %q", got)
	}
	if len(st.replaced) != 3 {
		t.Errorf("expected 3 replacements, got %d: %v", len(st.replaced), st.replaced)
	}

	// the webp sibling must be written before its jpg, so a failure in
	// between is repaired by a re-run
	var jpgAt, webpAt int
	for i, k := range st.order {
		switch k {
		case "2019/08/18/111.jpg":
			jpgAt = i
		case "2019/08/18/WEBP/111.webp":
			webpAt = i
		}
	}
	if webpAt > jpgAt {
		t.Errorf("webp must be replaced before jpg, got order %v", st.order)
	}
}

func TestRun_DryRunWritesNothing(t *testing.T) {
	setEnv(t)

	st := &mockStore{
		keys:     []string{"2019/08/18/111.jpg"},
		objects:  map[string][]byte{"2019/08/18/111.jpg": []byte("face")},
		metadata: map[string]map[string]string{},
		replaced: map[string][]byte{},
	}

	if err := run(context.Background(), true, nil, st, fakeDetect, fakeBlur, fakeToWEBP); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(st.replaced) != 0 {
		t.Errorf("dry-run must not write, got %v", st.replaced)
	}
}

func TestRun_ExplicitKeysSkipListing(t *testing.T) {
	setEnv(t)

	st := &mockStore{
		keys: []string{"2019/08/18/111.jpg", "2019/08/25/796751183.jpg"},
		objects: map[string][]byte{
			"2019/08/18/111.jpg":       []byte("face"),
			"2019/08/25/796751183.jpg": []byte("face"),
		},
		metadata: map[string]map[string]string{},
		replaced: map[string][]byte{},
	}

	err := run(
		context.Background(), false, []string{"2019/08/25/796751183.jpg"},
		st, fakeDetect, fakeBlur, fakeToWEBP,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := string(st.replaced["2019/08/25/796751183.jpg"]); got != "blurred" {
		t.Errorf("expected targeted key to be replaced, got %q", got)
	}
	if _, ok := st.replaced["2019/08/18/111.jpg"]; ok {
		t.Error("untargeted photo must not be rewritten")
	}
}
