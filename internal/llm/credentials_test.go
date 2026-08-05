package llm

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

type staticResolver map[string][]net.IP

func (r staticResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	ips, ok := r[host]
	if !ok {
		return nil, errors.New("not found")
	}
	result := make([]net.IPAddr, 0, len(ips))
	for _, ip := range ips {
		result = append(result, net.IPAddr{IP: ip})
	}
	return result, nil
}

type failingReader struct{}

func (failingReader) Read(_ []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func testCredentialKey() string {
	return base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
}

func TestCredentialBoxRoundTrip(t *testing.T) {
	t.Parallel()
	box, err := NewCredentialBox(testCredentialKey())
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := box.Encrypt("sk-secret-value", []byte("connection-id"))
	if err != nil {
		t.Fatal(err)
	}
	if string(encrypted.Ciphertext) == "sk-secret-value" ||
		len(encrypted.Fingerprint) != 64 || encrypted.Last4 != "alue" {
		t.Fatalf("credential metadata is unsafe or incomplete: %+v", encrypted)
	}
	decrypted, err := box.Decrypt(
		encrypted.Ciphertext,
		encrypted.Nonce,
		[]byte("connection-id"),
	)
	if err != nil || decrypted != "sk-secret-value" {
		t.Fatalf("round trip failed: %q, %v", decrypted, err)
	}
	if _, err := box.Decrypt(encrypted.Ciphertext, encrypted.Nonce, []byte("other")); err == nil {
		t.Fatal("credential decrypted with the wrong associated data")
	}
}

func TestCredentialBoxRejectsInvalidInputs(t *testing.T) {
	t.Parallel()
	if _, err := NewCredentialBox(""); !errors.Is(err, ErrCredentialKeyMissing) {
		t.Fatalf("unexpected missing-key error: %v", err)
	}
	if _, err := NewCredentialBox("not base64"); !errors.Is(err, ErrCredentialKeyInvalid) {
		t.Fatalf("unexpected invalid-key error: %v", err)
	}
	invalidLength := base64.StdEncoding.EncodeToString([]byte("short"))
	if _, err := NewCredentialBox(invalidLength); !errors.Is(err, ErrCredentialKeyInvalid) {
		t.Fatalf("unexpected key-length error: %v", err)
	}
	var missing *CredentialBox
	if _, err := missing.Encrypt("key", nil); !errors.Is(err, ErrCredentialKeyMissing) {
		t.Fatalf("unexpected nil-box encrypt error: %v", err)
	}
	if _, err := missing.Decrypt([]byte{1}, []byte{2}, nil); !errors.Is(
		err,
		ErrCredentialKeyMissing,
	) {
		t.Fatalf("unexpected nil-box decrypt error: %v", err)
	}
	box, err := NewCredentialBox(testCredentialKey())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := box.Encrypt(" ", nil); err == nil {
		t.Fatal("blank credential was encrypted")
	}
	if _, err := box.Decrypt(nil, nil, nil); err == nil {
		t.Fatal("malformed credential was decrypted")
	}
	box.rand = failingReader{}
	if _, err := box.Encrypt("secret", nil); err == nil {
		t.Fatal("nonce failure was ignored")
	}
}

func TestEndpointPolicy(t *testing.T) {
	t.Parallel()
	resolver := staticResolver{
		"provider.example": {net.ParseIP("93.184.216.34")},
		"internal.example": {net.ParseIP("10.0.0.5")},
		"mixed.example": {
			net.ParseIP("93.184.216.34"),
			net.ParseIP("192.168.1.8"),
		},
	}
	public := EndpointPolicy{Resolver: resolver}
	parsed, err := public.Validate(context.Background(), "https://provider.example/v1/")
	if err != nil || parsed.Hostname() != "provider.example" {
		t.Fatalf("public endpoint rejected: %v", err)
	}
	rejected := []string{
		"http://provider.example/v1",
		"https://localhost/v1",
		"https://127.0.0.1/v1",
		"https://10.0.0.1/v1",
		"https://internal.example/v1",
		"https://mixed.example/v1",
		"https://user:pass@provider.example/v1",
		"https://provider.example/v1#fragment",
		"not-a-url",
		"https://missing.example/v1",
	}
	for _, endpoint := range rejected {
		if _, err := public.Validate(context.Background(), endpoint); !errors.Is(
			err,
			ErrUnsafeEndpoint,
		) {
			t.Fatalf("%q should be rejected, got %v", endpoint, err)
		}
	}
	private := EndpointPolicy{AllowPrivate: true, Resolver: resolver}
	for _, endpoint := range []string{
		"http://localhost:11434/v1",
		"http://10.0.0.1:8000/v1",
		"https://internal.example/v1",
	} {
		if _, err := private.Validate(context.Background(), endpoint); err != nil {
			t.Fatalf("private opt-in endpoint %q rejected: %v", endpoint, err)
		}
	}
}

func TestEndpointHTTPClientSecurityDefaults(t *testing.T) {
	t.Parallel()
	client := (EndpointPolicy{}).HTTPClient(7 * time.Second)
	if client.Timeout != 7*time.Second {
		t.Fatalf("unexpected timeout: %v", client.Timeout)
	}
	if err := client.CheckRedirect(nil, nil); err == nil ||
		!strings.Contains(err.Error(), "disabled") {
		t.Fatalf("redirect policy not installed: %v", err)
	}
	transport := client.Transport.(*http.Transport)
	if _, err := transport.DialContext(
		context.Background(),
		"tcp",
		"malformed-address",
	); err == nil {
		t.Fatal("malformed dial address was accepted")
	}

	privateResolver := staticResolver{"provider.example": {net.ParseIP("10.0.0.1")}}
	client = (EndpointPolicy{Resolver: privateResolver}).HTTPClient(time.Second)
	transport = client.Transport.(*http.Transport)
	if _, err := transport.DialContext(
		context.Background(),
		"tcp",
		"provider.example:443",
	); !errors.Is(err, ErrUnsafeEndpoint) {
		t.Fatalf("private DNS rebinding was not blocked: %v", err)
	}

	missingResolver := staticResolver{}
	client = (EndpointPolicy{Resolver: missingResolver}).HTTPClient(time.Second)
	transport = client.Transport.(*http.Transport)
	if _, err := transport.DialContext(
		context.Background(),
		"tcp",
		"provider.example:443",
	); err == nil || !strings.Contains(err.Error(), "resolve") {
		t.Fatalf("DNS failure was not reported: %v", err)
	}

	client = (EndpointPolicy{AllowPrivate: true}).HTTPClient(200 * time.Millisecond)
	transport = client.Transport.(*http.Transport)
	if _, err := transport.DialContext(
		context.Background(),
		"tcp",
		"127.0.0.1:1",
	); err == nil {
		t.Fatal("unexpected local listener on test port")
	}
}

func TestPrivateOrSpecialIP(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		"127.0.0.1", "10.0.0.1", "169.254.1.1", "0.0.0.0", "224.0.0.1", "::1",
	} {
		if !isPrivateOrSpecialIP(net.ParseIP(raw)) {
			t.Fatalf("%s should be private or special", raw)
		}
	}
	if isPrivateOrSpecialIP(net.ParseIP("93.184.216.34")) {
		t.Fatal("public address classified as private")
	}
}
