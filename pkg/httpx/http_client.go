package httpx

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func CreateHTTPClient(proxyURL string, timeout time.Duration) (*http.Client, error) {
	client := &http.Client{Timeout: timeout, Transport: defaultHTTPTransport()}
	transport, _ := client.Transport.(*http.Transport)

	if proxyURL != "" {
		proxy, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}
		scheme := strings.ToLower(proxy.Scheme)
		switch scheme {
		case "http", "https", "socks5", "socks5h":
		default:
			return nil, fmt.Errorf(
				"unsupported proxy scheme %q (supported: http, https, socks5, socks5h)",
				proxy.Scheme,
			)
		}
		if proxy.Host == "" {
			return nil, fmt.Errorf("invalid proxy URL: missing host")
		}
		transport.Proxy = http.ProxyURL(proxy)
	} else {
		transport.Proxy = proxyFromEnvironmentExceptPrivate
	}

	return client, nil
}

func defaultHTTPTransport() *http.Transport {
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		transport := base.Clone()
		transport.MaxIdleConns = 10
		transport.IdleConnTimeout = 30 * time.Second
		transport.DisableCompression = false
		transport.TLSHandshakeTimeout = 15 * time.Second
		return transport
	}

	return &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  false,
		TLSHandshakeTimeout: 15 * time.Second,
	}
}

func proxyFromEnvironmentExceptPrivate(req *http.Request) (*url.URL, error) {
	if req == nil || shouldBypassEnvironmentProxy(req.URL) {
		return nil, nil
	}
	return http.ProxyFromEnvironment(req)
}

func shouldBypassEnvironmentProxy(u *url.URL) bool {
	if u == nil {
		return false
	}

	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}