package windowsclient

import (
	"errors"
	"strconv"
	"strings"
)

const (
	ReleaseChannelStable = "stable"
	ReleaseChannelBeta   = "beta"
)

func NormalizeReleaseChannel(channel string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(channel)) {
	case "", ReleaseChannelBeta:
		// Preserve compatibility with the previous prerelease-only channel.
		return ReleaseChannelBeta, nil
	case ReleaseChannelStable:
		return ReleaseChannelStable, nil
	default:
		return "", errors.New("update channel must be stable or beta")
	}
}

type releaseParts struct {
	major, minor, patch uint64
	suffix              string
}

func parseWindowsClientVersion(raw string) (releaseParts, bool) {
	raw = strings.TrimPrefix(strings.TrimSpace(raw), "v")
	parts := strings.SplitN(raw, "-", 2)
	num := strings.Split(parts[0], ".")
	if len(num) != 3 {
		return releaseParts{}, false
	}
	values := [3]uint64{}
	for i, component := range num {
		if component == "" {
			return releaseParts{}, false
		}
		for _, r := range component {
			if r < '0' || r > '9' {
				return releaseParts{}, false
			}
		}
		value, err := strconv.ParseUint(component, 10, 64)
		if err != nil {
			return releaseParts{}, false
		}
		values[i] = value
	}
	result := releaseParts{major: values[0], minor: values[1], patch: values[2]}
	if len(parts) == 2 {
		if parts[1] == "" {
			return releaseParts{}, false
		}
		for _, r := range parts[1] {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') ||
				(r >= 'A' && r <= 'Z') || r == '-' || r == '.') {
				return releaseParts{}, false
			}
		}
		result.suffix = parts[1]
	}
	return result, true
}

func validReleaseVersion(version string) bool {
	_, ok := parseWindowsClientVersion(version)
	return ok
}

// CompareWindowsClientVersions compares source release tag versions. It treats
// the stable tag as newer than the prerelease of the same numeric version.
// An unknown version (e.g. local `dev` build) sorts older than a valid release.
func CompareWindowsClientVersions(left, right string) int {
	l, lok := parseWindowsClientVersion(left)
	r, rok := parseWindowsClientVersion(right)
	if !lok {
		if !rok {
			return 0
		}
		return -1
	}
	if !rok {
		return 1
	}
	for _, item := range [][2]uint64{{l.major, r.major}, {l.minor, r.minor}, {l.patch, r.patch}} {
		if item[0] > item[1] {
			return 1
		}
		if item[0] < item[1] {
			return -1
		}
	}
	if l.suffix == r.suffix {
		return 0
	}
	if l.suffix == "" {
		return 1
	}
	if r.suffix == "" {
		return -1
	}
	return strings.Compare(l.suffix, r.suffix)
}
