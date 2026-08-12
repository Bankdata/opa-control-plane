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
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"

	"github.com/open-policy-agent/opa-control-plane/internal/config"
	ext_os "github.com/open-policy-agent/opa-control-plane/pkg/objectstorage"
)

func TestS3(t *testing.T) {
	// Set mock AWS credentials to avoid IMDS errors.
	t.Setenv("AWS_ACCESS_KEY_ID", "mock-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "mock-secret-key")
	t.Setenv("AWS_REGION", "us-east-1")

	// Create a mock S3 service with a test bucket.

	mock := s3mem.New()
	if err := mock.CreateBucket("test"); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(gofakes3.New(mock).Server())
	defer ts.Close()

	ctx := context.Background()

	// Upload a bundle to the mock S3 service.

	cfg := config.ObjectStorage{
		AmazonS3: &config.AmazonS3{
			Bucket: "test",
			Key:    "a/b/c",
			URL:    ts.URL,
		},
	}

	storage, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	bundle := bytes.NewReader([]byte("bundle content"))
	err = storage.Upload(ctx, bundle, ext_os.UploadOptions{})
	if err != nil {
		t.Fatalf("expected no error while uploading bundle: %v", err)
	}

	// Verify that the bundle was uploaded correctly.

	object, err := mock.GetObject("test", "a/b/c", nil)
	if err != nil {
		t.Fatalf("expected no error while getting object: %v", err)
	}

	contents, err := io.ReadAll(object.Contents)
	if err != nil {
		t.Fatalf("expected no error while reading object contents: %v", err)
	}

	if string(contents) != "bundle content" {
		t.Fatalf("expected object contents to be 'bundle content', got '%s'", contents)
	}

	reader, err := storage.Download(ctx)
	if err != nil {
		t.Fatal(err)
	}

	bs, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	if string(bs) != "bundle content" {
		t.Fatalf("expected object contents to be 'bundle content', got '%s'", contents)
	}
}

func TestS3WithRevision(t *testing.T) {
	// Set mock AWS credentials to avoid IMDS errors.
	t.Setenv("AWS_ACCESS_KEY_ID", "mock-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "mock-secret-key")
	t.Setenv("AWS_REGION", "us-east-1")

	// Create a mock S3 service with a test bucket.
	mock := s3mem.New()
	if err := mock.CreateBucket("test"); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(gofakes3.New(mock).Server())
	defer ts.Close()

	ctx := context.Background()

	cfg := config.ObjectStorage{
		AmazonS3: &config.AmazonS3{
			Bucket: "test",
			Key:    "bundle-with-revision",
			URL:    ts.URL,
		},
	}

	storage, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	s3Storage, ok := storage.(*AmazonS3)
	if !ok {
		t.Fatal("expected storage to be of type *AmazonS3")
	}

	// Upload a bundle with a revision
	bundleContent := []byte("bundle content with revision")
	bundle := bytes.NewReader(bundleContent)
	revision := "v1.2.3"
	err = storage.Upload(ctx, bundle, ext_os.UploadOptions{Revision: revision})
	if err != nil {
		t.Fatalf("expected no error while uploading bundle: %v", err)
	}

	// Verify that the bundle was uploaded with correct metadata using HeadObject
	output, err := s3Storage.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &s3Storage.bucket,
		Key:    &s3Storage.key,
	})
	if err != nil {
		t.Fatalf("expected no error while getting object metadata: %v", err)
	}

	// Verify sha256 metadata is present
	expectedHash := sha256.Sum256(bundleContent)
	expectedHashStr := hex.EncodeToString(expectedHash[:])
	if output.Metadata["sha256"] != expectedHashStr {
		t.Errorf("expected sha256 metadata to be %q, got %q", expectedHashStr, output.Metadata["sha256"])
	}

	// Verify revision metadata is present
	if output.Metadata["revision"] != revision {
		t.Errorf("expected revision metadata to be %q, got %q", revision, output.Metadata["revision"])
	}
}

func TestS3WithoutRevision(t *testing.T) {
	// Set mock AWS credentials to avoid IMDS errors.
	t.Setenv("AWS_ACCESS_KEY_ID", "mock-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "mock-secret-key")
	t.Setenv("AWS_REGION", "us-east-1")

	// Create a mock S3 service with a test bucket.
	mock := s3mem.New()
	if err := mock.CreateBucket("test"); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(gofakes3.New(mock).Server())
	defer ts.Close()

	ctx := context.Background()

	cfg := config.ObjectStorage{
		AmazonS3: &config.AmazonS3{
			Bucket: "test",
			Key:    "bundle-without-revision",
			URL:    ts.URL,
		},
	}

	storage, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	s3Storage, ok := storage.(*AmazonS3)
	if !ok {
		t.Fatal("expected storage to be of type *AmazonS3")
	}

	// Upload a bundle without a revision
	bundleContent := []byte("bundle content without revision")
	bundle := bytes.NewReader(bundleContent)
	err = storage.Upload(ctx, bundle, ext_os.UploadOptions{})
	if err != nil {
		t.Fatalf("expected no error while uploading bundle: %v", err)
	}

	// Verify that the bundle was uploaded with correct metadata using HeadObject
	output, err := s3Storage.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &s3Storage.bucket,
		Key:    &s3Storage.key,
	})
	if err != nil {
		t.Fatalf("expected no error while getting object metadata: %v", err)
	}

	// Verify sha256 metadata is present
	expectedHash := sha256.Sum256(bundleContent)
	expectedHashStr := hex.EncodeToString(expectedHash[:])
	if output.Metadata["sha256"] != expectedHashStr {
		t.Errorf("expected sha256 metadata to be %q, got %q", expectedHashStr, output.Metadata["sha256"])
	}

	// Verify revision metadata is NOT present when revision is empty
	if _, exists := output.Metadata["revision"]; exists {
		t.Errorf("expected revision metadata to not be present, but got %q", output.Metadata["revision"])
	}
}

func TestS3NotModified(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "mock-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "mock-secret-key")
	t.Setenv("AWS_REGION", "us-east-1")

	mock := s3mem.New()
	if err := mock.CreateBucket("test"); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(gofakes3.New(mock).Server())
	defer ts.Close()

	ctx := context.Background()

	storage, err := New(ctx, config.ObjectStorage{
		AmazonS3: &config.AmazonS3{
			Bucket: "test",
			Key:    "not-modified",
			URL:    ts.URL,
		},
	})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	content := []byte("same content")

	// First upload should succeed.
	r := bytes.NewReader(content)
	if err := storage.Upload(ctx, r, ext_os.UploadOptions{}); err != nil {
		t.Fatalf("first upload: %v", err)
	}

	// Second upload with identical content should return ErrNotModified.
	r = bytes.NewReader(content)
	if err := storage.Upload(ctx, r, ext_os.UploadOptions{}); !errors.Is(err, ext_os.ErrNotModified) {
		t.Fatalf("second upload: got %v, want ErrNotModified", err)
	}

	// Upload with different content should succeed.
	r2 := bytes.NewReader([]byte("different content"))
	if err := storage.Upload(ctx, r2, ext_os.UploadOptions{}); err != nil {
		t.Fatalf("third upload: %v", err)
	}
}

func TestFileSystemNotModified(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bundle.tar.gz")

	ctx := context.Background()

	storage, err := New(ctx, config.ObjectStorage{
		FileSystemStorage: &config.FileSystemStorage{Path: path},
	})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	content := []byte("same content")

	// First upload should write the file.
	r := bytes.NewReader(content)
	if err := storage.Upload(ctx, r, ext_os.UploadOptions{}); err != nil {
		t.Fatalf("first upload: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("bundle file not created: %v", err)
	}

	// Second upload with identical content should return ErrNotModified.
	r = bytes.NewReader(content)
	if err := storage.Upload(ctx, r, ext_os.UploadOptions{}); !errors.Is(err, ext_os.ErrNotModified) {
		t.Fatalf("second upload: got %v, want ErrNotModified", err)
	}

	// Upload with different content should succeed.
	r2 := bytes.NewReader([]byte("different content"))
	if err := storage.Upload(ctx, r2, ext_os.UploadOptions{}); err != nil {
		t.Fatalf("third upload: %v", err)
	}
}

func TestGCSNotModified(t *testing.T) {
	mock := fakestorage.NewServer(nil)
	defer mock.Stop()

	mock.CreateBucketWithOpts(fakestorage.CreateBucketOpts{
		Name: "test",
	})

	// fake-gcs-server requires its own pre-configured client (using a custom
	// HTTP transport), we can't nicely pass this into the New constructor as we do with S3.
	gcsStorage := &GCPCloudStorage{
		bucket: "test",
		object: "not-modified",
		client: mock.Client(),
	}

	content := []byte("same content")

	// First upload should write the file.
	r := bytes.NewReader(content)
	if err := gcsStorage.Upload(t.Context(), r, ext_os.UploadOptions{}); err != nil {
		t.Fatalf("first upload: %v", err)
	}

	// Second upload with identical content should return ErrNotModified.
	r = bytes.NewReader(content)
	if err := gcsStorage.Upload(t.Context(), r, ext_os.UploadOptions{}); !errors.Is(err, ext_os.ErrNotModified) {
		t.Fatalf("second upload: got %v, want ErrNotModified", err)
	}

	// Upload with different content should succeed.
	r2 := bytes.NewReader([]byte("different content"))
	if err := gcsStorage.Upload(t.Context(), r2, ext_os.UploadOptions{}); err != nil {
		t.Fatalf("third upload: %v", err)
	}
}

func TestJFrogArtifactory(t *testing.T) {
	mux := http.NewServeMux()
	uploads := make(map[string][]byte)
	var uploadedChecksum string

	mux.HandleFunc("PUT /test-repo/test-bundle", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		uploadedChecksum = r.Header.Get("X-Checksum-SHA256")
		uploads["test-bundle"] = body
		w.WriteHeader(http.StatusCreated)
	})

	mux.HandleFunc("HEAD /test-repo/test-bundle", func(w http.ResponseWriter, r *http.Request) {
		if content, ok := uploads["test-bundle"]; ok {
			digest := sha256.Sum256(content)
			w.Header.Set("X-Checksum-SHA256", hex.EncodeToString(digest[:]))
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	})

	mux.HandleFunc("GET /test-repo/test-bundle", func(w http.ResponseWriter, r *http.Request) {
		if content, ok := uploads["test-bundle"]; ok {
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
			Path:       "test-bundle",
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

func TestJFrogArtifactoryNotModified(t *testing.T) {
	mux := http.NewServeMux()
	uploads := make(map[string][]byte)

	mux.HandleFunc("PUT /test-repo/test-bundle", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		uploads["test-bundle"] = body
		w.WriteHeader(http.StatusCreated)
	})

	mux.HandleFunc("HEAD /test-repo/test-bundle", func(w http.ResponseWriter, r *http.Request) {
		if content, ok := uploads["test-bundle"]; ok {
			digest := sha256.Sum256(content)
			w.Header().Set("X-Checksum-SHA256", hex.EncodeToString(digest[:]))
			w.WriteHeader(http.StatusOK)
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
			Path:       "test-bundle",
		},
	})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	content := []byte("same content")

	r := bytes.NewReader(content)
	if err := storage.Upload(ctx, r, ext_os.UploadOptions{}); err != nil {
		t.Fatalf("first upload: %v", err)
	}

	r = bytes.NewReader(content)
	if err := storage.Upload(ctx, r, ext_os.UploadOptions{}); !errors.Is(err, ext_os.ErrNotModified) {
		t.Fatalf("second upload: got %v, want ErrNotModified", err)
	}

	r2 := bytes.NewReader([]byte("different content"))
	if err := storage.Upload(ctx, r2, ext_os.UploadOptions{}); err != nil {
		t.Fatalf("third upload: %v", err)
	}
}
