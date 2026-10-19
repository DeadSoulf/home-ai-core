package main

import (
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
)

func TestWritableFolderChoicesUseReadableNamesWithoutExposingIDs(t *testing.T) {
	id := "nsf_2ef0584d7b414d5273dc7dfe99d2627e"
	folders := []windowsclient.Folder{
		{ID: "readonly-123", Name: "Restricted", PoolName: "Files", CanWrite: false},
		{ID: id, Name: "Файлы", PoolName: "Файлы", CanWrite: true},
		{ID: "nsf_photos", Name: "Photos", PoolName: "Archive", CanWrite: true},
		{ID: "nsf_other", Name: "Photos", PoolName: "Backup", CanWrite: true},
	}
	choices, selected := writableFolderChoices(folders, id, "Сохранённая папка")
	if len(choices) != 3 {
		t.Fatalf("got %d folders, want 3", len(choices))
	}
	if selected != 0 || choices[0].label != "Файлы" || choices[0].id != id {
		t.Fatalf("unexpected initial selection: selected=%d, first=%+v", selected, choices[0])
	}
	if choices[1].label != "Photos (Archive)" || choices[2].label != "Photos (Backup)" {
		t.Fatalf("pool names should distinguish matching folder names: %+v", choices)
	}
	for _, choice := range choices {
		if strings.Contains(choice.label, "nsf_") || strings.Contains(choice.label, choice.id) {
			t.Fatalf("internal folder ID leaked to label: %+v", choice)
		}
	}
}

func TestWritableFolderChoicesKeepSavedIDWithoutDisplayingIt(t *testing.T) {
	id := "nsf_previous_folder"
	choices, selected := writableFolderChoices(
		[]windowsclient.Folder{{ID: "nsf_current", Name: "Current", CanWrite: true}},
		id, "Saved folder",
	)
	if selected != 1 || len(choices) != 2 {
		t.Fatalf("unexpected saved selection: selected=%d, choices=%+v", selected, choices)
	}
	if choices[1].id != id || choices[1].label != "Saved folder" {
		t.Fatalf("saved folder ID and visible label must be separate: %+v", choices[1])
	}
}

func TestWritableFolderChoicesHandleNoFoldersAndEmptyNames(t *testing.T) {
	choices, selected := writableFolderChoices(nil, "", "Saved folder")
	if selected != -1 || len(choices) != 0 {
		t.Fatalf("empty choices = %+v, selected=%d", choices, selected)
	}
	choices, selected = writableFolderChoices([]windowsclient.Folder{
		{ID: "empty-name", PoolName: "Data", CanWrite: true},
	}, "", "Saved folder")
	if selected != 0 || choices[0].label != "Data" || choices[0].id != "empty-name" {
		t.Fatalf("fallback label = %+v selected=%d", choices, selected)
	}
}
