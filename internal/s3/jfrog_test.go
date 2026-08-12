package s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-policy-agent/opa-control-plane/internal/config"
	ext_os "github.com/open-policy-agent/opa-control-plane/pkg/objectstorage"
)

func TestJFrogArtifactoryUpload(t *testing.T) {
	mux := http.NewServeMux()
	uploadedPaths := make(map[string][]byte)
	var uploadedChecksum string

	mux.HandleFunc("PUT /", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		uploadedChecksum = r.Header.Get("X-Checksum-SHA256")
		uploadedPaths[path] = body
		w.WriteHeader(http.StatusCreated)
	})

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if content, ok := uploadedPaths[path]; ok {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(content)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	ctx := context.Background()

	cfg := config.ObjectStorage{
		JFrogArtifactory: &config.JFrogArtifactory{
			URL:        ts.URL,
			Repository: "test-repo",
			Path:       "bundle.tar.gz",
		},
	}

	storage, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	bundleContent := []byte("test bundle content")
	bundle := bytes.NewReader(bundleContent)

	err = storage.Upload(ctx, bundle, ext_os.UploadOptions{})
	if err != nil {
		t.Fatalf("expected no error while uploading bundle: %v", err)
	}

	expectedDigest := sha256.Sum256(bundleContent)
	if uploadedChecksum != hex.EncodeToString(expectedDigest[:]) {
		t.Fatalf("expected checksum %s, got %s", hex.EncodeToString(expectedDigest[:]), uploadedChecksum)
	}

	if len(uploadedPaths) != 1 {
		t.Fatalf("expected 1 artifact uploaded, got %d", len(uploadedPaths))
	}

	var uploadedPath string
	for path := range uploadedPaths {
		uploadedPath = path
	}

	if !strings.HasPrefix(uploadedPath, "test-repo/bundle-") {
		t.Fatalf("expected path to start with 'test-repo/bundle-', got %s", uploadedPath)
	}

	if !strings.HasSuffix(uploadedPath, ".tar.gz") {
		t.Fatalf("expected path to end with '.tar.gz', got %s", uploadedPath)
	}
}

func TestJFrogArtifactoryDownload(t *testing.T) {
	bundleContent := []byte("test bundle content")
	bundlePath := "test-repo/bundle.tar.gz"

	mux := http.NewServeMux()

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == bundlePath {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(bundleContent)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	ctx := context.Background()

	storage, err := New(ctx, config.ObjectStorage{
		JFrogArtifactory: &config.JFrogArtifactory{
			URL:        ts.URL,
			Repository: "test-repo",
			Path:       "bundle.tar.gz",
		},
	})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	reader, err := storage.Download(ctx)
	if err != nil {
		t.Fatalf("expected no error while downloading bundle: %v", err)
	}

	downloaded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("expected no error while reading downloaded bundle: %v", err)
	}

	if !bytes.Equal(downloaded, bundleContent) {
		t.Fatalf("expected downloaded content to be %s, got %s", bundleContent, downloaded)
	}
}

func TestJFrogArtifactoryVersioning(t *testing.T) {
	mux := http.NewServeMux()
	uploadedPaths := make([]string, 0)

	mux.HandleFunc("PUT /", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		uploadedPaths = append(uploadedPaths, path)
		io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	ctx := context.Background()

	storage, err := New(ctx, config.ObjectStorage{
		JFrogArtifactory: &config.JFrogArtifactory{
			URL:        ts.URL,
			Repository: "test-repo",
			Path:       "bundle.tar.gz",
		},
	})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	bundleContent := []byte("test bundle content")

	for i := 0; i < 3; i++ {
		bundle := bytes.NewReader(bundleContent)
		err := storage.Upload(ctx, bundle, ext_os.UploadOptions{})
		if err != nil {
			t.Fatalf("upload %d failed: %v", i+1, err)
		}
	}

	if len(uploadedPaths) != 3 {
		t.Fatalf("expected 3 uploads with different versioned paths, got %d", len(uploadedPaths))
	}

	for i, path := range uploadedPaths {
		if !strings.Contains(path, "bundle-") || !strings.HasSuffix(path, ".tar.gz") {
			t.Fatalf("upload %d: invalid versioned path %s", i+1, path)
		}
	}

	if uploadedPaths[0] == uploadedPaths[1] {
		t.Fatalf("expected different versioned paths for each upload, got duplicates")
	}
}

func TestJFrogArtifactoryWithRevision(t *testing.T) {
	mux := http.NewServeMux()
	var revisionHeader string

	mux.HandleFunc("PUT /", func(w http.ResponseWriter, r *http.Request) {
		revisionHeader = r.Header.Get("X-Artifactory-Meta-Revision")
		io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	ctx := context.Background()

	storage, err := New(ctx, config.ObjectStorage{
		JFrogArtifactory: &config.JFrogArtifactory{
			URL:        ts.URL,
			Repository: "test-repo",
			Path:       "bundle.tar.gz",
		},
	})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	bundleContent := []byte("test bundle content")
	bundle := bytes.NewReader(bundleContent)
	revision := "v1.2.3"

	err = storage.Upload(ctx, bundle, ext_os.UploadOptions{Revision: revision})
	if err != nil {
		t.Fatalf("expected no error while uploading bundle: %v", err)
	}

	if revisionHeader != revision {
		t.Fatalf("expected revision header to be %q, got %q", revision, revisionHeader)
	}
}

func TestJFrogArtifactoryDownloadNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer ts.Close()

	ctx := context.Background()

	storage, err := New(ctx, config.ObjectStorage{
		JFrogArtifactory: &config.JFrogArtifactory{
			URL:        ts.URL,
			Repository: "test-repo",
			Path:       "nonexistent.tar.gz",
		},
	})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	_, err = storage.Download(ctx)
	if err == nil {
		t.Fatalf("expected error when downloading nonexistent artifact")
	}

	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 error, got: %v", err)
	}
}

func TestJFrogArtifactoryUploadError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer ts.Close()

	ctx := context.Background()

	storage, err := New(ctx, config.ObjectStorage{
		JFrogArtifactory: &config.JFrogArtifactory{
			URL:        ts.URL,
			Repository: "test-repo",
			Path:       "bundle.tar.gz",
		},
	})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	bundleContent := []byte("test bundle content")
	bundle := bytes.NewReader(bundleContent)

	err = storage.Upload(ctx, bundle, ext_os.UploadOptions{})
	if err == nil {
		t.Fatalf("expected error when upload fails")
	}

	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 error, got: %v", err)
	}
}
