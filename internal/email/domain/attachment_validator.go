package domain

import (
	"fmt"
	"mime"
	"path/filepath"
	"strings"
)

// AttachmentData represents attachment data (duplicated to avoid import cycle).
type AttachmentData struct {
	Filename    string
	ContentType string
	Data        []byte
	Size        int64
}

const (
	// Size limits
	MaxAttachmentSize      = 25 * 1024 * 1024 // 25MB per attachment
	MaxTotalAttachmentSize = 50 * 1024 * 1024 // 50MB total
	MaxAttachmentCount     = 10               // Max 10 attachments

	// Filename limits
	MaxFilenameLength = 255
)

var (
	// Allowed MIME types
	allowedMimeTypes = map[string]bool{
		// Documents
		"application/pdf":    true,
		"application/msword": true,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
		"application/vnd.ms-excel": true,
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
		"application/vnd.ms-powerpoint":                                             true,
		"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
		"text/plain":      true,
		"text/csv":        true,
		"application/rtf": true,

		// Images
		"image/jpeg":    true,
		"image/png":     true,
		"image/gif":     true,
		"image/webp":    true,
		"image/svg+xml": true,

		// Archives
		"application/zip":              true,
		"application/x-rar-compressed": true,
		"application/x-7z-compressed":  true,
		"application/gzip":             true,

		// Other
		"application/json": true,
		"application/xml":  true,
		"text/xml":         true,
	}

	// Dangerous file extensions
	dangerousExtensions = map[string]bool{
		".exe": true, ".bat": true, ".cmd": true, ".com": true, ".pif": true,
		".scr": true, ".vbs": true, ".js": true, ".jar": true, ".app": true,
		".deb": true, ".pkg": true, ".rpm": true, ".dmg": true, ".iso": true,
		".msi": true, ".dll": true, ".sys": true, ".drv": true, ".bin": true,
	}
)

// AttachmentValidationResult contains validation results for attachments.
type AttachmentValidationResult struct {
	Valid              bool
	Reason             string
	TotalSize          int64
	AttachmentCount    int
	InvalidAttachments []string
}

// AttachmentValidator validates email attachments.
type AttachmentValidator struct{}

// NewAttachmentValidator creates a new attachment validator.
func NewAttachmentValidator() *AttachmentValidator {
	return &AttachmentValidator{}
}

// ValidateAttachments validates all attachments in a request.
func (av *AttachmentValidator) ValidateAttachments(attachments []AttachmentData) *AttachmentValidationResult {
	result := &AttachmentValidationResult{
		Valid:              true,
		InvalidAttachments: []string{},
	}

	// Check attachment count
	result.AttachmentCount = len(attachments)
	if result.AttachmentCount > MaxAttachmentCount {
		result.Valid = false
		result.Reason = fmt.Sprintf("too many attachments (max %d)", MaxAttachmentCount)
		return result
	}

	// Validate each attachment
	for i, attachment := range attachments {
		// Set size if not provided
		if attachment.Size == 0 {
			attachment.Size = int64(len(attachment.Data))
		}

		// Validate individual attachment
		if err := av.validateSingleAttachment(attachment); err != nil {
			result.Valid = false
			result.InvalidAttachments = append(result.InvalidAttachments,
				fmt.Sprintf("attachment %d (%s): %s", i+1, attachment.Filename, err.Error()))
		}

		result.TotalSize += attachment.Size
	}

	// Check total size
	if result.TotalSize > MaxTotalAttachmentSize {
		result.Valid = false
		result.Reason = fmt.Sprintf("total attachment size exceeds limit (max %d MB)", MaxTotalAttachmentSize/(1024*1024))
		return result
	}

	// Set reason if there are invalid attachments
	if len(result.InvalidAttachments) > 0 {
		result.Reason = "one or more attachments are invalid"
	}

	return result
}

// validateSingleAttachment validates a single attachment.
func (av *AttachmentValidator) validateSingleAttachment(attachment AttachmentData) error {
	// Check filename
	if len(attachment.Filename) == 0 {
		return fmt.Errorf("filename is required")
	}

	if len(attachment.Filename) > MaxFilenameLength {
		return fmt.Errorf("filename too long (max %d characters)", MaxFilenameLength)
	}

	// Check for dangerous extensions
	ext := strings.ToLower(filepath.Ext(attachment.Filename))
	if dangerousExtensions[ext] {
		return fmt.Errorf("file type not allowed: %s", ext)
	}

	// Check size
	if attachment.Size > MaxAttachmentSize {
		return fmt.Errorf("file too large (max %d MB)", MaxAttachmentSize/(1024*1024))
	}

	if attachment.Size == 0 {
		return fmt.Errorf("empty file not allowed")
	}

	// Check MIME type
	if !allowedMimeTypes[attachment.ContentType] {
		return fmt.Errorf("content type not allowed: %s", attachment.ContentType)
	}

	// Validate MIME type matches extension
	expectedMimeType := mime.TypeByExtension(ext)
	if expectedMimeType != "" && expectedMimeType != attachment.ContentType {
		return fmt.Errorf("content type mismatch: expected %s, got %s", expectedMimeType, attachment.ContentType)
	}

	// Check data
	if len(attachment.Data) == 0 {
		return fmt.Errorf("attachment data is empty")
	}

	// Verify size matches data length
	if attachment.Size != int64(len(attachment.Data)) {
		return fmt.Errorf("size mismatch: declared %d bytes, actual %d bytes", attachment.Size, len(attachment.Data))
	}

	return nil
}

// IsAllowedMimeType checks if a MIME type is allowed.
func (av *AttachmentValidator) IsAllowedMimeType(mimeType string) bool {
	return allowedMimeTypes[mimeType]
}

// IsDangerousExtension checks if a file extension is dangerous.
func (av *AttachmentValidator) IsDangerousExtension(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return dangerousExtensions[ext]
}

// AddAllowedMimeType adds a MIME type to the allowed list.
func AddAllowedMimeType(mimeType string) {
	allowedMimeTypes[mimeType] = true
}

// RemoveAllowedMimeType removes a MIME type from the allowed list.
func RemoveAllowedMimeType(mimeType string) {
	delete(allowedMimeTypes, mimeType)
}

// AddDangerousExtension adds an extension to the dangerous list.
func AddDangerousExtension(ext string) {
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	dangerousExtensions[strings.ToLower(ext)] = true
}

// RemoveDangerousExtension removes an extension from the dangerous list.
func RemoveDangerousExtension(ext string) {
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	delete(dangerousExtensions, strings.ToLower(ext))
}
