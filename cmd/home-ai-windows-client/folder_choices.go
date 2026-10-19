package main

import (
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
)

// folderChoice keeps the API's stable folder ID separate from the visible
// label. IDs must never be appended to folder names in the settings combo.
type folderChoice struct {
	label string
	id    string
}

func writableFolderChoices(folders []windowsclient.Folder, preferred, savedLabel string) ([]folderChoice, int) {
	choices := make([]folderChoice, 0, len(folders)+1)
	selected := -1
	for _, folder := range folders {
		if !folder.CanWrite {
			continue
		}
		name := strings.TrimSpace(folder.Name)
		pool := strings.TrimSpace(folder.PoolName)
		label := name
		if label == "" {
			label = pool
		}
		if label == "" {
			label = savedLabel
		}
		if name != "" && pool != "" && !strings.EqualFold(name, pool) {
			label += " (" + pool + ")"
		}
		choices = append(choices, folderChoice{label: label, id: folder.ID})
		if folder.ID == preferred {
			selected = len(choices) - 1
		}
	}
	if preferred != "" && selected < 0 {
		// A saved profile may point to a temporarily inaccessible folder.
		// Keep its ID for profile editing, but do not expose it in the UI.
		choices = append(choices, folderChoice{label: savedLabel, id: preferred})
		selected = len(choices) - 1
	}
	if selected < 0 && len(choices) > 0 {
		selected = 0
	}
	return choices, selected
}
