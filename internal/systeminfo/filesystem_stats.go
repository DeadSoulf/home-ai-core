package systeminfo

import "github.com/DeadSoulf/home-ai-core/internal/updaterhelper"

func ApplyFilesystemStats(info *Info, stats []updaterhelper.FilesystemStat) {
	if info == nil || len(stats) == 0 {
		return
	}
	byPath := make(map[string]updaterhelper.FilesystemStat, len(stats))
	for _, stat := range stats {
		if stat.Device != "" && stat.FreeKnown {
			byPath[stat.Device] = stat
		}
	}
	for i := range info.BlockTree {
		applyFilesystemStatsNode(&info.BlockTree[i], byPath)
	}
}

func applyFilesystemStatsNode(node *BlockNode, stats map[string]updaterhelper.FilesystemStat) {
	if node == nil {
		return
	}
	if stat, ok := stats[node.Path]; ok {
		node.FreeBytes = stat.FreeBytes
	}
	for i := range node.Children {
		applyFilesystemStatsNode(&node.Children[i], stats)
	}
	if node.Type == "disk" {
		// A physical disk's direct free space represents only unallocated
		// capacity. Filesystem free space remains on child nodes so callers can
		// sum the tree without double-counting.
		node.FreeBytes = node.UnallocatedBytes
	}
}
