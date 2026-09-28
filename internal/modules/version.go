package modules

import (
	"fmt"
	"strconv"
	"strings"
)

type version struct {
	major int
	minor int
	patch int
}

func parseVersion(value string) (version, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "v") {
		value = strings.TrimPrefix(value, "v")
	}
	if strings.ContainsAny(value, "+-") {
		return version{}, fmt.Errorf("pre-release/build metadata is not supported in manifest v1")
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return version{}, fmt.Errorf("version must use major.minor.patch")
	}
	values := make([]int, 3)
	for i, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return version{}, fmt.Errorf("invalid semantic version")
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return version{}, fmt.Errorf("invalid semantic version")
		}
		values[i] = n
	}
	return version{major: values[0], minor: values[1], patch: values[2]}, nil
}

func compareVersion(a, b version) int {
	if a.major != b.major {
		if a.major < b.major { return -1 }
		return 1
	}
	if a.minor != b.minor {
		if a.minor < b.minor { return -1 }
		return 1
	}
	if a.patch != b.patch {
		if a.patch < b.patch { return -1 }
		return 1
	}
	return 0
}

func satisfies(value, constraint string) (bool, error) {
	current, err := parseVersion(value)
	if err != nil {
		return false, err
	}
	constraint = strings.TrimSpace(constraint)
	if constraint == "" || constraint == "*" {
		return true, nil
	}
	for _, term := range strings.Fields(constraint) {
		op := "="
		target := term
		for _, candidate := range []string{">=", "<=", ">", "<", "="} {
			if strings.HasPrefix(term, candidate) {
				op = candidate
				target = strings.TrimPrefix(term, candidate)
				break
			}
		}
		required, err := parseVersion(target)
		if err != nil {
			return false, fmt.Errorf("invalid constraint %q: %w", term, err)
		}
		cmp := compareVersion(current, required)
		ok := false
		switch op {
		case "=":
			ok = cmp == 0
		case ">=":
			ok = cmp >= 0
		case "<=":
			ok = cmp <= 0
		case ">":
			ok = cmp > 0
		case "<":
			ok = cmp < 0
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}
