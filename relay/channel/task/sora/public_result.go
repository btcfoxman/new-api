package sora

import (
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// Only completed public results may replace the owned content endpoint. This
// also keeps private/authenticated temporary asset URLs out of public metadata.
func completedPublicVideoURL(status, raw string) string {
	if status != "completed" {
		return ""
	}
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return ""
	}
	fetch := system_setting.GetFetchSetting()
	// Publication always requires a public address, even when the administrator
	// allows private asset downloads through the authenticated content endpoint.
	if common.ValidateURLWithFetchSetting(raw, true, false, fetch.DomainFilterMode, fetch.IpFilterMode, fetch.DomainList, fetch.IpList, fetch.AllowedPorts, true) != nil {
		return ""
	}
	return raw
}
