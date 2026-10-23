package capability

import "testing"

func TestSnapshotResourceNamesIncludesDAV(t *testing.T) {
	names, err := SnapshotResourceNames([]byte(`{"webdav":[{"resourceId":"dav_test","displayName":"Workspace files"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if names["dav_test"] != "Workspace files" {
		t.Fatalf("DAV resource name lost: %v", names)
	}
}
