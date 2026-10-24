package controller

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// channelVideoContentURL grants no trust to caller/result URLs. It constructs
// only the content endpoint under the stored channel base, including bases on
// internal Docker service ports that are valid for the configured API channel.
func channelVideoContentURL(baseURL, upstreamTaskID string) (string, error) {
	base, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || base == nil || (base.Scheme != "http" && base.Scheme != "https") || base.Hostname() == "" ||
		base.User != nil || base.Opaque != "" || base.RawQuery != "" || base.ForceQuery || base.Fragment != "" {
		return "", fmt.Errorf("invalid channel base URL")
	}
	if upstreamTaskID == "" || upstreamTaskID == "." || strings.Contains(upstreamTaskID, "..") || strings.Contains(upstreamTaskID, "://") {
		return "", fmt.Errorf("invalid stored upstream task ID")
	}
	for _, char := range upstreamTaskID {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || strings.ContainsRune("-_+/=:.", char)) {
			return "", fmt.Errorf("invalid stored upstream task ID")
		}
	}
	escapedBase := strings.TrimRight(base.EscapedPath(), "/")
	base.Path = strings.TrimRight(base.Path, "/") + "/v1/videos/" + upstreamTaskID + "/content"
	base.RawPath = escapedBase + "/v1/videos/" + url.PathEscape(upstreamTaskID) + "/content"
	return base.String(), nil
}

// The configured first hop is the only exception. Redirects have the ordinary
// fetch policy and receive no upstream credentials, even on the same host.
// Copying the client keeps the shared relay/proxy client unchanged.
func channelVideoContentClient(original *http.Client) *http.Client {
	if original == nil {
		original = http.DefaultClient
	}
	client := *original
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		for _, name := range []string{"Authorization", "Proxy-Authorization", "Cookie", "X-Api-Key", "X-Goog-Api-Key"} {
			req.Header.Del(name)
		}
		fetch := system_setting.GetFetchSetting()
		if err := common.ValidateURLWithFetchSetting(req.URL.String(), fetch.EnableSSRFProtection, fetch.AllowPrivateIp, fetch.DomainFilterMode, fetch.IpFilterMode, fetch.DomainList, fetch.IpList, fetch.AllowedPorts, fetch.ApplyIPFilterForDomain); err != nil {
			return fmt.Errorf("video content redirect blocked: %w", err)
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		if original.CheckRedirect != nil {
			return original.CheckRedirect(req, via)
		}
		return nil
	}
	return &client
}
