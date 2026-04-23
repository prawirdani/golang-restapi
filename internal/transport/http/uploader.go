package http

import (
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path"
	"strings"

	"github.com/prawirdani/golang-restapi/pkg/log"
)

var (
	ImageMIMEs = []string{"image/jpeg", "image/png", "image/gif", "image/webp"}
	ImageExts  = []string{".jpg", ".jpeg", ".png", ".gif", ".webp"}
)

var ErrUploader = &Error{
	Message: "invalid file uploaded",
	Code:    "FILE_UPLOAD",
	Details: nil,
	status:  http.StatusBadRequest,
}

type ParsedFile struct {
	header      *multipart.FileHeader
	filename    string
	size        int64
	contentType string
	file        multipart.File // opened lazily
}

// NewParsedFile constructs ParsedFile metadata (does not open file yet).
func NewParsedFile(fh *multipart.FileHeader) *ParsedFile {
	if fh == nil {
		return emptyFile()
	}

	return &ParsedFile{
		header:      fh,
		filename:    fh.Filename,
		size:        fh.Size,
		contentType: fh.Header.Get("Content-Type"),
	}
}

// Open lazily opens the underlying multipart file.
// Safe to call multiple times; closes any previously opened handle.
func (pf *ParsedFile) Open() error {
	if pf.header == nil {
		return fmt.Errorf("no file available")
	}

	// Close existing file if open
	if pf.file != nil {
		if err := pf.file.Close(); err != nil {
			return fmt.Errorf("failed to close previous file: %w", err)
		}
	}

	f, err := pf.header.Open()
	if err != nil {
		pf.file = nil // Ensure consistent state
		return err
	}
	pf.file = f
	return nil
}

// Close closes the underlying file if opened.
func (pf *ParsedFile) Close() error {
	if pf.file != nil {
		err := pf.file.Close()
		pf.file = nil
		return err
	}
	return nil
}

// Read implements io.Reader.
func (pf *ParsedFile) Read(p []byte) (int, error) {
	if pf.file == nil {
		if err := pf.Open(); err != nil {
			return 0, err
		}
	}
	return pf.file.Read(p)
}

// Seek implements io.Seeker, if supported.
func (pf *ParsedFile) Seek(offset int64, whence int) (int64, error) {
	if pf.file == nil {
		if err := pf.Open(); err != nil {
			return 0, err
		}
	}
	return pf.file.Seek(offset, whence)
}

// Name implements storage.File.
func (pf *ParsedFile) Name() string {
	return pf.filename
}

// SetName implements storage.File.
func (pf *ParsedFile) SetName(name string) error {
	if name == "" {
		return fmt.Errorf("filename cannot be empty")
	}
	// Strip any existing extension and add the current one
	base := strings.TrimSuffix(name, path.Ext(name))
	pf.filename = base + pf.Ext()
	return nil
}

// Ext implements storage.File.
func (pf *ParsedFile) Ext() string {
	return path.Ext(pf.filename)
}

// Size implements storage.File.
func (pf *ParsedFile) Size() int64 {
	return pf.size
}

// ContentType implements storage.File.
func (pf *ParsedFile) ContentType() string {
	return pf.contentType
}

// NoFile implements storage.File.
func (pf *ParsedFile) NoFile() bool {
	return pf.header == nil
}

// Header exposes the raw multipart header (optional).
func (pf *ParsedFile) Header() *multipart.FileHeader {
	return pf.header
}

// emptyFile produced no file for ParsedFile
func emptyFile() *ParsedFile {
	return &ParsedFile{
		filename: "",
		size:     0,
	}
}

type ValidationRules struct {
	MaxSize      int64
	AllowedMIMEs []string
}

// Check if MIME type is in allowed list
func (r ValidationRules) isMIMEAllowed(mime string) bool {
	mimeBase := strings.Split(mime, ";")[0]
	for _, allowed := range r.AllowedMIMEs {
		if strings.EqualFold(mimeBase, allowed) {
			return true
		}
	}
	return false
}

// ValidateFile validates a parsed and prepared file based on the given rules.
// Performs checks in order of cost: existence → size → MIME type → extension verification.
func ValidateFile(ctx context.Context, f *ParsedFile, rules ValidationRules) error {
	// Existence check
	if f == nil || f.NoFile() {
		return fmt.Errorf("no file provided")
	}

	// Size check (cheap)
	if rules.MaxSize > 0 && f.Size() > rules.MaxSize {
		return ErrUploader.SetMessage("file size exceeds maximum allowed").SetDetails(
			map[string]any{
				"max_bytes": rules.MaxSize,
				"received":  f.size,
			},
		)
	}

	// MIME type validation (primary security check)
	if len(rules.AllowedMIMEs) > 0 {
		claimedType := f.ContentType() // From headers - UNTRUSTED
		actualType, err := detectMIME(f)
		if err != nil {
			return fmt.Errorf("failed to detect MIME type: %w", err)
		}

		// SPECIAL CASE: Skip MIME mismatch check for Office documents
		// since they are ZIP containers and often detected as application/zip
		if isOfficeDocument(f.Ext(), actualType) {
			// For Office documents, we trust the extension and claimed type
			// but still validate that the actual type is either ZIP or the expected Office MIME
			if !rules.isMIMEAllowed(claimedType) && !rules.isMIMEAllowed(actualType) {
				return ErrUploader.SetMessage("file type is not allowed").SetDetails(
					map[string]any{"allowed_mimes": rules.AllowedMIMEs},
				)
			}
			// Office document passed special validation
			return nil
		}

		// SECURITY CHECK: Verify claimed type matches actual type
		if claimedType != "" && !mimeTypesMatch(claimedType, actualType) {
			log.WarnCtx(
				ctx,
				"MIME type mismatch, possible malicious upload",
				"claimed",
				claimedType,
				"actual",
				actualType,
			)
			return ErrUploader.SetMessage("invalid file").SetDetails("what a sus file")
		}

		// Check if actual type is allowed
		if !rules.isMIMEAllowed(actualType) {
			return ErrUploader.SetMessage("file type is not allowed").SetDetails(
				map[string]any{"actual": actualType, "allowed_mimes": rules.AllowedMIMEs},
			)
		}

		// EXTENSION VERIFICATION: Ensure extension matches detected MIME type
		if err := verifyExtensionMatchesMIME(f.Ext(), actualType); err != nil {
			return fmt.Errorf("file extension validation failed: %w", err)
		}
	}

	return nil
}

// verifyExtensionMatchesMIME ensures the file extension matches the detected MIME type
func verifyExtensionMatchesMIME(ext, detectedMIME string) error {
	expectedExtensions := getExpectedExtensions(detectedMIME)
	if len(expectedExtensions) == 0 {
		// No expected extensions for this MIME type, skip verification
		return nil
	}

	for _, expectedExt := range expectedExtensions {
		if strings.EqualFold(ext, expectedExt) {
			return nil // Extension matches
		}
	}

	return fmt.Errorf(
		"extension '%s' does not match detected MIME type '%s' (expected extensions: %v)",
		ext, detectedMIME, expectedExtensions,
	)
}

// getExpectedExtensions returns file extensions for a given MIME type using Go's mime package
func getExpectedExtensions(mimeType string) []string {
	mimeBase := strings.Split(mimeType, ";")[0]

	// Use Go's built-in MIME type to extension mapping
	extensions, err := mime.ExtensionsByType(mimeBase)
	if err != nil {
		// getCommonExtensionsFallback provides extensions for common types not fully covered by mime package
		// Common web formats that might not be fully covered by mime package
		fallback := map[string][]string{
			"image/webp":               {".webp"},
			"image/svg+xml":            {".svg"},
			"application/wasm":         {".wasm"},
			"font/woff":                {".woff"},
			"font/woff2":               {".woff2"},
			"application/octet-stream": {}, // Skip validation for binary files
		}

		if exts, exists := fallback[mimeBase]; exists {
			return exts
		}
		return nil
	}

	return extensions
}

// detectMIME detects the MIME type of the file content and resets the file pointer.
func detectMIME(f *ParsedFile) (string, error) {
	originalOffset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return "", fmt.Errorf("failed to get current position: %w", err)
	}

	// Read first 512 bytes for MIME detection
	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}

	// Reset position
	_, err = f.Seek(originalOffset, io.SeekStart)
	if err != nil {
		return "", fmt.Errorf("failed to reset file position: %w", err)
	}

	// For empty files, return generic MIME type
	if n == 0 {
		return "application/octet-stream", nil
	}

	return http.DetectContentType(buf[:n]), nil
}

// Helper function to compare MIME types
func mimeTypesMatch(claimed, actual string) bool {
	// Normalize by removing parameters
	claimedBase := strings.Split(claimed, ";")[0]
	actualBase := strings.Split(actual, ";")[0]

	return strings.EqualFold(strings.TrimSpace(claimedBase), strings.TrimSpace(actualBase))
}

// isOfficeDocument checks if the file is a modern Office document
// that might be detected as application/zip due to its container format
func isOfficeDocument(ext, actualType string) bool {
	officeExtensions := map[string][]string{
		".xlsx": {
			"application/zip",
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			"application/octet-stream",
		},
		".docx": {
			"application/zip",
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			"application/octet-stream",
		},
		".pptx": {
			"application/zip",
			"application/vnd.openxmlformats-officedocument.presentationml.presentation",
			"application/octet-stream",
		},
	}

	allowedMimes, exists := officeExtensions[strings.ToLower(ext)]
	if !exists {
		return false
	}

	// Check if the actual detected type is expected for this Office extension
	for _, allowedMime := range allowedMimes {
		if strings.Contains(actualType, allowedMime) {
			return true
		}
	}

	return false
}
