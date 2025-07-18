package cache

import (
	"testing"
	"time"
)

func TestRandomizerDiversityControls_BasicFunctionality(t *testing.T) {
	config := DefaultDiversityControlsConfig()
	rdc := NewRandomizerDiversityControls(config)
	
	// Test initial state
	metrics := rdc.GetDiversityMetrics()
	if metrics.TotalRandomizers != 0 {
		t.Errorf("Expected 0 randomizers initially, got %d", metrics.TotalRandomizers)
	}
	
	if metrics.HealthStatus != "Healthy" {
		t.Errorf("Expected healthy status initially, got %s", metrics.HealthStatus)
	}
}

func TestRandomizerDiversityControls_RecordSelection(t *testing.T) {
	config := DefaultDiversityControlsConfig()
	rdc := NewRandomizerDiversityControls(config)
	
	// Record some selections
	cids := []string{"cid1", "cid2", "cid3", "cid1", "cid2", "cid1"}
	
	for _, cid := range cids {
		rdc.RecordRandomizerSelection(cid)
	}
	
	// Check metrics
	metrics := rdc.GetDiversityMetrics()
	
	if metrics.TotalRandomizers != 3 {
		t.Errorf("Expected 3 unique randomizers, got %d", metrics.TotalRandomizers)
	}
	
	if metrics.ActiveRandomizers != 3 {
		t.Errorf("Expected 3 active randomizers, got %d", metrics.ActiveRandomizers)
	}
	
	// cid1 is used 3 times out of 6 = 50%
	expectedMaxUsage := 0.5
	if metrics.MaxUsageRatio != expectedMaxUsage {
		t.Errorf("Expected max usage ratio of %f, got %f", expectedMaxUsage, metrics.MaxUsageRatio)
	}
}

func TestRandomizerDiversityControls_ConcentrationDetection(t *testing.T) {
	config := DefaultDiversityControlsConfig()
	config.ConcentrationThreshold = 0.4 // 40% threshold
	config.CriticalThreshold = 0.6      // 60% critical
	
	rdc := NewRandomizerDiversityControls(config)
	
	// Create concentration by heavily using one randomizer
	for i := 0; i < 70; i++ {
		rdc.RecordRandomizerSelection("concentrated_cid")
	}
	
	// Add some diversity
	for i := 0; i < 30; i++ {
		rdc.RecordRandomizerSelection("other_cid")
	}
	
	metrics := rdc.GetDiversityMetrics()
	
	// concentrated_cid has 70% usage - should trigger critical
	if metrics.MaxUsageRatio < 0.6 {
		t.Errorf("Expected max usage ratio >= 0.6, got %f", metrics.MaxUsageRatio)
	}
	
	if metrics.HealthStatus != "Critical" {
		t.Errorf("Expected critical health status, got %s", metrics.HealthStatus)
	}
}

func TestRandomizerDiversityControls_DiversityScoring(t *testing.T) {
	config := DefaultDiversityControlsConfig()
	config.EnableDiversityBoost = true
	config.EnableConcentrationPenalty = true
	
	rdc := NewRandomizerDiversityControls(config)
	
	// Create different usage patterns
	// High usage randomizer
	for i := 0; i < 50; i++ {
		rdc.RecordRandomizerSelection("heavy_use")
	}
	
	// Low usage randomizer
	for i := 0; i < 5; i++ {
		rdc.RecordRandomizerSelection("light_use")
	}
	
	// New randomizer (never used)
	baseScore := 1.0
	
	heavyScore := rdc.CalculateRandomizerScore("heavy_use", baseScore)
	lightScore := rdc.CalculateRandomizerScore("light_use", baseScore)
	newScore := rdc.CalculateRandomizerScore("new_randomizer", baseScore)
	
	// Light usage should get boost, heavy usage should get penalty
	if lightScore <= heavyScore {
		t.Errorf("Light usage randomizer should have higher score than heavy usage")
	}
	
	// New randomizer should get highest score
	if newScore <= lightScore {
		t.Errorf("New randomizer should have highest score")
	}
}

func TestRandomizerDiversityControls_EmergencyMode(t *testing.T) {
	config := DefaultDiversityControlsConfig()
	config.EmergencyDiversityMode = true
	
	rdc := NewRandomizerDiversityControls(config)
	
	// Create some usage
	for i := 0; i < 10; i++ {
		rdc.RecordRandomizerSelection("used_cid")
	}
	
	baseScore := 1.0
	usedScore := rdc.CalculateRandomizerScore("used_cid", baseScore)
	newScore := rdc.CalculateRandomizerScore("new_cid", baseScore)
	
	// In emergency mode, new randomizers should get massive boost
	if newScore <= usedScore*1.5 {
		t.Errorf("Emergency mode should heavily favor new randomizers")
	}
	
	metrics := rdc.GetDiversityMetrics()
	if !metrics.EmergencyMode {
		t.Error("Emergency mode should be reflected in metrics")
	}
}

func TestRandomizerDiversityControls_BlockedRandomizers(t *testing.T) {
	config := DefaultDiversityControlsConfig()
	config.BlockConcentratedRandomizers = true
	config.CriticalThreshold = 0.5
	
	rdc := NewRandomizerDiversityControls(config)
	
	// Create critical concentration
	for i := 0; i < 60; i++ {
		rdc.RecordRandomizerSelection("critical_cid")
	}
	
	for i := 0; i < 40; i++ {
		rdc.RecordRandomizerSelection("normal_cid")
	}
	
	// Update the critical randomizers list manually for testing
	rdc.concentrationTracker.criticalRandomizers["critical_cid"] = true
	
	baseScore := 1.0
	criticalScore := rdc.CalculateRandomizerScore("critical_cid", baseScore)
	normalScore := rdc.CalculateRandomizerScore("normal_cid", baseScore)
	
	// Critical randomizer should be blocked (score = 0)
	if criticalScore != 0.0 {
		t.Errorf("Critical randomizer should be blocked, got score %f", criticalScore)
	}
	
	if normalScore == 0.0 {
		t.Error("Normal randomizer should not be blocked")
	}
}

func TestEntropyCalculator_ShannonEntropy(t *testing.T) {
	ec := NewEntropyCalculator(100)
	
	// Test uniform distribution (maximum entropy)
	// With 4 equal items, entropy should be log2(4) = 2 bits
	selections := []string{"a", "b", "c", "d"}
	for i := 0; i < 40; i++ {
		ec.RecordSelection(selections[i%4])
	}
	
	entropy := ec.GetCurrentEntropy()
	expectedEntropy := 2.0 // log2(4) for perfectly uniform distribution
	tolerance := 0.1
	
	if entropy < expectedEntropy-tolerance || entropy > expectedEntropy+tolerance {
		t.Errorf("Expected entropy around %f, got %f", expectedEntropy, entropy)
	}
	
	// Test concentrated distribution (low entropy)
	ec = NewEntropyCalculator(100)
	
	// 90% one item, 10% another
	for i := 0; i < 90; i++ {
		ec.RecordSelection("concentrated")
	}
	for i := 0; i < 10; i++ {
		ec.RecordSelection("rare")
	}
	
	lowEntropy := ec.GetCurrentEntropy()
	
	if lowEntropy >= expectedEntropy {
		t.Errorf("Concentrated distribution should have lower entropy than uniform")
	}
}

func TestConcentrationTracker_HHI(t *testing.T) {
	ct := NewConcentrationTracker()
	
	// Test perfect competition (low concentration)
	// 4 items with equal shares: HHI = 4 * (0.25)^2 = 0.25
	for i := 0; i < 25; i++ {
		ct.RecordSelection("a")
		ct.RecordSelection("b")  
		ct.RecordSelection("c")
		ct.RecordSelection("d")
	}
	
	hhi := ct.GetConcentrationScore()
	expectedHHI := 0.25
	tolerance := 0.01
	
	if hhi < expectedHHI-tolerance || hhi > expectedHHI+tolerance {
		t.Errorf("Expected HHI around %f, got %f", expectedHHI, hhi)
	}
	
	// Test monopoly (high concentration)
	ct = NewConcentrationTracker()
	
	// One item with 100% share: HHI = 1 * (1.0)^2 = 1.0
	for i := 0; i < 100; i++ {
		ct.RecordSelection("monopoly")
	}
	
	monopolyHHI := ct.GetConcentrationScore()
	expectedMonopolyHHI := 1.0
	
	if monopolyHHI < expectedMonopolyHHI-tolerance {
		t.Errorf("Expected monopoly HHI around %f, got %f", expectedMonopolyHHI, monopolyHHI)
	}
	
	if monopolyHHI <= hhi {
		t.Error("Monopoly should have higher concentration than competition")
	}
}

func TestRandomizerDiversityControls_Cleanup(t *testing.T) {
	config := DefaultDiversityControlsConfig()
	config.CleanupInterval = 100 * time.Millisecond // Very short for testing
	config.UsageHistoryWindow = 200 * time.Millisecond
	
	rdc := NewRandomizerDiversityControls(config)
	
	// Record old selections
	rdc.RecordRandomizerSelection("old_cid")
	
	// Wait for history window to expire
	time.Sleep(300 * time.Millisecond)
	
	// Record new selection to trigger cleanup
	rdc.RecordRandomizerSelection("new_cid")
	
	// Check that cleanup occurred
	metrics := rdc.GetDiversityMetrics()
	
	// Should only have new_cid tracked now
	if metrics.TotalRandomizers != 1 {
		t.Errorf("Expected 1 randomizer after cleanup, got %d", metrics.TotalRandomizers)
	}
}

func TestRandomizerDiversityControls_UniqueRatio(t *testing.T) {
	config := DefaultDiversityControlsConfig()
	rdc := NewRandomizerDiversityControls(config)
	
	// Record 10 selections with 5 unique randomizers
	// This gives unique ratio of 5/10 = 0.5
	for i := 0; i < 2; i++ {
		rdc.RecordRandomizerSelection("cid1")
		rdc.RecordRandomizerSelection("cid2")
		rdc.RecordRandomizerSelection("cid3") 
		rdc.RecordRandomizerSelection("cid4")
		rdc.RecordRandomizerSelection("cid5")
	}
	
	metrics := rdc.GetDiversityMetrics()
	expectedRatio := 0.5 // 5 unique / 10 total
	
	if metrics.UniqueRatio != expectedRatio {
		t.Errorf("Expected unique ratio %f, got %f", expectedRatio, metrics.UniqueRatio)
	}
}

func BenchmarkRandomizerDiversityControls_RecordSelection(b *testing.B) {
	config := DefaultDiversityControlsConfig()
	rdc := NewRandomizerDiversityControls(config)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		cid := "cid" + string(rune(i%100)) // 100 different CIDs
		rdc.RecordRandomizerSelection(cid)
	}
}

func BenchmarkRandomizerDiversityControls_CalculateScore(b *testing.B) {
	config := DefaultDiversityControlsConfig()
	rdc := NewRandomizerDiversityControls(config)
	
	// Pre-populate with some data
	for i := 0; i < 1000; i++ {
		cid := "cid" + string(rune(i%50)) // 50 different CIDs
		rdc.RecordRandomizerSelection(cid)
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		cid := "cid" + string(rune(i%50))
		rdc.CalculateRandomizerScore(cid, 1.0)
	}
}

func BenchmarkEntropyCalculator_GetCurrentEntropy(b *testing.B) {
	ec := NewEntropyCalculator(1000)
	
	// Pre-populate with data
	for i := 0; i < 1000; i++ {
		cid := "cid" + string(rune(i%20)) // 20 different CIDs
		ec.RecordSelection(cid)
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		ec.GetCurrentEntropy()
	}
}