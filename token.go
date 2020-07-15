package hub

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// create Tokenizer interface
type Tokenizer interface {
	Tokenize(username string) string
}

// TokenizerFunc satisfies the Tokenizer interface.
// TokenizerFunc is a Tokenizer.
type TokenizerFunc func(username string) string

// Tokenize satisfies the Tokenizer interface, that is,
// TokenizerFunc is now a Tokenizer.
func (t TokenizerFunc) Tokenize(username string) string {
	return t(username)
}

// HmacSha256Tokenizer is an implementation of Tokenizer
func HmacSha256Tokenizer(token string) Tokenizer {
	return TokenizerFunc(func(username) string {
		hasher := hmac.New(sha256.New, []byte(secret))
		hasher.Write([]byte(username))
		return hex.EncodeToString(hasher.Sum(nil))
	})
}
