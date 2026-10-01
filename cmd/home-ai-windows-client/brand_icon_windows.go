//go:build windows

package main

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

const homeAIIconBase64 = "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"

var (
	brandUser32                     = windows.NewLazySystemDLL("user32.dll")
	procBrandCreateIconFromResource = brandUser32.NewProc("CreateIconFromResourceEx")
	procBrandDestroyIcon            = brandUser32.NewProc("DestroyIcon")
)

func loadHomeAIIcon(size int) (windows.Handle, error) {
	data, err := base64.StdEncoding.DecodeString(homeAIIconBase64)
	if err != nil {
		return 0, err
	}
	if len(data) < 22 || binary.LittleEndian.Uint16(data[0:2]) != 0 ||
		binary.LittleEndian.Uint16(data[2:4]) != 1 ||
		binary.LittleEndian.Uint16(data[4:6]) < 1 {
		return 0, errors.New("invalid embedded HOME AI icon")
	}
	bytesInResource := int(binary.LittleEndian.Uint32(data[14:18]))
	imageOffset := int(binary.LittleEndian.Uint32(data[18:22]))
	if bytesInResource <= 0 || imageOffset < 0 || imageOffset+bytesInResource > len(data) {
		return 0, errors.New("invalid embedded HOME AI icon entry")
	}
	if size <= 0 {
		size = 32
	}
	image := data[imageOffset : imageOffset+bytesInResource]
	handle, _, callErr := procBrandCreateIconFromResource.Call(
		uintptr(unsafe.Pointer(&image[0])),
		uintptr(len(image)),
		1,
		0x00030000,
		uintptr(size),
		uintptr(size),
		0,
	)
	runtime.KeepAlive(data)
	if handle == 0 {
		return 0, callErr
	}
	return windows.Handle(handle), nil
}

func destroyHomeAIIcon(icon windows.Handle) {
	if icon != 0 {
		procBrandDestroyIcon.Call(uintptr(icon))
	}
}
