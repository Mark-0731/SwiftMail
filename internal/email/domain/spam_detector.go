package domain

import (
	"regexp"
	"strings"
)

// SpamDetector analyzes email content for spam indicators.
type SpamDetector struct {
	spamWords    []string
	spamPatterns []*regexp.Regexp
}

// SpamScore represents the spam analysis result.
type SpamScore struct {
	Score     int      // 0-100, higher = more likely spam
	Reasons   []string // Why it was flagged
	IsSpam    bool     // True if score > threshold
	Threshold int      // Spam threshold used
}

// NewSpamDetector creates a new spam detector.
func NewSpamDetector() *SpamDetector {
	spamWords := []string{
		// Financial spam
		"free money", "make money fast", "get rich quick", "guaranteed income",
		"cash bonus", "prize", "winner", "congratulations", "lottery",

		// Urgency/pressure
		"urgent", "immediate", "act now", "limited time", "expires today",
		"don't delay", "hurry", "rush", "instant",

		// Suspicious claims
		"100% free", "no cost", "risk free", "guaranteed", "amazing deal",
		"incredible offer", "once in lifetime", "exclusive deal",

		// Medical/adult
		"lose weight", "viagra", "cialis", "enlargement", "enhancement",

		// Suspicious requests
		"click here", "click below", "visit our website", "call now",
		"subscribe", "unsubscribe here", "remove from list",

		// Cryptocurrency/investment
		"bitcoin", "cryptocurrency", "investment opportunity", "trading",
		"forex", "stocks", "profit guaranteed",
	}

	spamPatterns := []*regexp.Regexp{
		// Multiple exclamation marks
		regexp.MustCompile(`!{3,}`),
		// All caps words (5+ chars)
		regexp.MustCompile(`\b[A-Z]{5,}\b`),
		// Excessive use of dollar signs
		regexp.MustCompile(`\${2,}`),
		// Phone numbers with suspicious patterns
		regexp.MustCompile(`\b1-?800-?\d{3}-?\d{4}\b`),
		// Suspicious URLs
		regexp.MustCompile(`bit\.ly|tinyurl|t\.co|goo\.gl`),
		// Email addresses in content (suspicious)
		regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
	}

	return &SpamDetector{
		spamWords:    spamWords,
		spamPatterns: spamPatterns,
	}
}

// AnalyzeContent analyzes email content for spam indicators.
func (sd *SpamDetector) AnalyzeContent(subject, htmlBody, textBody string) *SpamScore {
	score := &SpamScore{
		Score:     0,
		Reasons:   []string{},
		Threshold: 70, // Configurable threshold
	}

	// Combine all content for analysis
	content := strings.ToLower(subject + " " + htmlBody + " " + textBody)

	// 1. Check for spam words
	for _, word := range sd.spamWords {
		if strings.Contains(content, strings.ToLower(word)) {
			score.Score += 10
			score.Reasons = append(score.Reasons, "contains spam word: "+word)
		}
	}

	// 2. Check for spam patterns
	for _, pattern := range sd.spamPatterns {
		if pattern.MatchString(content) {
			score.Score += 15
			score.Reasons = append(score.Reasons, "matches spam pattern: "+pattern.String())
		}
	}

	// 3. Subject line analysis
	if len(subject) > 0 {
		// All caps subject
		if subject == strings.ToUpper(subject) && len(subject) > 10 {
			score.Score += 20
			score.Reasons = append(score.Reasons, "subject line is all caps")
		}

		// Too many exclamation marks in subject
		exclamationCount := strings.Count(subject, "!")
		if exclamationCount > 2 {
			score.Score += exclamationCount * 5
			score.Reasons = append(score.Reasons, "excessive exclamation marks in subject")
		}

		// Subject too long
		if len(subject) > 100 {
			score.Score += 10
			score.Reasons = append(score.Reasons, "subject line too long")
		}
	}

	// 4. Content analysis
	if len(htmlBody) > 0 {
		// Too many links
		linkCount := strings.Count(strings.ToLower(htmlBody), "<a href")
		if linkCount > 10 {
			score.Score += linkCount * 2
			score.Reasons = append(score.Reasons, "too many links in content")
		}

		// Hidden text (white text on white background)
		if strings.Contains(strings.ToLower(htmlBody), "color:white") ||
			strings.Contains(strings.ToLower(htmlBody), "color:#ffffff") {
			score.Score += 25
			score.Reasons = append(score.Reasons, "potential hidden text")
		}

		// Suspicious image-to-text ratio
		imgCount := strings.Count(strings.ToLower(htmlBody), "<img")
		textLength := len(strings.ReplaceAll(htmlBody, "<", ""))
		if imgCount > 5 && textLength < 200 {
			score.Score += 20
			score.Reasons = append(score.Reasons, "high image-to-text ratio")
		}
	}

	// 5. Cap the score at 100
	if score.Score > 100 {
		score.Score = 100
	}

	// 6. Determine if spam
	score.IsSpam = score.Score >= score.Threshold

	return score
}

// IsLikelySpam performs a quick spam check.
func (sd *SpamDetector) IsLikelySpam(subject, htmlBody, textBody string) bool {
	score := sd.AnalyzeContent(subject, htmlBody, textBody)
	return score.IsSpam
}

// AddSpamWord adds a word to the spam word list.
func (sd *SpamDetector) AddSpamWord(word string) {
	sd.spamWords = append(sd.spamWords, strings.ToLower(word))
}

// RemoveSpamWord removes a word from the spam word list.
func (sd *SpamDetector) RemoveSpamWord(word string) {
	word = strings.ToLower(word)
	for i, w := range sd.spamWords {
		if w == word {
			sd.spamWords = append(sd.spamWords[:i], sd.spamWords[i+1:]...)
			break
		}
	}
}
