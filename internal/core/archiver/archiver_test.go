package archiver

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestCalculateDirSize(t *testing.T) {
	tmpDir := t.TempDir()

	// Create file 1: 100 bytes
	f1 := filepath.Join(tmpDir, "file1")
	d1 := make([]byte, 100)
	if err := os.WriteFile(f1, d1, 0644); err != nil {
		t.Fatal(err)
	}

	// Create file 2: 200 bytes in subdir
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	f2 := filepath.Join(subDir, "file2")
	d2 := make([]byte, 200)
	if err := os.WriteFile(f2, d2, 0644); err != nil {
		t.Fatal(err)
	}

	// Create a symlink (should verify it doesn't double count or crash)
	sym := filepath.Join(tmpDir, "symlink")
	if err := os.Symlink(f1, sym); err != nil {
		// Skip if symlinks not supported (e.g. some Windows envs), though we removed windows support
		t.Log("Skipping symlink test due to creation failure")
	}

	// Expected size: 100 + 200 = 300 (Symlinks are not followed for size calc in logic, usually just file info size which is small, or ignored depending on implementation details.
	// Looking at archiver.go: CalculateDirSize uses WalkDir.
	// It checks `d.Type().IsRegular()` -> adds size.
	// Symlinks are `d.Type()&os.ModeSymlink != 0`.
	// The logic calls `calculateDirSize` which:
	// 1. Checks symlink loop.
	// 2. Checks `d.Type().IsRegular()`.
	// Symlinks are NOT regular files. So symlink size itself (path length) is NOT added? 
	// Wait, let's check code: `if d.Type().IsRegular() { size += info.Size() }`. 
	// So only regular files.
	
	expectedSize := int64(300)
	
	size, err := CalculateDirSize(tmpDir)
	if err != nil {
		t.Fatalf("CalculateDirSize failed: %v", err)
	}

	if size != expectedSize {
		t.Errorf("Expected size %d, got %d", expectedSize, size)
	}
}

func TestCompress(t *testing.T) {
	// Source Directory
	srcDir := t.TempDir()
	testData := []byte("Hello Backup Go")
	if err := os.WriteFile(filepath.Join(srcDir, "test.txt"), testData, 0644); err != nil {
		t.Fatal(err)
	}

	// Dest Directory
	dstDir := t.TempDir()
	dstFile := filepath.Join(dstDir, "archive.tar.zst")

	// Run Compress
	origSize, compSize, err := Compress(srcDir, dstFile)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	if origSize != int64(len(testData)) {
		t.Errorf("Expected original size %d, got %d", len(testData), origSize)
	}
	if compSize == 0 {
		t.Error("Compressed size is 0")
	}

	// Verify File Exists
	if _, err := os.Stat(dstFile); os.IsNotExist(err) {
		t.Fatal("Destination file not created")
	}

	// Verify Decompression (Integration-style check)
	f, err := os.Open(dstFile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	decoder, err := zstd.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.Close()

	tr := tar.NewReader(decoder)
	
	// Expect to find test.txt
	header, err := tr.Next()
	if err == io.EOF {
		t.Fatal("Archive is empty")
	}
	if err != nil {
		t.Fatal(err)
	}

	if header.Name != "test.txt" {
		t.Errorf("Expected file test.txt, got %s", header.Name)
	}

	content, err := io.ReadAll(tr)
	if err != nil {
		t.Fatal(err)
	}

	if string(content) != string(testData) {
		t.Errorf("Content mismatch. Got %s, want %s", string(content), string(testData))
	}
}
