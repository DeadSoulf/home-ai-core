package realtime

import (
	"fmt"
	"testing"
)

func TestCumulativeTopicLimitIsAtomic(t *testing.T) {
	client := newClient()
	topics := make([]string, MaxTopics)
	for i := range topics {
		topics[i] = fmt.Sprintf("files.folder%d", i)
	}
	if _, err := client.subscribe(topics); err != nil {
		t.Fatal(err)
	}
	if _, err := client.subscribe([]string{"files.extra"}); err == nil {
		t.Fatal("cumulative topic limit was bypassed")
	}
	if _, err := client.subscribe(topics); err != nil {
		t.Fatal("duplicate topics count twice", err)
	}
	if len(client.topics) != MaxTopics {
		t.Fatalf("mutation despite reject: %d", len(client.topics))
	}
}
