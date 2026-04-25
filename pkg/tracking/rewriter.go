package tracking

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var hrefRegex = regexp.MustCompile(`(?i)href\s*=\s*["']([^"']+)["']`)

// RewriteLinks rewrites all links in HTML content for click tracking.
func RewriteLinks(html string, emailLogID string, baseURL string) string {
	return hrefRegex.ReplaceAllStringFunc(html, func(match string) string {
		submatches := hrefRegex.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}

		originalURL := submatches[1]

		// Skip mailto:, tel:, and anchor links
		if strings.HasPrefix(originalURL, "mailto:") ||
			strings.HasPrefix(originalURL, "tel:") ||
			strings.HasPrefix(originalURL, "#") ||
			strings.HasPrefix(originalURL, "{{") {
			return match
		}

		// Build tracking URL
		trackingURL := fmt.Sprintf("%s/t/c/%s?url=%s",
			strings.TrimRight(baseURL, "/"),
			emailLogID,
			url.QueryEscape(originalURL),
		)

		return fmt.Sprintf(`href="%s"`, trackingURL)
	})
}

// InjectPixel adds the open tracking pixel before the closing </body> tag.
func InjectPixel(html string, emailLogID string, baseURL string) string {
	pixelTag := fmt.Sprintf(
		`<img src="%s/t/o/%s" width="1" height="1" style="display:none" alt="" />`,
		strings.TrimRight(baseURL, "/"),
		emailLogID,
	)

	if strings.Contains(html, "</body>") {
		return strings.Replace(html, "</body>", pixelTag+"</body>", 1)
	}

	return html + pixelTag
}
