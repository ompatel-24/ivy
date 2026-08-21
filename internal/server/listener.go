package server

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Listen validates that address names one concrete local interface before
// binding it. Rome never silently listens on every interface.
func Listen(address string) (net.Listener, error) {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid ROME_LISTEN address %q: %w", address, err)
	}
	if host == "" {
		return nil, fmt.Errorf("invalid ROME_LISTEN address %q: host is required", address)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 0 || port > 65535 {
		return nil, fmt.Errorf("invalid ROME_LISTEN address %q: port must be between 0 and 65535", address)
	}
	if isUnspecifiedHost(host) {
		return nil, fmt.Errorf("invalid ROME_LISTEN address %q: wildcard hosts are not allowed", address)
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", address, err)
	}
	tcpAddress, ok := listener.Addr().(*net.TCPAddr)
	if !ok || tcpAddress.IP == nil || tcpAddress.IP.IsUnspecified() {
		_ = listener.Close()
		return nil, fmt.Errorf("invalid ROME_LISTEN address %q: address did not resolve to a concrete interface", address)
	}
	return listener, nil
}

func isUnspecifiedHost(host string) bool {
	withoutZone, _, _ := strings.Cut(host, "%")
	ip := net.ParseIP(withoutZone)
	return ip != nil && ip.IsUnspecified()
}

// ConnectionURL builds the one-time local token handoff URL. URL fragments
// are not included in HTTP requests.
func ConnectionURL(address net.Addr, sessionID, token string) (string, error) {
	tcpAddress, ok := address.(*net.TCPAddr)
	if !ok || tcpAddress.IP == nil {
		return "", fmt.Errorf("unsupported listener address %q", address.String())
	}
	host := tcpAddress.IP.String()
	if tcpAddress.Zone != "" {
		host += "%" + tcpAddress.Zone
	}
	result := url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort(host, strconv.Itoa(tcpAddress.Port)),
		Path:     "/s/" + sessionID,
		Fragment: "token=" + token,
	}
	return result.String(), nil
}
