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
	"context"
	"fmt"
	"log/slog"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

type Client interface {
	Validate(ctx context.Context, filePath string) error
	CopyToOutputs(ctx context.Context, inputfilePath string, destinationPath string) error
	Close() error
}

type client struct {
	storageClient *storage.Client
	logger        *slog.Logger
}

func NewClient(ctx context.Context, logger *slog.Logger) (Client, error) {
	storageClient, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	return &client{storageClient: storageClient, logger: logger}, nil
}

// Validate checks that `metrics_config.zip` and `publisher_config.zip` exist in the given filePath.
func (s client) Validate(ctx context.Context, filePath string) error {
	bucketName, folderPrefix := parseGCSPath(filePath)
	if bucketName == "" || folderPrefix == "" {
		return fmt.Errorf("invalid file_path: %s", filePath)
	}

	if !strings.HasSuffix(folderPrefix, "/") {
		folderPrefix += "/"
	}

	// List the objects and ensure the two required files exist
	requiredFiles := []string{"metrics_config.zip", "publisher_config.zip"}
	foundFiles := make(map[string]bool)
	bucket := s.storageClient.Bucket(bucketName)
	it := bucket.Objects(ctx, &storage.Query{
		Prefix: folderPrefix,
	})

	// Iterate through the objects to find the required files
	for {
		objAttrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("error listing objects: %w", err)
		}

		objectName := strings.TrimPrefix(objAttrs.Name, folderPrefix) // Get the file name
		for _, requiredFile := range requiredFiles {
			if objectName == requiredFile {
				foundFiles[requiredFile] = true
			}
		}
	}

	// Check if all required files are found
	for _, requiredFile := range requiredFiles {
		if !foundFiles[requiredFile] {
			return fmt.Errorf("missing required file: %s", requiredFile)
		}
	}

	s.logger.Info("All required files found", "path", filePath)
	return nil
}

// CopyToOutputs copies only `metrics_config.zip` and `publisher_config.zip` to the target path based on the provided `id`.
func (s client) CopyToOutputs(ctx context.Context, inputfilePath string, destinationPath string) error {
	// Parse the source bucket and folder prefix from inputfilePath
	sourceBucketName, sourcePrefix := parseGCSPath(inputfilePath)
	if sourceBucketName == "" || sourcePrefix == "" {
		return fmt.Errorf("invalid input_file_path: %s", inputfilePath)
	}

	// Parse the destination bucket and folder prefix from destinationPath
	destBucketName, destPrefix := parseGCSPath(destinationPath)
	if destBucketName == "" || destPrefix == "" {
		return fmt.Errorf("invalid destination_path: %s", destinationPath)
	}

	// Ensure sourcePrefix and destPrefix end with a slash
	if !strings.HasSuffix(sourcePrefix, "/") {
		sourcePrefix += "/"
	}
	if !strings.HasSuffix(destPrefix, "/") {
		destPrefix += "/"
	}

	// Only copy these specific files
	requiredFiles := []string{"metrics_config.zip", "publisher_config.zip"}

	// Get handles for the source and destination buckets
	sourceBucket := s.storageClient.Bucket(sourceBucketName)
	destBucket := s.storageClient.Bucket(destBucketName)

	for _, requiredFile := range requiredFiles {
		// Construct source file name and destination file name
		sourceObjectName := sourcePrefix + requiredFile
		destObjectName := destPrefix + requiredFile

		// Get handles for the source and destination objects
		sourceObjectHandle := sourceBucket.Object(sourceObjectName)
		destObjectHandle := destBucket.Object(destObjectName)

		// Copy the file from the source bucket to the destination bucket
		_, err := destObjectHandle.CopierFrom(sourceObjectHandle).Run(ctx)
		if err != nil {
			return fmt.Errorf("error copying file %s to %s: %w", sourceObjectName, destObjectName, err)
		}

		s.logger.Info("Successfully copied file", "source", sourceObjectName, "destination", destObjectName)
	}

	s.logger.Info("All required files successfully copied", "source", inputfilePath, "destination", destinationPath)
	return nil
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

func (s client) Close() error {
	err := s.storageClient.Close()
	return err
}
