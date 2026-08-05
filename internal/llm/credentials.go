package llm

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

var (
	ErrCredentialKeyMissing = errors.New("LLM credential encryption key is not configured")
	ErrCredentialKeyInvalid = errors.New("LLM credential encryption key must decode to 32 bytes")
)

type CredentialBox struct {
	aead cipher.AEAD
	rand io.Reader
}

type EncryptedSecret struct {
	Ciphertext  []byte
	Nonce       []byte
	Fingerprint string
	Last4       string
}

func NewCredentialBox(encodedKey string) (*CredentialBox, error) {
	encodedKey = strings.TrimSpace(encodedKey)
	if encodedKey == "" {
		return nil, ErrCredentialKeyMissing
	}
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil || len(key) != 32 {
		return nil, ErrCredentialKeyInvalid
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("initialize credential cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize credential AEAD: %w", err)
	}
	return &CredentialBox{aead: aead, rand: rand.Reader}, nil
}

func (b *CredentialBox) Encrypt(secret string, associatedData []byte) (EncryptedSecret, error) {
	if b == nil || b.aead == nil {
		return EncryptedSecret{}, ErrCredentialKeyMissing
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return EncryptedSecret{}, errors.New("credential is empty")
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(b.rand, nonce); err != nil {
		return EncryptedSecret{}, fmt.Errorf("create credential nonce: %w", err)
	}
	sum := sha256.Sum256([]byte(secret))
	last4 := secret
	if len(last4) > 4 {
		last4 = last4[len(last4)-4:]
	}
	return EncryptedSecret{
		Ciphertext:  b.aead.Seal(nil, nonce, []byte(secret), associatedData),
		Nonce:       nonce,
		Fingerprint: hex.EncodeToString(sum[:]),
		Last4:       last4,
	}, nil
}

func (b *CredentialBox) Decrypt(
	ciphertext []byte,
	nonce []byte,
	associatedData []byte,
) (string, error) {
	if b == nil || b.aead == nil {
		return "", ErrCredentialKeyMissing
	}
	if len(nonce) != b.aead.NonceSize() || len(ciphertext) == 0 {
		return "", errors.New("encrypted credential is malformed")
	}
	plain, err := b.aead.Open(nil, nonce, ciphertext, associatedData)
	if err != nil {
		return "", errors.New("encrypted credential could not be authenticated")
	}
	return string(plain), nil
}
