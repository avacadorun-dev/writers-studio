package llm

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrUnsafeEndpoint = errors.New("provider endpoint is not allowed")

type Resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type EndpointPolicy struct {
	AllowPrivate bool
	Resolver     Resolver
}

func (p EndpointPolicy) Validate(ctx context.Context, rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Hostname() == "" {
		return nil, fmt.Errorf("%w: invalid URL", ErrUnsafeEndpoint)
	}
	if parsed.Scheme != "https" && !(p.AllowPrivate && parsed.Scheme == "http") {
		return nil, fmt.Errorf("%w: HTTPS is required", ErrUnsafeEndpoint)
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return nil, fmt.Errorf("%w: credentials and fragments are forbidden", ErrUnsafeEndpoint)
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		if !p.AllowPrivate {
			return nil, fmt.Errorf("%w: local endpoint", ErrUnsafeEndpoint)
		}
		return parsed, nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if !p.AllowPrivate && isPrivateOrSpecialIP(ip) {
			return nil, fmt.Errorf("%w: private network endpoint", ErrUnsafeEndpoint)
		}
		return parsed, nil
	}
	resolver := p.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	lookupContext, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	addresses, err := resolver.LookupIPAddr(lookupContext, host)
	if err != nil || len(addresses) == 0 {
		return nil, fmt.Errorf("%w: DNS lookup failed", ErrUnsafeEndpoint)
	}
	if !p.AllowPrivate {
		for _, address := range addresses {
			if isPrivateOrSpecialIP(address.IP) {
				return nil, fmt.Errorf("%w: DNS resolves to a private network", ErrUnsafeEndpoint)
			}
		}
	}
	return parsed, nil
}

func (p EndpointPolicy) HTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	baseDialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport.DialContext = func(
		ctx context.Context,
		network string,
		address string,
	) (net.Conn, error) {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("parse provider address: %w", err)
		}
		if !p.AllowPrivate {
			resolver := p.Resolver
			if resolver == nil {
				resolver = net.DefaultResolver
			}
			addresses, err := resolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("resolve provider address: %w", err)
			}
			for _, resolved := range addresses {
				if isPrivateOrSpecialIP(resolved.IP) {
					return nil, ErrUnsafeEndpoint
				}
			}
		}
		return baseDialer.DialContext(ctx, network, address)
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("provider redirects are disabled")
		},
	}
}

func isPrivateOrSpecialIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}
