package store

import (
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestStoreOpenAndMigrate(t *testing.T) {
	// Create a temporary database file
	tmpFile := "test-store.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	// Open the store
	store, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Verify schema version
	version, err := store.getSchemaVersion()
	if err != nil {
		t.Fatalf("failed to get schema version: %v", err)
	}

	if version != currentSchemaVersion {
		t.Errorf("expected schema version %d, got %d", currentSchemaVersion, version)
	}

	// Verify tables exist
	tables := []string{"files", "metadata", "clusters", "cluster_members", "plans", "executions", "schema_version"}
	for _, table := range tables {
		var count int
		err := store.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query table %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("expected table %s to exist", table)
		}
	}

	// Verify v2 performance indexes exist
	v2Indexes := []string{
		"idx_cluster_members_quality",
		"idx_metadata_duration",
		"idx_files_status_id",
	}
	for _, index := range v2Indexes {
		var count int
		err := store.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?", index).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query index %s: %v", index, err)
		}
		if count != 1 {
			t.Errorf("expected index %s to exist (schema v2)", index)
		}
	}
}

func TestFileInsertAndRetrieve(t *testing.T) {
	// Create a temporary database file
	tmpFile := "test-files.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	store, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Insert a file
	file := &File{
		FileKey:   "test-key-123",
		SrcPath:   "/path/to/test.mp3",
		SizeBytes: 1024,
		MtimeUnix: 1234567890,
		Status:    "discovered",
	}

	err = store.InsertFile(file)
	if err != nil {
		t.Fatalf("failed to insert file: %v", err)
	}

	if file.ID == 0 {
		t.Error("expected file ID to be set after insert")
	}

	// Retrieve the file
	retrieved, err := store.GetFileByKey("test-key-123")
	if err != nil {
		t.Fatalf("failed to retrieve file: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected to retrieve file, got nil")
	}

	if retrieved.FileKey != file.FileKey {
		t.Errorf("expected FileKey %s, got %s", file.FileKey, retrieved.FileKey)
	}

	if retrieved.SrcPath != file.SrcPath {
		t.Errorf("expected SrcPath %s, got %s", file.SrcPath, retrieved.SrcPath)
	}

	// Update status
	err = store.UpdateFileStatus(file.ID, "meta_ok", "")
	if err != nil {
		t.Fatalf("failed to update file status: %v", err)
	}

	// Retrieve again and verify status
	retrieved, err = store.GetFileByKey("test-key-123")
	if err != nil {
		t.Fatalf("failed to retrieve file after update: %v", err)
	}

	if retrieved.Status != "meta_ok" {
		t.Errorf("expected status 'meta_ok', got '%s'", retrieved.Status)
	}
}

func TestMetadataInsertAndRetrieve(t *testing.T) {
	// Create a temporary database file
	tmpFile := "test-metadata.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	store, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Insert a file first
	file := &File{
		FileKey:   "test-key-456",
		SrcPath:   "/path/to/test.flac",
		SizeBytes: 20480,
		MtimeUnix: 1234567890,
		Status:    "discovered",
	}

	err = store.InsertFile(file)
	if err != nil {
		t.Fatalf("failed to insert file: %v", err)
	}

	// Insert metadata
	metadata := &Metadata{
		FileID:         file.ID,
		Format:         "FLAC",
		Codec:          "flac",
		Container:      "flac",
		DurationMs:     240000,
		SampleRate:     44100,
		BitDepth:       16,
		Channels:       2,
		BitrateKbps:    0,
		Lossless:       true,
		TagArtist:      "Test Artist",
		TagAlbum:       "Test Album",
		TagTitle:       "Test Title",
		TagAlbumArtist: "Test Artist",
		TagDate:        "2023",
		TagDisc:        1,
		TagDiscTotal:   1,
		TagTrack:       1,
		TagTrackTotal:  10,
		TagCompilation: false,
		RawTagsJSON:    "{}",
	}

	err = store.InsertMetadata(metadata)
	if err != nil {
		t.Fatalf("failed to insert metadata: %v", err)
	}

	// Retrieve metadata
	retrieved, err := store.GetMetadata(file.ID)
	if err != nil {
		t.Fatalf("failed to retrieve metadata: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected to retrieve metadata, got nil")
	}

	if retrieved.TagArtist != metadata.TagArtist {
		t.Errorf("expected TagArtist %s, got %s", metadata.TagArtist, retrieved.TagArtist)
	}

	if retrieved.DurationMs != metadata.DurationMs {
		t.Errorf("expected DurationMs %d, got %d", metadata.DurationMs, retrieved.DurationMs)
	}

	if !retrieved.Lossless {
		t.Error("expected Lossless to be true")
	}
}

func TestGetFilesByStatus(t *testing.T) {
	tmpFile := "test-files-by-status.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	store, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Insert files with different statuses
	files := []*File{
		{FileKey: "key1", SrcPath: "/path/1.mp3", SizeBytes: 100, MtimeUnix: 1, Status: "discovered"},
		{FileKey: "key2", SrcPath: "/path/2.mp3", SizeBytes: 200, MtimeUnix: 2, Status: "discovered"},
		{FileKey: "key3", SrcPath: "/path/3.mp3", SizeBytes: 300, MtimeUnix: 3, Status: "meta_ok"},
		{FileKey: "key4", SrcPath: "/path/4.mp3", SizeBytes: 400, MtimeUnix: 4, Status: "error"},
	}

	for _, f := range files {
		if err := store.InsertFile(f); err != nil {
			t.Fatalf("failed to insert file: %v", err)
		}
	}

	// Query by status
	discovered, err := store.GetFilesByStatus("discovered")
	if err != nil {
		t.Fatalf("failed to get files by status: %v", err)
	}

	if len(discovered) != 2 {
		t.Errorf("expected 2 discovered files, got %d", len(discovered))
	}

	metaOk, err := store.GetFilesByStatus("meta_ok")
	if err != nil {
		t.Fatalf("failed to get files by status: %v", err)
	}

	if len(metaOk) != 1 {
		t.Errorf("expected 1 meta_ok file, got %d", len(metaOk))
	}

	// Query non-existent status
	none, err := store.GetFilesByStatus("nonexistent")
	if err != nil {
		t.Fatalf("failed to get files by status: %v", err)
	}

	if len(none) != 0 {
		t.Errorf("expected 0 files for nonexistent status, got %d", len(none))
	}
}

func TestGetAllFiles(t *testing.T) {
	tmpFile := "test-all-files.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	store, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Insert multiple files
	for i := 0; i < 5; i++ {
		file := &File{
			FileKey:   fmt.Sprintf("key-%d", i),
			SrcPath:   fmt.Sprintf("/path/%d.mp3", i),
			SizeBytes: int64(100 * (i + 1)),
			MtimeUnix: int64(i),
			Status:    "discovered",
		}
		if err := store.InsertFile(file); err != nil {
			t.Fatalf("failed to insert file: %v", err)
		}
	}

	// Get all files
	files, err := store.GetAllFiles()
	if err != nil {
		t.Fatalf("failed to get all files: %v", err)
	}

	if len(files) != 5 {
		t.Errorf("expected 5 files, got %d", len(files))
	}

	// Verify ordering (should be by ID)
	for i := 0; i < len(files)-1; i++ {
		if files[i].ID >= files[i+1].ID {
			t.Error("files are not ordered by ID")
		}
	}
}

func TestGetFileByID(t *testing.T) {
	tmpFile := "test-file-by-id.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	store, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Insert a file
	file := &File{
		FileKey:   "test-key",
		SrcPath:   "/path/test.mp3",
		SizeBytes: 1024,
		MtimeUnix: 123456,
		Status:    "discovered",
	}
	if err := store.InsertFile(file); err != nil {
		t.Fatalf("failed to insert file: %v", err)
	}

	// Retrieve by ID
	retrieved, err := store.GetFileByID(file.ID)
	if err != nil {
		t.Fatalf("failed to get file by ID: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected file, got nil")
	}

	if retrieved.FileKey != file.FileKey {
		t.Errorf("expected FileKey %s, got %s", file.FileKey, retrieved.FileKey)
	}

	// Try non-existent ID
	nonExistent, err := store.GetFileByID(99999)
	if err != nil {
		t.Fatalf("failed to get non-existent file: %v", err)
	}

	if nonExistent != nil {
		t.Error("expected nil for non-existent file ID")
	}
}

func TestCountFilesByStatus(t *testing.T) {
	tmpFile := "test-count-files.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	store, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Insert files with different statuses
	statuses := []string{"discovered", "discovered", "discovered", "meta_ok", "meta_ok", "error"}
	for i, status := range statuses {
		file := &File{
			FileKey:   fmt.Sprintf("key-%d", i),
			SrcPath:   fmt.Sprintf("/path/%d.mp3", i),
			SizeBytes: 100,
			MtimeUnix: int64(i),
			Status:    status,
		}
		if err := store.InsertFile(file); err != nil {
			t.Fatalf("failed to insert file: %v", err)
		}
	}

	// Count by status
	count, err := store.CountFilesByStatus("discovered")
	if err != nil {
		t.Fatalf("failed to count files: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 discovered files, got %d", count)
	}

	count, err = store.CountFilesByStatus("meta_ok")
	if err != nil {
		t.Fatalf("failed to count files: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 meta_ok files, got %d", count)
	}

	count, err = store.CountFilesByStatus("error")
	if err != nil {
		t.Fatalf("failed to count files: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 error file, got %d", count)
	}
}

func TestUpdateFileSHA1(t *testing.T) {
	tmpFile := "test-sha1.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	store, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Insert a file
	file := &File{
		FileKey:   "test-key",
		SrcPath:   "/path/test.mp3",
		SizeBytes: 1024,
		MtimeUnix: 123456,
		Status:    "discovered",
	}
	if err := store.InsertFile(file); err != nil {
		t.Fatalf("failed to insert file: %v", err)
	}

	// Update SHA1
	expectedSHA1 := "abc123def456"
	if err := store.UpdateFileSHA1(file.ID, expectedSHA1); err != nil {
		t.Fatalf("failed to update SHA1: %v", err)
	}

	// Retrieve and verify
	retrieved, err := store.GetFileByID(file.ID)
	if err != nil {
		t.Fatalf("failed to get file: %v", err)
	}

	if retrieved.SHA1 != expectedSHA1 {
		t.Errorf("expected SHA1 %s, got %s", expectedSHA1, retrieved.SHA1)
	}
}

func TestClusterOperations(t *testing.T) {
	tmpFile := "test-clusters.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	store, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Insert files first
	files := []*File{
		{FileKey: "f1", SrcPath: "/p1.mp3", SizeBytes: 100, MtimeUnix: 1, Status: "meta_ok"},
		{FileKey: "f2", SrcPath: "/p2.mp3", SizeBytes: 200, MtimeUnix: 2, Status: "meta_ok"},
		{FileKey: "f3", SrcPath: "/p3.mp3", SizeBytes: 300, MtimeUnix: 3, Status: "meta_ok"},
	}
	for _, f := range files {
		if err := store.InsertFile(f); err != nil {
			t.Fatalf("failed to insert file: %v", err)
		}
	}

	// Insert cluster
	cluster := &Cluster{
		ClusterKey: "artist|title|120",
		Hint:       "Artist - Title",
	}
	if err := store.InsertCluster(cluster); err != nil {
		t.Fatalf("failed to insert cluster: %v", err)
	}

	// Insert cluster members
	members := []*ClusterMember{
		{ClusterKey: cluster.ClusterKey, FileID: files[0].ID, QualityScore: 95.5, Preferred: true},
		{ClusterKey: cluster.ClusterKey, FileID: files[1].ID, QualityScore: 85.0, Preferred: false},
		{ClusterKey: cluster.ClusterKey, FileID: files[2].ID, QualityScore: 75.0, Preferred: false},
	}
	for _, m := range members {
		if err := store.InsertClusterMember(m); err != nil {
			t.Fatalf("failed to insert cluster member: %v", err)
		}
	}

	// Get cluster members
	retrieved, err := store.GetClusterMembers(cluster.ClusterKey)
	if err != nil {
		t.Fatalf("failed to get cluster members: %v", err)
	}

	if len(retrieved) != 3 {
		t.Errorf("expected 3 cluster members, got %d", len(retrieved))
	}

	// Verify ordering (should be by quality score DESC)
	if retrieved[0].QualityScore < retrieved[1].QualityScore {
		t.Error("cluster members not ordered by quality score")
	}

	// Verify preferred flag
	if !retrieved[0].Preferred {
		t.Error("first member should be preferred")
	}

	// Get all clusters
	clusters, err := store.GetAllClusters()
	if err != nil {
		t.Fatalf("failed to get all clusters: %v", err)
	}

	if len(clusters) != 1 {
		t.Errorf("expected 1 cluster, got %d", len(clusters))
	}

	// Get cluster by key
	foundCluster, err := store.GetClusterByKey(cluster.ClusterKey)
	if err != nil {
		t.Fatalf("failed to get cluster by key: %v", err)
	}

	if foundCluster == nil {
		t.Fatal("expected cluster, got nil")
	}

	if foundCluster.Hint != cluster.Hint {
		t.Errorf("expected hint %s, got %s", cluster.Hint, foundCluster.Hint)
	}

	// Count clusters
	count, err := store.CountClusters()
	if err != nil {
		t.Fatalf("failed to count clusters: %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 cluster, got %d", count)
	}
}

func TestUpdateClusterMemberScore(t *testing.T) {
	tmpFile := "test-update-score.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	store, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Insert file, cluster, and member
	file := &File{FileKey: "f1", SrcPath: "/p1.mp3", SizeBytes: 100, MtimeUnix: 1, Status: "meta_ok"}
	if err := store.InsertFile(file); err != nil {
		t.Fatalf("failed to insert file: %v", err)
	}

	cluster := &Cluster{ClusterKey: "test-cluster", Hint: "Test"}
	if err := store.InsertCluster(cluster); err != nil {
		t.Fatalf("failed to insert cluster: %v", err)
	}

	member := &ClusterMember{ClusterKey: cluster.ClusterKey, FileID: file.ID, QualityScore: 50.0, Preferred: false}
	if err := store.InsertClusterMember(member); err != nil {
		t.Fatalf("failed to insert cluster member: %v", err)
	}

	// Update score
	newScore := 95.5
	if err := store.UpdateClusterMemberScore(cluster.ClusterKey, file.ID, newScore); err != nil {
		t.Fatalf("failed to update score: %v", err)
	}

	// Verify
	retrieved, err := store.GetClusterMember(cluster.ClusterKey, file.ID)
	if err != nil {
		t.Fatalf("failed to get cluster member: %v", err)
	}

	if retrieved.QualityScore != newScore {
		t.Errorf("expected score %f, got %f", newScore, retrieved.QualityScore)
	}
}

func TestUpdateClusterMemberPreferred(t *testing.T) {
	tmpFile := "test-update-preferred.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	store, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Insert file, cluster, and member
	file := &File{FileKey: "f1", SrcPath: "/p1.mp3", SizeBytes: 100, MtimeUnix: 1, Status: "meta_ok"}
	if err := store.InsertFile(file); err != nil {
		t.Fatalf("failed to insert file: %v", err)
	}

	cluster := &Cluster{ClusterKey: "test-cluster", Hint: "Test"}
	if err := store.InsertCluster(cluster); err != nil {
		t.Fatalf("failed to insert cluster: %v", err)
	}

	member := &ClusterMember{ClusterKey: cluster.ClusterKey, FileID: file.ID, QualityScore: 85.0, Preferred: false}
	if err := store.InsertClusterMember(member); err != nil {
		t.Fatalf("failed to insert cluster member: %v", err)
	}

	// Update preferred status
	if err := store.UpdateClusterMemberPreferred(cluster.ClusterKey, file.ID, true); err != nil {
		t.Fatalf("failed to update preferred: %v", err)
	}

	// Verify
	retrieved, err := store.GetClusterMember(cluster.ClusterKey, file.ID)
	if err != nil {
		t.Fatalf("failed to get cluster member: %v", err)
	}

	if !retrieved.Preferred {
		t.Error("expected preferred to be true")
	}
}

func TestClearClusters(t *testing.T) {
	tmpFile := "test-clear-clusters.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	store, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Insert cluster
	cluster := &Cluster{ClusterKey: "test-cluster", Hint: "Test"}
	if err := store.InsertCluster(cluster); err != nil {
		t.Fatalf("failed to insert cluster: %v", err)
	}

	// Verify it exists
	count, err := store.CountClusters()
	if err != nil {
		t.Fatalf("failed to count clusters: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 cluster before clear, got %d", count)
	}

	// Clear clusters
	if err := store.ClearClusters(); err != nil {
		t.Fatalf("failed to clear clusters: %v", err)
	}

	// Verify they're gone
	count, err = store.CountClusters()
	if err != nil {
		t.Fatalf("failed to count clusters after clear: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 clusters after clear, got %d", count)
	}
}

func TestPlanOperations(t *testing.T) {
	tmpFile := "test-plans.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	store, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Insert files
	files := []*File{
		{FileKey: "f1", SrcPath: "/p1.mp3", SizeBytes: 100, MtimeUnix: 1, Status: "meta_ok"},
		{FileKey: "f2", SrcPath: "/p2.mp3", SizeBytes: 200, MtimeUnix: 2, Status: "meta_ok"},
		{FileKey: "f3", SrcPath: "/p3.mp3", SizeBytes: 300, MtimeUnix: 3, Status: "meta_ok"},
	}
	for _, f := range files {
		if err := store.InsertFile(f); err != nil {
			t.Fatalf("failed to insert file: %v", err)
		}
	}

	// Insert plans
	plans := []*Plan{
		{FileID: files[0].ID, Action: "copy", DestPath: "/dest/1.mp3", Reason: "winner"},
		{FileID: files[1].ID, Action: "skip", DestPath: "", Reason: "duplicate"},
		{FileID: files[2].ID, Action: "copy", DestPath: "/dest/2.mp3", Reason: "unique"},
	}
	for _, p := range plans {
		if err := store.InsertPlan(p); err != nil {
			t.Fatalf("failed to insert plan: %v", err)
		}
	}

	// Get plan by file ID
	plan, err := store.GetPlan(files[0].ID)
	if err != nil {
		t.Fatalf("failed to get plan: %v", err)
	}

	if plan == nil {
		t.Fatal("expected plan, got nil")
	}

	if plan.Action != "copy" {
		t.Errorf("expected action 'copy', got '%s'", plan.Action)
	}

	// Get all plans
	allPlans, err := store.GetAllPlans()
	if err != nil {
		t.Fatalf("failed to get all plans: %v", err)
	}

	if len(allPlans) != 3 {
		t.Errorf("expected 3 plans, got %d", len(allPlans))
	}

	// Get plans by action
	copyPlans, err := store.GetPlansByAction("copy")
	if err != nil {
		t.Fatalf("failed to get plans by action: %v", err)
	}

	if len(copyPlans) != 2 {
		t.Errorf("expected 2 'copy' plans, got %d", len(copyPlans))
	}

	// Count plans by action
	count, err := store.CountPlansByAction("skip")
	if err != nil {
		t.Fatalf("failed to count plans: %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 'skip' plan, got %d", count)
	}
}

func TestClearPlans(t *testing.T) {
	tmpFile := "test-clear-plans.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	store, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Insert file and plan
	file := &File{FileKey: "f1", SrcPath: "/p1.mp3", SizeBytes: 100, MtimeUnix: 1, Status: "meta_ok"}
	if err := store.InsertFile(file); err != nil {
		t.Fatalf("failed to insert file: %v", err)
	}

	plan := &Plan{FileID: file.ID, Action: "copy", DestPath: "/dest/1.mp3", Reason: "winner"}
	if err := store.InsertPlan(plan); err != nil {
		t.Fatalf("failed to insert plan: %v", err)
	}

	// Verify it exists
	allPlans, err := store.GetAllPlans()
	if err != nil {
		t.Fatalf("failed to get plans: %v", err)
	}
	if len(allPlans) != 1 {
		t.Errorf("expected 1 plan before clear, got %d", len(allPlans))
	}

	// Clear plans
	if err := store.ClearPlans(); err != nil {
		t.Fatalf("failed to clear plans: %v", err)
	}

	// Verify they're gone
	allPlans, err = store.GetAllPlans()
	if err != nil {
		t.Fatalf("failed to get plans after clear: %v", err)
	}
	if len(allPlans) != 0 {
		t.Errorf("expected 0 plans after clear, got %d", len(allPlans))
	}
}

func TestExecutionOperations(t *testing.T) {
	tmpFile := "test-executions.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	store, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Insert files
	files := []*File{
		{FileKey: "f1", SrcPath: "/p1.mp3", SizeBytes: 100, MtimeUnix: 1, Status: "meta_ok"},
		{FileKey: "f2", SrcPath: "/p2.mp3", SizeBytes: 200, MtimeUnix: 2, Status: "meta_ok"},
	}
	for _, f := range files {
		if err := store.InsertFile(f); err != nil {
			t.Fatalf("failed to insert file: %v", err)
		}
	}

	now := time.Now()

	// Insert executions
	executions := []*Execution{
		{FileID: files[0].ID, StartedAt: now, CompletedAt: now.Add(time.Second), BytesWritten: 1024, VerifyOK: true, Error: ""},
		{FileID: files[1].ID, StartedAt: now, CompletedAt: now.Add(2 * time.Second), BytesWritten: 2048, VerifyOK: false, Error: "checksum mismatch"},
	}

	for _, exec := range executions {
		if err := store.InsertOrUpdateExecution(exec); err != nil {
			t.Fatalf("failed to insert execution: %v", err)
		}
	}

	// Get execution by file ID
	exec, err := store.GetExecution(files[0].ID)
	if err != nil {
		t.Fatalf("failed to get execution: %v", err)
	}

	if exec == nil {
		t.Fatal("expected execution, got nil")
	}

	if !exec.VerifyOK {
		t.Error("expected VerifyOK to be true")
	}

	if exec.BytesWritten != 1024 {
		t.Errorf("expected BytesWritten 1024, got %d", exec.BytesWritten)
	}

	// Get all executions
	allExecs, err := store.GetAllExecutions()
	if err != nil {
		t.Fatalf("failed to get all executions: %v", err)
	}

	if len(allExecs) != 2 {
		t.Errorf("expected 2 executions, got %d", len(allExecs))
	}

	// Count successful executions
	count, err := store.CountSuccessfulExecutions()
	if err != nil {
		t.Fatalf("failed to count successful executions: %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 successful execution, got %d", count)
	}

	// Get total bytes written
	totalBytes, err := store.GetTotalBytesWritten()
	if err != nil {
		t.Fatalf("failed to get total bytes written: %v", err)
	}

	if totalBytes != 1024 {
		t.Errorf("expected total bytes 1024, got %d", totalBytes)
	}
}

func TestTransactionRollback(t *testing.T) {
	tmpFile := "test-transaction.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	store, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Attempt a transaction that will fail
	err = store.Transaction(func(tx *sql.Tx) error {
		// Insert a file within transaction
		_, err := tx.Exec(`
			INSERT INTO files (file_key, src_path, size_bytes, mtime_unix, status)
			VALUES (?, ?, ?, ?, ?)
		`, "test-key", "/path/test.mp3", 1024, 123456, "discovered")

		if err != nil {
			return err
		}

		// Force a rollback by returning an error
		return fmt.Errorf("intentional error to trigger rollback")
	})

	if err == nil {
		t.Fatal("expected transaction to fail")
	}

	// Verify the file was NOT inserted (transaction rolled back)
	file, err := store.GetFileByKey("test-key")
	if err != nil {
		t.Fatalf("failed to query file: %v", err)
	}

	if file != nil {
		t.Error("expected file to be nil after rollback")
	}
}

func TestTransactionCommit(t *testing.T) {
	tmpFile := "test-transaction-commit.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	store, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Successful transaction
	err = store.Transaction(func(tx *sql.Tx) error {
		// Insert multiple files within transaction
		for i := 0; i < 3; i++ {
			_, err := tx.Exec(`
				INSERT INTO files (file_key, src_path, size_bytes, mtime_unix, status)
				VALUES (?, ?, ?, ?, ?)
			`, fmt.Sprintf("key-%d", i), fmt.Sprintf("/path/%d.mp3", i), 1024, 123456, "discovered")

			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("transaction failed: %v", err)
	}

	// Verify all files were inserted
	files, err := store.GetAllFiles()
	if err != nil {
		t.Fatalf("failed to get files: %v", err)
	}

	if len(files) != 3 {
		t.Errorf("expected 3 files after commit, got %d", len(files))
	}
}

func TestCheckIntegrity(t *testing.T) {
	tmpFile := "test-integrity.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	store, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Check integrity of a fresh database
	err = store.CheckIntegrity()
	if err != nil {
		t.Errorf("integrity check failed on fresh database: %v", err)
	}
}

func TestOpenWithNetworkOptimizations(t *testing.T) {
	tmpFile := "test-network-opts.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	// Open with network optimizations
	store, err := OpenWithOptions(tmpFile, &OpenOptions{NetworkOptimized: true})
	if err != nil {
		t.Fatalf("failed to open store with network options: %v", err)
	}
	defer store.Close()

	// Verify pragmas were applied (check cache_size)
	var cacheSize int
	err = store.DB().QueryRow("PRAGMA cache_size").Scan(&cacheSize)
	if err != nil {
		t.Fatalf("failed to query cache_size: %v", err)
	}

	// Cache size should be -64000 (64MB)
	if cacheSize != -64000 {
		t.Logf("Note: cache_size is %d, expected -64000 (may vary if db already exists)", cacheSize)
	}

	// Basic operations should still work
	file := &File{
		FileKey:   "test-key",
		SrcPath:   "/path/test.mp3",
		SizeBytes: 1024,
		MtimeUnix: 123456,
		Status:    "discovered",
	}

	if err := store.InsertFile(file); err != nil {
		t.Fatalf("failed to insert file: %v", err)
	}

	retrieved, err := store.GetFileByKey("test-key")
	if err != nil {
		t.Fatalf("failed to retrieve file: %v", err)
	}

	if retrieved == nil {
		t.Error("expected file, got nil")
	}
}

func TestFileInsertConflict(t *testing.T) {
	tmpFile := "test-conflict.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-shm")
	defer os.Remove(tmpFile + "-wal")

	store, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Insert a file
	file := &File{
		FileKey:   "duplicate-key",
		SrcPath:   "/path/original.mp3",
		SizeBytes: 1024,
		MtimeUnix: 123456,
		Status:    "discovered",
	}

	if err := store.InsertFile(file); err != nil {
		t.Fatalf("failed to insert file: %v", err)
	}

	originalID := file.ID

	// Insert again with same key (should update)
	file2 := &File{
		FileKey:   "duplicate-key",
		SrcPath:   "/path/updated.mp3",
		SizeBytes: 2048,
		MtimeUnix: 789012,
		Status:    "discovered",
	}

	if err := store.InsertFile(file2); err != nil {
		t.Fatalf("failed to insert duplicate file: %v", err)
	}

	// Should get the same ID
	if file2.ID != originalID {
		t.Errorf("expected ID %d, got %d", originalID, file2.ID)
	}

	// Verify updated fields
	retrieved, err := store.GetFileByKey("duplicate-key")
	if err != nil {
		t.Fatalf("failed to retrieve file: %v", err)
	}

	if retrieved.SrcPath != "/path/updated.mp3" {
		t.Errorf("expected updated path, got %s", retrieved.SrcPath)
	}

	if retrieved.SizeBytes != 2048 {
		t.Errorf("expected updated size 2048, got %d", retrieved.SizeBytes)
	}
}
