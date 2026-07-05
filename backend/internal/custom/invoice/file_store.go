package invoice

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const pdfMagic = "%PDF-"

// FileStore validates, stores and opens invoice PDF files.
type FileStore struct {
	dataDir string
	now     func() time.Time
}

// NewFileStore creates a filesystem-backed invoice file store.
func NewFileStore(dataDir string) (*FileStore, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return nil, errors.New("invoice file store requires data dir")
	}
	return &FileStore{dataDir: dataDir, now: func() time.Time { return time.Now().UTC() }}, nil
}

// SavePDF validates and stores one uploaded PDF file for an issued application.
func (s *FileStore) SavePDF(ctx context.Context, applicationID int64, header *multipart.FileHeader) (StoredFile, error) {
	if s == nil || strings.TrimSpace(s.dataDir) == "" || applicationID <= 0 || header == nil {
		return StoredFile{}, ErrInvalidInput
	}
	if header.Size <= 0 || header.Size > MaxPDFSizeBytes {
		return StoredFile{}, ErrInvalidFile
	}
	if !strings.EqualFold(filepath.Ext(header.Filename), ".pdf") {
		return StoredFile{}, ErrInvalidFile
	}
	src, err := header.Open()
	if err != nil {
		return StoredFile{}, fmt.Errorf("open invoice PDF: %w", err)
	}
	defer src.Close()

	head := make([]byte, len(pdfMagic))
	if _, err := io.ReadFull(src, head); err != nil {
		return StoredFile{}, ErrInvalidFile
	}
	if !bytes.Equal(head, []byte(pdfMagic)) {
		return StoredFile{}, ErrInvalidFile
	}

	now := s.now()
	relDir := filepath.Join("custom", "invoices", now.Format("2006"), now.Format("01"))
	absDir := filepath.Join(s.dataDir, relDir)
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return StoredFile{}, fmt.Errorf("create invoice file dir: %w", err)
	}
	token, err := randomHex(12)
	if err != nil {
		return StoredFile{}, fmt.Errorf("generate invoice file name: %w", err)
	}
	fileName := fmt.Sprintf("%d-%s.pdf", applicationID, token)
	objectKey := filepath.ToSlash(filepath.Join(relDir, fileName))
	absPath := filepath.Join(s.dataDir, filepath.FromSlash(objectKey))

	dst, err := os.OpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return StoredFile{}, fmt.Errorf("create invoice PDF: %w", err)
	}
	committed := false
	defer func() {
		_ = dst.Close()
		if !committed {
			_ = os.Remove(absPath)
		}
	}()

	if _, err := dst.Write(head); err != nil {
		return StoredFile{}, fmt.Errorf("write invoice PDF header: %w", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		return StoredFile{}, fmt.Errorf("write invoice PDF: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return StoredFile{}, err
	}
	committed = true
	return StoredFile{ObjectKey: objectKey, OriginalName: filepath.Base(header.Filename), Size: header.Size, Path: absPath}, nil
}

// Open returns a readable PDF file handle after path traversal validation.
func (s *FileStore) Open(objectKey string) (*os.File, string, error) {
	if s == nil || strings.TrimSpace(s.dataDir) == "" {
		return nil, "", ErrInvalidInput
	}
	objectKey = filepath.ToSlash(strings.TrimSpace(objectKey))
	if objectKey == "" || strings.Contains(objectKey, "..") || filepath.IsAbs(objectKey) {
		return nil, "", ErrInvalidFile
	}
	absPath := filepath.Join(s.dataDir, filepath.FromSlash(objectKey))
	cleanDataDir, err := filepath.Abs(s.dataDir)
	if err != nil {
		return nil, "", fmt.Errorf("resolve invoice data dir: %w", err)
	}
	cleanPath, err := filepath.Abs(absPath)
	if err != nil {
		return nil, "", fmt.Errorf("resolve invoice file: %w", err)
	}
	if !strings.HasPrefix(cleanPath, cleanDataDir+string(os.PathSeparator)) && cleanPath != cleanDataDir {
		return nil, "", ErrInvalidFile
	}
	file, err := os.Open(cleanPath)
	if err != nil {
		return nil, "", fmt.Errorf("open invoice file: %w", err)
	}
	return file, cleanPath, nil
}

// Remove deletes one stored file during rollback after database update failure.
func (s *FileStore) Remove(objectKey string) {
	if s == nil || strings.TrimSpace(objectKey) == "" {
		return
	}
	if file, path, err := s.Open(objectKey); err == nil {
		_ = file.Close()
		_ = os.Remove(path)
	}
}

func randomHex(n int) (string, error) {
	if n <= 0 {
		return "", ErrInvalidInput
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
