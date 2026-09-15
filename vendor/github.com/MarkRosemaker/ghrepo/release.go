package ghrepo

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-github/v80/github"
)

// lenChecksum is the length of a SHA-256 checksum when encoded as hexadecimal (64 characters).
const lenChecksum int64 = 64

func (r *Repository) LatestRelease(ctx context.Context) (*github.RepositoryRelease, error) {
	rel, _, err := r.s.github.Repositories.GetLatestRelease(ctx, r.owner, r.name)
	return rel, err
}

func (r *Repository) LatestReleaseVersion(ctx context.Context) (*semver.Version, error) {
	rel, err := r.LatestRelease(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting latest release: %w", err)
	}

	return semver.NewVersion(rel.GetTagName())
}

// CreateRelease creates a new release for the repository.
func (r *Repository) CreateRelease(ctx context.Context, release *github.RepositoryRelease) (*github.RepositoryRelease, error) {
	rel, _, err := r.s.github.Repositories.CreateRelease(ctx, r.owner, r.name, release)
	return rel, err
}

func closeAndLog(closer io.Closer, name string) {
	if err := closer.Close(); err != nil {
		log.Printf("warning: failed to close %s: %v", name, err)
	}
}

// UploadReleaseBinary zips a binary file and uploads it as a release asset to a GitHub release.
// It also computes a SHA-256 checksum during the upload and uploads a separate checksum file.
//
// The binary is placed inside a zip archive with a single entry. The name of the file inside the zip
// is the repository name with an optional suffix (e.g., ".exe" for Windows binaries).
func (r *Repository) UploadReleaseBinary(ctx context.Context, relID int,
	path string, info fs.FileInfo, suffix string,
) error {
	src, err := r.Open(path)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer closeAndLog(src, path)

	// Create a temporary zip file containing the binary.
	// This gives us the exact size of the compressed asset for the upload request.
	tmpPath, err := r.zipBinary(src, info, suffix)
	if err != nil {
		return fmt.Errorf("zipping binary: %w", err)
	}
	defer os.Remove(tmpPath) //nolint:errcheck

	fi, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("opening temporary zip: %w", err)
	}
	defer closeAndLog(fi, tmpPath)

	// Get file info to know the exact size (required by GitHub API).
	stat, err := fi.Stat()
	if err != nil {
		return fmt.Errorf("stating temporary zip: %w", err)
	}

	// Prepare to compute SHA-256 hash while uploading.
	hash := sha256.New()

	// Name of the zip asset (e.g., "mybinary.zip").
	zipName := info.Name() + ".zip"

	// Upload the zip asset, hashing its contents simultaneously via TeeReader.
	if _, err := r.uploadReleaseAsset(ctx, relID, zipName, io.TeeReader(fi, hash), stat.Size()); err != nil {
		return fmt.Errorf("uploading %q: %w", zipName, err)
	}

	// Now that the hash is complete, create the checksum content (hex-encoded SHA-256).
	checksumContent := hex.EncodeToString(hash.Sum(nil))

	// Name of the checksum asset (standard format used by many projects).
	checksumName := fmt.Sprintf("%s_checksum_sha256.txt", info.Name())

	// Upload the checksum file (fixed size of 64 bytes).
	if _, err := r.uploadReleaseAsset(ctx, relID, checksumName,
		strings.NewReader(checksumContent),
		lenChecksum); err != nil {
		return fmt.Errorf("uploading %q: %w", checksumName, err)
	}

	return nil
}

// zipBinary creates a temporary zip file containing a single binary entry.
// Returns the path to the temporary zip file.
func (r *Repository) zipBinary(fi io.Reader, info fs.FileInfo, suffix string) (string, error) {
	name := r.name + ".zip"
	// Create a temporary file for the zip (pattern ensures unique name).
	tmp, err := os.CreateTemp("", name)
	if err != nil {
		return "", fmt.Errorf("creating temp zip file: %w", err)
	}
	defer closeAndLog(tmp, name)

	// Create a zip header from the original file's metadata.
	fh, err := zip.FileInfoHeader(info)
	if err != nil {
		return "", fmt.Errorf("creating zip header: %w", err)
	}

	// Override the entry name inside the zip to be the repository name + suffix.
	// This makes the extracted file have a predictable, clean name (e.g., "myrepo.exe").
	fh.Name = r.name + suffix

	// Use Deflate compression (better ratio than default Store method).
	fh.Method = zip.Deflate

	// Initialize zip writer.
	zw := zip.NewWriter(tmp)
	defer closeAndLog(zw, "zip writer")

	// Create the single entry in the zip.
	h, err := zw.CreateHeader(fh)
	if err != nil {
		return "", fmt.Errorf("creating zip entry: %w", err)
	}

	// Copy the binary content into the zip entry.
	if _, err := io.Copy(h, fi); err != nil {
		return "", fmt.Errorf("writing to zip: %w", err)
	}

	// Flush and close the writer to finalize the zip file.
	if err := zw.Close(); err != nil {
		return "", fmt.Errorf("closing zip writer: %w", err)
	}

	return tmp.Name(), nil
}

// uploadReleaseAsset uploads a single release asset to GitHub.
//
// It constructs the upload URL, sets the correct Content-Type based on file extension,
// and performs the HTTP request using the go-github client.
//
// Returns the created ReleaseAsset on success.
func (r *Repository) uploadReleaseAsset(ctx context.Context, relID int,
	assetName string, reader io.Reader, size int64,
) (*github.ReleaseAsset, error) {
	// Create the upload request with known content length.
	req, err := r.s.github.NewUploadRequest(
		fmt.Sprintf("repos/%s/%s/releases/%d/assets?name=%s", r.owner, r.name, relID, assetName),
		reader, size,
		mime.TypeByExtension(filepath.Ext(assetName)),
	)
	if err != nil {
		return nil, fmt.Errorf("creating upload request: %w", err)
	}

	// Execute the request and unmarshal the response into a ReleaseAsset.
	asset := &github.ReleaseAsset{}

	resp, err := r.s.github.Do(ctx, req, asset)
	if err != nil {
		return nil, fmt.Errorf("performing upload request: %w", err)
	}

	// Check for successful creation.
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upload failed with status %d %s: %s",
			resp.StatusCode, http.StatusText(resp.StatusCode), string(b))
	}

	return asset, nil
}
