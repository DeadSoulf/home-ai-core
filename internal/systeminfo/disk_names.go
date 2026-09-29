package systeminfo

import "strings"

func DiskStableID(path, serial string) string {
	if value := strings.TrimSpace(serial); value != "" {
		return "serial:" + value
	}
	return "path:" + strings.TrimSpace(path)
}

func ApplyDiskNames(info *Info, names map[string]string) {
	if info == nil || len(names) == 0 {
		return
	}
	for i := range info.BlockTree {
		applyBlockNodeName(&info.BlockTree[i], names)
	}
	for i := range info.BlockDevices {
		if name := names[DiskStableID(info.BlockDevices[i].Path, info.BlockDevices[i].Serial)]; name != "" {
			info.BlockDevices[i].DisplayName = name
		}
	}
}

func applyBlockNodeName(node *BlockNode, names map[string]string) {
	if node == nil {
		return
	}
	if node.Type == "disk" {
		if name := names[DiskStableID(node.Path, node.Serial)]; name != "" {
			node.DisplayName = name
		}
	}
	for i := range node.Children {
		applyBlockNodeName(&node.Children[i], names)
	}
}
