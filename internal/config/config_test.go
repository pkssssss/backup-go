package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadConfig(t *testing.T) {
	// Create a temp file
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "test_config.toml")

	// Prepare config data
	cfg := &Config{
		Cos: CosConfig{
			SecretID:  "test-id",
			SecretKey: "test-key",
			Bucket:    "bucket-123",
			Region:    "ap-shanghai",
			Prefix:    "backup/",
			KeepDays:  7,
		},
		Backup: BackupConfig{
			DataDir: "/tmp/data",
			Schedule: ScheduleConfig{
				Enabled:  true,
				Hour:     3,
				Minute:   30,
				Timezone: "Asia/Shanghai",
			},
		},
	}

	// Test Save
	if err := SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Test Load
	loadedCfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify content
	if loadedCfg.Cos.SecretID != cfg.Cos.SecretID {
		t.Errorf("Expected SecretID %s, got %s", cfg.Cos.SecretID, loadedCfg.Cos.SecretID)
	}
	if loadedCfg.Backup.Schedule.Hour != cfg.Backup.Schedule.Hour {
		t.Errorf("Expected Hour %d, got %d", cfg.Backup.Schedule.Hour, loadedCfg.Backup.Schedule.Hour)
	}
}

func TestCalculateNextRunTime(t *testing.T) {
	// Since CalculateNextRunTime uses time.Now() internally, we test relative scenarios
	
	now := time.Now()
	
	// Case 1: Schedule is 1 hour later today
	futureHour := now.Hour() + 1
	if futureHour >= 24 {
		// If it's late night, skip this specific sub-test or handle differently, 
		// but for simplicity we just mock a schedule that wraps to tomorrow logic if needed.
		// Let's stick to safe calculation.
		return 
	}
	
	schedule := ScheduleConfig{
		Hour:     futureHour,
		Minute:   now.Minute(),
		Timezone: "", // Local
	}
	
	nextRun := CalculateNextRunTime(schedule)
	
	if nextRun.Day() != now.Day() {
		t.Errorf("Expected run time to be today (Day %d), got Day %d", now.Day(), nextRun.Day())
	}
	
	// Case 2: Schedule is 1 hour ago (should run tomorrow)
	pastHour := now.Hour() - 1
	if pastHour < 0 {
		return
	}
	
	schedule2 := ScheduleConfig{
		Hour:     pastHour,
		Minute:   now.Minute(),
		Timezone: "",
	}
	
	nextRun2 := CalculateNextRunTime(schedule2)
	
	expectedTomorrow := now.Add(24 * time.Hour)
	if nextRun2.Day() != expectedTomorrow.Day() {
		t.Errorf("Expected run time to be tomorrow (Day %d), got Day %d", expectedTomorrow.Day(), nextRun2.Day())
	}
}
