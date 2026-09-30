//go:build !linux

package filedata

func installUploadPart(partPath, target string) error {
	return installUploadPartByLink(partPath, target)
}
