package systeminfo

import (
	"path/filepath"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

func ApplyStorageDetails(info *Info, health []updaterhelper.DiskHealthStat, lvm []updaterhelper.LVMStat) {
	if info == nil {
		return
	}
	healthByPath := make(map[string]updaterhelper.DiskHealthStat, len(health))
	for _, stat := range health {
		if stat.Device != "" {
			healthByPath[stat.Device] = stat
		}
	}
	for i := range info.BlockTree {
		applyStorageDetailsNode(&info.BlockTree[i], healthByPath, lvm)
	}
}

func applyStorageDetailsNode(
	node *BlockNode,
	health map[string]updaterhelper.DiskHealthStat,
	lvm []updaterhelper.LVMStat,
) {
	if node == nil {
		return
	}
	if stat, ok := health[node.Path]; ok {
		if stat.Transport != "" {
			node.Transport = strings.ToLower(stat.Transport)
		}
		node.Health = stat.Health
		node.TemperatureC = stat.TemperatureC
		node.PowerOnHours = stat.PowerOnHours
		node.LifeRemainingPct = stat.LifeRemainingPct
		node.SmartAvailable = stat.SmartAvailable
		node.SmartError = stat.SmartError
	}
	if node.Type == "lvm" {
		for _, stat := range lvm {
			if stat.Device == node.Path || stat.Name == node.Name || stat.Name == filepath.Base(node.Path) {
				active := stat.Active
				node.LVMVGName = stat.VGName
				node.LVMLVName = stat.LVName
				node.LVMActive = &active
				node.LVMDataPercent = stat.DataPercent
				node.LVMMetadataPercent = stat.MetadataPercent
				if stat.SizeBytes > 0 {
					node.SizeBytes = stat.SizeBytes
				}
				break
			}
		}
	}
	for i := range node.Children {
		applyStorageDetailsNode(&node.Children[i], health, lvm)
	}
}
