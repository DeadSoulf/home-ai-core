package coreupdate

import (
	"strconv"
	"strings"
)

type parsedVersion struct {
	major, minor, patch int
	suffix              string
	valid               bool
}

func compareVersions(a, b string) int {
	left := parseVersion(a)
	right := parseVersion(b)
	if !left.valid || !right.valid {
		return strings.Compare(a, b)
	}
	for _, pair := range [][2]int{{left.major, right.major}, {left.minor, right.minor}, {left.patch, right.patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	if left.suffix == right.suffix {
		return 0
	}
	if left.suffix == "" {
		return 1
	}
	if right.suffix == "" {
		return -1
	}
	return strings.Compare(left.suffix, right.suffix)
}

func parseVersion(value string) parsedVersion {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	core, suffix, _ := strings.Cut(value, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return parsedVersion{}
	}
	nums := [3]int{}
	for i, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return parsedVersion{}
		}
		nums[i] = number
	}
	return parsedVersion{major: nums[0], minor: nums[1], patch: nums[2], suffix: suffix, valid: true}
}
