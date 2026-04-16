// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
)

type Client interface {
	Download(ctx context.Context, filePath string) error
	Upload(ctx context.Context, bucketPath, localDir string) error
	Close() error
}

type client struct {
	client *storage.Client
	logger *slog.Logger
}

func NewClient(ctx context.Context, logger *slog.Logger) (Client, error) {
	storageClient, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	return &client{client: storageClient, logger: logger}, nil
}

func (s client) Upload(ctx context.Context, bucketPath, localDir string) error {
	bucketName, folderPrefix := parseGCSPath(bucketPath)
	if bucketName == "" || folderPrefix == "" {
		return fmt.Errorf("invalid bucket_path: %s", bucketPath)
	}

	// Ensure the folder prefix ends with a slash
	if !strings.HasSuffix(folderPrefix, "/") {
		folderPrefix += "/"
	}

	bucket := s.client.Bucket(bucketName)

	return filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// relPath should be a fileName
		relPath, err := filepath.Rel(localDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		objectName := folderPrefix + relPath

		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", path, err)
		}
		defer f.Close()

		obj := bucket.Object(objectName)
		w := obj.NewWriter(ctx)
		if _, err = io.Copy(w, f); err != nil {
			return fmt.Errorf("failed to copy file contents to GCS: %w", err)
		}
		if err := w.Close(); err != nil {
			return fmt.Errorf("failed to close GCS writer: %w", err)
		}

		return nil
	})
}

// Download only the required ZIP files (`metrics_config.zip` and `publisher_config.zip`) and save them locally.
func (s client) Download(ctx context.Context, filePath string) error {
	bucketName, folderPrefix := parseGCSPath(filePath)
	if bucketName == "" || folderPrefix == "" {
		return fmt.Errorf("invalid file_path: %s", filePath)
	}

	// Ensure folderPrefix ends with a slash
	if !strings.HasSuffix(folderPrefix, "/") {
		folderPrefix += "/"
	}

	bucket := s.client.Bucket(bucketName)

	// Define the names of the required ZIP files
	requiredFiles := []string{"metrics_config.zip", "publisher_config.zip"}

	// Iterate through the required files and download each
	for _, fileName := range requiredFiles {
		sourceObjectPath := folderPrefix + fileName
		localZipPath := filepath.Join("downloads", fileName)

		// Step 1: Download the ZIP file
		err := s.downloadFile(ctx, bucket, sourceObjectPath, localZipPath)
		if err != nil {
			return fmt.Errorf("failed to download file %s: %w", fileName, err)
		}
		s.logger.Info("Downloaded file", "source", sourceObjectPath, "destination", localZipPath)

		// Step 2: Extract the ZIP file into a directory with the same name (without ".zip")
		destDir := strings.TrimSuffix(localZipPath, ".zip")
		err = unzip(localZipPath, destDir)
		if err != nil {
			return fmt.Errorf("failed to extract ZIP file %s: %w", fileName, err)
		}
		s.logger.Info("Extracted zip file", "source", localZipPath, "destination", destDir)

		// Step 3 (Optional): Remove the ZIP file after extraction
		err = os.Remove(localZipPath)
		if err != nil {
			return fmt.Errorf("failed to remove ZIP file %s: %w", localZipPath, err)
		}
		s.logger.Info("Removed zip file after extraction", "path", localZipPath)
	}
	return nil
}

func (s client) Close() error {
	err := s.client.Close()
	return err
}

func parseGCSPath(path string) (string, string) {
	// Remove gs:// prefix if present
	path = strings.TrimPrefix(path, "gs://")

	// Split the path into bucket and object name
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}

	return parts[0], parts[1]
}

// Downloads a single file from Google Cloud Storage to a local file.
func (s client) downloadFile(ctx context.Context, bucket *storage.BucketHandle, objectPath, localPath string) error {
	s.logger.Info("Downloading file", "source", objectPath, "destination", localPath)

	// Create a reader for the GCS object
	object := bucket.Object(objectPath)
	reader, err := object.NewReader(ctx)
	if err != nil {
		return fmt.Errorf("error creating reader for object %s: %w", objectPath, err)
	}
	defer reader.Close()

	// Create the local file where the object will be downloaded
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("error creating local directory: %w", err)
	}

	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("error creating local file %s: %w", localPath, err)
	}
	defer localFile.Close()

	// Copy the object's contents to the local file
	if _, err := io.Copy(localFile, reader); err != nil {
		return fmt.Errorf("error copying object content to file %s: %w", localPath, err)
	}

	return nil
}

func unzip(zipFilePath, destDir string) error {
	r, err := zip.OpenReader(zipFilePath)
	if err != nil {
		return fmt.Errorf("error opening zip reader: %w, file = %s", err, zipFilePath)
	}
	defer r.Close()

	for _, f := range r.File {
		err := extractAndWriteFile(destDir, f)
		if err != nil {
			return fmt.Errorf("error extracting and writing file from zip: %w", err)
		}
	}

	return nil
}

func extractAndWriteFile(destDir string, f *zip.File) error {
	// Clean and resolve absolute paths for comparison
	absDestDir, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for destination dir: %w", err)
	}

	// Resolve the destination path of the extracted file
	absPath, err := filepath.Abs(filepath.Join(destDir, f.Name))
	if err != nil {
		return fmt.Errorf("failed to get absolute path for file: %w", err)
	}

	// Prevent zip slip vulnerability by checking the file path
	if !strings.HasPrefix(absPath, absDestDir+string(os.PathSeparator)) {
		return fmt.Errorf("illegal file path in zip: %s", f.Name)
	}

	// Handle directories in the zip file
	if f.FileInfo().IsDir() {
		if err := os.MkdirAll(absPath, f.Mode()); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", absPath, err)
		}
		return nil
	}

	// Open the file within the zip archive
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("error opening zipped file: %w", err)
	}
	defer rc.Close()

	// Create parent directories for the file
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return fmt.Errorf("error creating directory for file %s: %w", absPath, err)
	}

	// Ensure we do not overwrite symlinks
	destFileInfo, err := os.Lstat(absPath)
	if err == nil && destFileInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to overwrite symlink: %s", absPath)
	}

	// Create the destination file
	outFile, err := os.Create(absPath)
	if err != nil {
		return fmt.Errorf("error creating file %s: %w", absPath, err)
	}
	defer outFile.Close()

	// Copy the content from the zipped file to the destination file
	if _, err := io.Copy(outFile, rc); err != nil {
		return fmt.Errorf("failed to write file contents to %s: %w", absPath, err)
	}

	return nil
}
