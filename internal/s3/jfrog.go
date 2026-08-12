package s3

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"github.com/open-policy-agent/opa-control-plane/internal/config"
	ext_os "github.com/open-policy-agent/opa-control-plane/pkg/objectstorage"
)

// JFrogArtifactory implements ObjectStorage for JFrog Cloud Artifactory.
// It uses the Artifactory REST API for artifact deployment.
// See: https://jfrog.com/help/display/JFROG/Deploy+Artifacts
//
// Artifacts are stored with a timestamp suffix to support versioning.
// For example, if path is "bundle.tar.gz", the actual artifact will be
// stored as "bundle-20060102T150405.tar.gz" (using Go's reference time format).
type JFrogArtifactory struct {
	url        string
	repository string
	path       string
	client     *http.Client
}

// newJFrogArtifactory creates a new JFrog Artifactory storage client.
func newJFrogArtifactory(ctx context.Context, c *config.JFrogArtifactory) (*JFrogArtifactory, error) {
	var client *http.Client

	if c.Credentials == nil {
		client = &http.Client{}
	} else {
		value, err := c.Credentials.Resolve(ctx)
		if err != nil {
			return nil, err
		}

		auth, ok := value.(config.SecretJFrog)
		if !ok {
			return nil, fmt.Errorf("invalid JFrog secret type")
		}

		client, err = auth.Client(ctx)
		if err != nil {
			return nil, err
		}
	}

	return &JFrogArtifactory{
		url:        c.URL,
		repository: c.Repository,
		path:       c.Path,
		client:     client,
	}, nil
}

// versionedPath generates a versioned path with timestamp suffix.
// For example: "bundle.tar.gz" -> "bundle-20060102T150405.tar.gz"
func (j *JFrogArtifactory) versionedPath() string {
	ext := filepath.Ext(j.path)
	base := j.path[:len(j.path)-len(ext)]
	timestamp := time.Now().Format("20060102T150405")
	return fmt.Sprintf("%s-%s%s", base, timestamp, ext)
}

// Upload deploys an artifact to JFrog Artifactory with a timestamp suffix.
// Uses HTTP PUT to deploy the artifact.
// Includes SHA256 checksum as X-Checksum-SHA256 header.
// See: https://jfrog.com/help/display/JFROG/Deploy+Artifacts
func (j *JFrogArtifactory) Upload(ctx context.Context, body io.ReadSeeker, opts ext_os.UploadOptions) error {
	digest, err := j.computeDigest(body)
	if err != nil {
		return err
	}

	_, err = body.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	versionedPath := j.versionedPath()
	url := fmt.Sprintf("%s/%s/%s", j.url, j.repository, versionedPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Checksum-SHA256", hex.EncodeToString(digest))
	if opts.Revision != "" {
		req.Header.Set("X-Artifactory-Meta-Revision", opts.Revision)
	}

	resp, err := j.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload to JFrog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("jfrog upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Download retrieves an artifact from JFrog Artifactory.
// Uses HTTP GET to download the artifact at the configured path.
func (j *JFrogArtifactory) Download(ctx context.Context) (io.Reader, error) {
	url := fmt.Sprintf("%s/%s/%s", j.url, j.repository, j.path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := j.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download from JFrog: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("jfrog download failed with status %d: %s", resp.StatusCode, string(body))
	}

	return resp.Body, nil
}

// computeDigest calculates the SHA256 digest of the artifact content.
func (j *JFrogArtifactory) computeDigest(body io.Reader) ([]byte, error) {
	d := sha256.New()
	_, err := io.Copy(d, body)
	if err != nil {
		return nil, err
	}
	return d.Sum(nil), nil
}
