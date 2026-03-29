package security

import "regexp"

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-ant-[a-zA-Z0-9_-]{10,}`),
	regexp.MustCompile(`sk-[a-zA-Z0-9]{20,}`),
	regexp.MustCompile(`AKIA[A-Z0-9]{12,}`),
	regexp.MustCompile(`ghp_[a-zA-Z0-9]{36,}`),
	regexp.MustCompile(`gho_[a-zA-Z0-9]{36,}`),
	regexp.MustCompile(`-----BEGIN[A-Z ]*KEY-----`),
	regexp.MustCompile(`Bearer\s+[a-zA-Z0-9._-]{20,}`),
	regexp.MustCompile(`(?i)password\s*[=:]\s*\S+`),
	regexp.MustCompile(`(?i)secret\s*[=:]\s*\S+`),
}

// RedactSecrets replaces potential secrets in text with [REDACTED].
func RedactSecrets(text string) string {
	for _, p := range secretPatterns {
		text = p.ReplaceAllString(text, "[REDACTED]")
	}
	return text
}
