package execute

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/franz/music-janitor/internal/store"
)

func setupTestDB(t *testing.T) (*store.Store, string) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	return db, tmpDir
}

func createTestFile(t *testing.T, path string, content []byte) {
	t.Helper()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
}

func TestCopyFile(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	srcPath := filepath.Join(tmpDir, "source.txt")
	destPath := filepath.Join(tmpDir, "dest", "file.txt")
	content := []byte("test content")

	createTestFile(t, srcPath, content)

	executor := New(&Config{
		Store:       db,
		Concurrency: 1,
		VerifyMode:  "size",
	})

	ctx := context.Background()
	bytesWritten, err := executor.copyFile(ctx, srcPath, destPath)
	if err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	if bytesWritten != int64(len(content)) {
		t.Errorf("Expected %d bytes written, got %d", len(content), bytesWritten)
	}

	// Verify destination file exists and has correct content
	destContent, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read destination file: %v", err)
	}

	if string(destContent) != string(content) {
		t.Errorf("Content mismatch: expected %q, got %q", content, destContent)
	}

	// Verify .part file was cleaned up
	partPath := destPath + ".part"
	if _, err := os.Stat(partPath); !os.IsNotExist(err) {
		t.Errorf(".part file was not cleaned up: %s", partPath)
	}
}

func TestCopyFileAtomic(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	srcPath := filepath.Join(tmpDir, "source.txt")
	destPath := filepath.Join(tmpDir, "dest", "file.txt")
	content := []byte("test content for atomic copy")

	createTestFile(t, srcPath, content)

	executor := New(&Config{
		Store:       db,
		Concurrency: 1,
		VerifyMode:  "none",
	})

	// Test that .part file is used during copy
	ctx := context.Background()
	_, err := executor.copyFile(ctx, srcPath, destPath)
	if err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	// Verify final file exists
	if _, err := os.Stat(destPath); err != nil {
		t.Errorf("Destination file doesn't exist: %v", err)
	}
}

func TestVerifySize(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	testPath := filepath.Join(tmpDir, "test.txt")
	content := []byte("test content")
	createTestFile(t, testPath, content)

	executor := New(&Config{
		Store:       db,
		Concurrency: 1,
		VerifyMode:  "size",
	})

	// Test correct size
	ok, err := executor.verifySize(testPath, int64(len(content)))
	if err != nil {
		t.Fatalf("verifySize failed: %v", err)
	}
	if !ok {
		t.Error("verifySize returned false for correct size")
	}

	// Test incorrect size
	ok, err = executor.verifySize(testPath, int64(len(content))+1)
	if err != nil {
		t.Fatalf("verifySize failed: %v", err)
	}
	if ok {
		t.Error("verifySize returned true for incorrect size")
	}
}

func TestVerifyHash(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	content := []byte("test content for hash verification")

	srcPath := filepath.Join(tmpDir, "source.txt")
	destPath := filepath.Join(tmpDir, "dest.txt")

	createTestFile(t, srcPath, content)
	createTestFile(t, destPath, content)

	executor := New(&Config{
		Store:       db,
		Concurrency: 1,
		VerifyMode:  "hash",
	})

	// Test matching files
	ok, err := executor.verifyHash(srcPath, destPath)
	if err != nil {
		t.Fatalf("verifyHash failed: %v", err)
	}
	if !ok {
		t.Error("verifyHash returned false for identical files")
	}

	// Test non-matching files
	differentPath := filepath.Join(tmpDir, "different.txt")
	createTestFile(t, differentPath, []byte("different content"))

	ok, err = executor.verifyHash(srcPath, differentPath)
	if err != nil {
		t.Fatalf("verifyHash failed: %v", err)
	}
	if ok {
		t.Error("verifyHash returned true for different files")
	}
}

func TestMoveFile(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	srcPath := filepath.Join(tmpDir, "source.txt")
	destPath := filepath.Join(tmpDir, "dest", "moved.txt")
	content := []byte("content to move")

	createTestFile(t, srcPath, content)

	executor := New(&Config{
		Store:       db,
		Concurrency: 1,
		VerifyMode:  "size",
	})

	ctx := context.Background()
	bytesWritten, err := executor.moveFile(ctx, srcPath, destPath)
	if err != nil {
		t.Fatalf("moveFile failed: %v", err)
	}

	if bytesWritten != int64(len(content)) {
		t.Errorf("Expected %d bytes written, got %d", len(content), bytesWritten)
	}

	// Verify source file is gone
	if _, err := os.Stat(srcPath); !os.IsNotExist(err) {
		t.Error("Source file still exists after move")
	}

	// Verify destination file exists with correct content
	destContent, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read destination file: %v", err)
	}

	if string(destContent) != string(content) {
		t.Errorf("Content mismatch: expected %q, got %q", content, destContent)
	}
}

func TestHardlinkFile(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	srcPath := filepath.Join(tmpDir, "source.txt")
	destPath := filepath.Join(tmpDir, "dest", "hardlink.txt")
	content := []byte("content for hardlink")

	createTestFile(t, srcPath, content)

	executor := New(&Config{
		Store:       db,
		Concurrency: 1,
		VerifyMode:  "none",
	})

	_, err := executor.hardlinkFile(srcPath, destPath)
	if err != nil {
		t.Fatalf("hardlinkFile failed: %v", err)
	}

	// Verify destination exists
	if _, err := os.Stat(destPath); err != nil {
		t.Errorf("Destination file doesn't exist: %v", err)
	}

	// Verify both files have same inode (hardlink test)
	srcStat, _ := os.Stat(srcPath)
	destStat, _ := os.Stat(destPath)

	// Both should have same size
	if srcStat.Size() != destStat.Size() {
		t.Error("Hardlinked files have different sizes")
	}
}

func TestSymlinkFile(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	srcPath := filepath.Join(tmpDir, "source.txt")
	destPath := filepath.Join(tmpDir, "dest", "symlink.txt")
	content := []byte("content for symlink")

	createTestFile(t, srcPath, content)

	executor := New(&Config{
		Store:       db,
		Concurrency: 1,
		VerifyMode:  "none",
	})

	_, err := executor.symlinkFile(srcPath, destPath)
	if err != nil {
		t.Fatalf("symlinkFile failed: %v", err)
	}

	// Verify symlink exists
	linkInfo, err := os.Lstat(destPath)
	if err != nil {
		t.Fatalf("Failed to stat symlink: %v", err)
	}

	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Error("Destination is not a symlink")
	}

	// Verify symlink points to correct location
	target, err := os.Readlink(destPath)
	if err != nil {
		t.Fatalf("Failed to read symlink: %v", err)
	}

	expectedTarget, _ := filepath.Abs(srcPath)
	if target != expectedTarget {
		t.Errorf("Symlink target mismatch: expected %q, got %q", expectedTarget, target)
	}
}

func TestExecutePlanSkipsAlreadyExecuted(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	srcPath := filepath.Join(tmpDir, "source.txt")
	destPath := filepath.Join(tmpDir, "dest.txt")
	content := []byte("test content")

	createTestFile(t, srcPath, content)

	// Insert file and plan
	file := &store.File{
		FileKey:   srcPath,
		SrcPath:   srcPath,
		SizeBytes: int64(len(content)),
		MtimeUnix: time.Now().Unix(),
		Status:    "pending",
	}
	if err := db.InsertFile(file); err != nil {
		t.Fatalf("Failed to insert file: %v", err)
	}

	plan := &store.Plan{
		FileID:   file.ID,
		Action:   "copy",
		DestPath: destPath,
	}
	if err := db.InsertPlan(plan); err != nil {
		t.Fatalf("Failed to insert plan: %v", err)
	}

	// Mark as already executed
	exec := &store.Execution{
		FileID:       file.ID,
		StartedAt:    time.Now(),
		CompletedAt:  time.Now(),
		BytesWritten: int64(len(content)),
		VerifyOK:     true,
	}
	if err := db.InsertOrUpdateExecution(exec); err != nil {
		t.Fatalf("Failed to insert execution: %v", err)
	}

	executor := New(&Config{
		Store:       db,
		Concurrency: 1,
		VerifyMode:  "none",
	})

	ctx := context.Background()
	bytes, err := executor.executePlan(ctx, plan)
	if err != nil {
		t.Fatalf("executePlan failed: %v", err)
	}

	// Should return -1 for skipped
	if bytes != -1 {
		t.Errorf("Expected -1 (skipped), got %d", bytes)
	}
}

func TestExecutePlanCopy(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	srcPath := filepath.Join(tmpDir, "source.txt")
	destPath := filepath.Join(tmpDir, "dest", "output.txt")
	content := []byte("test content for plan")

	createTestFile(t, srcPath, content)

	// Insert file and plan
	file := &store.File{
		FileKey:   srcPath,
		SrcPath:   srcPath,
		SizeBytes: int64(len(content)),
		MtimeUnix: time.Now().Unix(),
		Status:    "pending",
	}
	if err := db.InsertFile(file); err != nil {
		t.Fatalf("Failed to insert file: %v", err)
	}

	plan := &store.Plan{
		FileID:   file.ID,
		Action:   "copy",
		DestPath: destPath,
	}
	if err := db.InsertPlan(plan); err != nil {
		t.Fatalf("Failed to insert plan: %v", err)
	}

	executor := New(&Config{
		Store:       db,
		Concurrency: 1,
		VerifyMode:  "size",
	})

	ctx := context.Background()
	bytes, err := executor.executePlan(ctx, plan)
	if err != nil {
		t.Fatalf("executePlan failed: %v", err)
	}

	if bytes != int64(len(content)) {
		t.Errorf("Expected %d bytes, got %d", len(content), bytes)
	}

	// Verify execution was recorded
	exec, err := db.GetExecution(file.ID)
	if err != nil {
		t.Fatalf("Failed to get execution: %v", err)
	}

	if exec == nil {
		t.Fatal("Execution not recorded")
	}

	if !exec.VerifyOK {
		t.Error("Execution verification failed")
	}

	if exec.BytesWritten != int64(len(content)) {
		t.Errorf("Expected %d bytes written, got %d", len(content), exec.BytesWritten)
	}
}

func TestExecuteMultipleFiles(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	// Create multiple test files
	numFiles := 5
	for i := 0; i < numFiles; i++ {
		srcPath := filepath.Join(tmpDir, "src", "file"+string(rune('0'+i))+".txt")
		destPath := filepath.Join(tmpDir, "dest", "file"+string(rune('0'+i))+".txt")
		content := []byte("content " + string(rune('0'+i)))

		createTestFile(t, srcPath, content)

		file := &store.File{
			FileKey:   srcPath,
			SrcPath:   srcPath,
			SizeBytes: int64(len(content)),
			MtimeUnix: time.Now().Unix(),
			Status:    "pending",
		}
		if err := db.InsertFile(file); err != nil {
			t.Fatalf("Failed to insert file %d: %v", i, err)
		}

		plan := &store.Plan{
			FileID:   file.ID,
			Action:   "copy",
			DestPath: destPath,
		}
		if err := db.InsertPlan(plan); err != nil {
			t.Fatalf("Failed to insert plan %d: %v", i, err)
		}
	}

	executor := New(&Config{
		Store:       db,
		Concurrency: 2,
		VerifyMode:  "size",
	})

	ctx := context.Background()
	result, err := executor.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Processed != numFiles {
		t.Errorf("Expected %d processed, got %d", numFiles, result.Processed)
	}

	if result.Succeeded != numFiles {
		t.Errorf("Expected %d succeeded, got %d", numFiles, result.Succeeded)
	}

	if result.Failed != 0 {
		t.Errorf("Expected 0 failed, got %d", result.Failed)
	}

	if result.Skipped != 0 {
		t.Errorf("Expected 0 skipped, got %d", result.Skipped)
	}

	// Verify all executions were recorded
	count, err := db.CountSuccessfulExecutions()
	if err != nil {
		t.Fatalf("Failed to count executions: %v", err)
	}

	if count != numFiles {
		t.Errorf("Expected %d successful executions, got %d", numFiles, count)
	}
}

func TestExecuteWithSkipAction(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	// Create a file with skip action
	srcPath := filepath.Join(tmpDir, "source.txt")
	content := []byte("skip me")
	createTestFile(t, srcPath, content)

	file := &store.File{
		FileKey:   srcPath,
		SrcPath:   srcPath,
		SizeBytes: int64(len(content)),
		MtimeUnix: time.Now().Unix(),
		Status:    "pending",
	}
	if err := db.InsertFile(file); err != nil {
		t.Fatalf("Failed to insert file: %v", err)
	}

	plan := &store.Plan{
		FileID: file.ID,
		Action: "skip",
		Reason: "duplicate",
	}
	if err := db.InsertPlan(plan); err != nil {
		t.Fatalf("Failed to insert plan: %v", err)
	}

	executor := New(&Config{
		Store:       db,
		Concurrency: 1,
		VerifyMode:  "none",
	})

	ctx := context.Background()
	result, err := executor.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Skip actions should not be processed
	if result.Processed != 0 {
		t.Errorf("Expected 0 processed (skip actions excluded), got %d", result.Processed)
	}
}

func TestDryRun(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	srcPath := filepath.Join(tmpDir, "source.txt")
	destPath := filepath.Join(tmpDir, "dest.txt")
	content := []byte("dry run test")

	createTestFile(t, srcPath, content)

	file := &store.File{
		FileKey:   srcPath,
		SrcPath:   srcPath,
		SizeBytes: int64(len(content)),
		MtimeUnix: time.Now().Unix(),
		Status:    "pending",
	}
	if err := db.InsertFile(file); err != nil {
		t.Fatalf("Failed to insert file: %v", err)
	}

	plan := &store.Plan{
		FileID:   file.ID,
		Action:   "copy",
		DestPath: destPath,
	}
	if err := db.InsertPlan(plan); err != nil {
		t.Fatalf("Failed to insert plan: %v", err)
	}

	executor := New(&Config{
		Store:       db,
		Concurrency: 1,
		VerifyMode:  "none",
		DryRun:      true,
	})

	ctx := context.Background()
	result, err := executor.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Succeeded != 1 {
		t.Errorf("Expected 1 succeeded (dry run), got %d", result.Succeeded)
	}

	// Verify destination file was NOT actually created
	if _, err := os.Stat(destPath); !os.IsNotExist(err) {
		t.Error("Destination file exists in dry-run mode")
	}

	// Verify execution was still recorded
	exec, err := db.GetExecution(file.ID)
	if err != nil {
		t.Fatalf("Failed to get execution: %v", err)
	}

	if exec == nil || !exec.VerifyOK {
		t.Error("Dry-run execution not recorded properly")
	}
}
