package security

import "testing"

func TestRedactAPIKey(t *testing.T) {
	input := "token: sk-ant-abc123def456ghi789"
	got := RedactSecrets(input)
	if got == input {
		t.Error("expected sk-ant key to be redacted")
	}
	if got != "token: [REDACTED]" {
		t.Errorf("unexpected result: %q", got)
	}
}

func TestRedactAWSKey(t *testing.T) {
	input := "aws_key=AKIAIOSFODNN7EXAMPLE"
	got := RedactSecrets(input)
	if got == input {
		t.Error("expected AWS key to be redacted")
	}
}

func TestRedactGitHubToken(t *testing.T) {
	input := "gh token: ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmn"
	got := RedactSecrets(input)
	if got == input {
		t.Error("expected GitHub token to be redacted")
	}
}

func TestRedactPrivateKey(t *testing.T) {
	input := "-----BEGIN RSA PRIVATE KEY-----"
	got := RedactSecrets(input)
	if got == input {
		t.Error("expected private key header to be redacted")
	}
}

func TestNoRedactNormalText(t *testing.T) {
	input := "This is normal text with no secrets"
	got := RedactSecrets(input)
	if got != input {
		t.Errorf("normal text was modified: %q", got)
	}
}
