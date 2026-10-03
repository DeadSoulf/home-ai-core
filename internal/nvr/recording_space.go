package nvr

type FilesystemSpace struct {
	TotalBytes     uint64
	AvailableBytes uint64
}

type SpaceChecker interface {
	Space(string) (FilesystemSpace, error)
}

type osSpaceChecker struct{}
