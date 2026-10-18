// Package storage provides file upload and deletion abstractions.
package storage

import (
	"context"
	"io"
)

// Service uploads and deletes files. UploadFile stores file and returns its URL or path;
// DeleteFile removes a previously uploaded file given the URL UploadFile returned.
type Service interface {
	UploadFile(ctx context.Context, file io.Reader, originalFilename string) (string, error)
	DeleteFile(ctx context.Context, fileURL string) error
}
