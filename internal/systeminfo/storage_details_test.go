package systeminfo

import (
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

func TestApplyStorageDetails(t *testing.T) {
	temp := 34
	hours := uint64(1234)
	life := 91
	data := 42.5
	meta := 3.5

	info := Info{BlockTree: []BlockNode{{
		Name: "sdb",
		Path: "/dev/sdb",
		Type: "disk",
		Children: []BlockNode{{
			Name: "vg-data",
			Path: "/dev/mapper/vg-data",
			Type: "lvm",
		}},
	}}}

	ApplyStorageDetails(
		&info,
		[]updaterhelper.DiskHealthStat{{
			Device: "/dev/sdb", Transport: "sata", Health: "ok",
			TemperatureC: &temp, PowerOnHours: &hours, LifeRemainingPct: &life,
			SmartAvailable: true,
		}},
		[]updaterhelper.LVMStat{{
			Device: "/dev/vg/data", Name: "vg-data", VGName: "vg", LVName: "data",
			SizeBytes: 1000, VGSizeBytes: 2000, VGFreeBytes: 500, Active: true,
			DataPercent: &data, MetadataPercent: &meta,
		}},
	)

	disk := info.BlockTree[0]
	if disk.Health != "ok" || disk.Transport != "sata" || disk.TemperatureC == nil || *disk.TemperatureC != 34 {
		t.Fatalf("disk health details not applied: %+v", disk)
	}
	lv := disk.Children[0]
	if lv.LVMVGName != "vg" || lv.LVMLVName != "data" || lv.LVMActive == nil || !*lv.LVMActive {
		t.Fatalf("LVM details not applied: %+v", lv)
	}
	if lv.LVMVGFreeBytes != 500 || lv.LVMVGSizeBytes != 2000 {
		t.Fatalf("LVM capacity not applied: %+v", lv)
	}
}
