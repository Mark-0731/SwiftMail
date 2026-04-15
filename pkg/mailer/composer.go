package mailer

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Message represents an email message to be composed into MIME format.
type Message struct {
	From        string
	To          string
	ReplyTo     string
	Subject     string
	HTMLBody    string
	TextBody    string
	MessageID   string
	Headers     map[string]string
	Attachments []Attachment
}

// Attachment represents a file attached to an email.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// Compose builds a complete MIME message from the Message struct.
func Compose(msg *Message) ([]byte, error) {
	if msg.MessageID == "" {
		msg.MessageID = fmt.Sprintf("<%s@swiftmail>", uuid.New().String())
	}

	var buf bytes.Buffer

	// Write standard headers
	buf.WriteString(fmt.Sprintf("From: %s\r\n", msg.From))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", msg.To))
	if msg.ReplyTo != "" {
		buf.WriteString(fmt.Sprintf("Reply-To: %s\r\n", msg.ReplyTo))
	}
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", mime.QEncoding.Encode("utf-8", msg.Subject)))
	buf.WriteString(fmt.Sprintf("Message-ID: %s\r\n", msg.MessageID))
	buf.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z)))
	buf.WriteString("MIME-Version: 1.0\r\n")

	// Write custom headers
	for k, v := range msg.Headers {
		buf.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}

	hasAttachments := len(msg.Attachments) > 0
	hasText := msg.TextBody != ""
	hasHTML := msg.HTMLBody != ""

	if hasAttachments {
		return composeMixed(&buf, msg), nil
	}

	if hasText && hasHTML {
		return composeAlternative(&buf, msg), nil
	}

	if hasHTML {
		buf.WriteString("Content-Type: text/html; charset=utf-8\r\n")
		buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		buf.WriteString(msg.HTMLBody)
	} else {
		buf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
		buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		buf.WriteString(msg.TextBody)
	}

	return buf.Bytes(), nil
}

func composeAlternative(buf *bytes.Buffer, msg *Message) []byte {
	w := multipart.NewWriter(buf)
	boundary := w.Boundary()

	buf.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", boundary))

	// Text part
	textHeader := textproto.MIMEHeader{}
	textHeader.Set("Content-Type", "text/plain; charset=utf-8")
	textHeader.Set("Content-Transfer-Encoding", "quoted-printable")
	part, _ := w.CreatePart(textHeader)
	part.Write([]byte(msg.TextBody))

	// HTML part
	htmlHeader := textproto.MIMEHeader{}
	htmlHeader.Set("Content-Type", "text/html; charset=utf-8")
	htmlHeader.Set("Content-Transfer-Encoding", "quoted-printable")
	part, _ = w.CreatePart(htmlHeader)
	part.Write([]byte(msg.HTMLBody))

	w.Close()
	return buf.Bytes()
}

func composeMixed(buf *bytes.Buffer, msg *Message) []byte {
	w := multipart.NewWriter(buf)
	boundary := w.Boundary()

	buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", boundary))

	// Body part (alternative if both text and HTML)
	if msg.TextBody != "" && msg.HTMLBody != "" {
		altHeader := textproto.MIMEHeader{}
		altW := multipart.NewWriter(nil)
		altBoundary := altW.Boundary()
		altHeader.Set("Content-Type", fmt.Sprintf("multipart/alternative; boundary=\"%s\"", altBoundary))
		part, _ := w.CreatePart(altHeader)

		// Text
		var altBuf bytes.Buffer
		altBuf.WriteString(fmt.Sprintf("--%s\r\n", altBoundary))
		altBuf.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
		altBuf.WriteString(msg.TextBody)
		altBuf.WriteString(fmt.Sprintf("\r\n--%s\r\n", altBoundary))
		altBuf.WriteString("Content-Type: text/html; charset=utf-8\r\n\r\n")
		altBuf.WriteString(msg.HTMLBody)
		altBuf.WriteString(fmt.Sprintf("\r\n--%s--\r\n", altBoundary))
		part.Write(altBuf.Bytes())
	} else if msg.HTMLBody != "" {
		htmlHeader := textproto.MIMEHeader{}
		htmlHeader.Set("Content-Type", "text/html; charset=utf-8")
		part, _ := w.CreatePart(htmlHeader)
		part.Write([]byte(msg.HTMLBody))
	} else {
		textHeader := textproto.MIMEHeader{}
		textHeader.Set("Content-Type", "text/plain; charset=utf-8")
		part, _ := w.CreatePart(textHeader)
		part.Write([]byte(msg.TextBody))
	}

	// Attachments
	for _, a := range msg.Attachments {
		ct := a.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		h := textproto.MIMEHeader{}
		h.Set("Content-Type", ct)
		h.Set("Content-Transfer-Encoding", "base64")
		h.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", a.Filename))
		part, _ := w.CreatePart(h)

		encoded := base64.StdEncoding.EncodeToString(a.Data)
		for i := 0; i < len(encoded); i += 76 {
			end := i + 76
			if end > len(encoded) {
				end = len(encoded)
			}
			part.Write([]byte(encoded[i:end] + "\r\n"))
		}
	}

	w.Close()
	return buf.Bytes()
}

// ExtractDomain extracts the domain part from an email address.
func ExtractDomain(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.ToLower(parts[1])
}
