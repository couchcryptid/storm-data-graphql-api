package model_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/couchcryptid/storm-data-api/internal/model"
)

func loadMockData(t *testing.T) []model.StormReport {
	t.Helper()
	data, err := os.ReadFile("../../data/mock/storm_reports_240426_transformed.json")
	if err != nil {
		t.Fatalf("read mock data: %v", err)
	}
	var reports []model.StormReport
	if err := json.Unmarshal(data, &reports); err != nil {
		t.Fatalf("unmarshal mock data: %v", err)
	}
	return reports
}

func TestLoadMockData(t *testing.T) {
	reports := loadMockData(t)
	if len(reports) != 271 {
		t.Fatalf("expected 271 reports, got %d", len(reports))
	}
}

func TestMockDataTypes(t *testing.T) {
	reports := loadMockData(t)

	counts := map[string]int{}
	for _, r := range reports {
		counts[r.EventType]++
	}

	if counts["hail"] != 79 {
		t.Errorf("expected 79 hail reports, got %d", counts["hail"])
	}
	if counts["tornado"] != 149 {
		t.Errorf("expected 149 tornado reports, got %d", counts["tornado"])
	}
	if counts["wind"] != 43 {
		t.Errorf("expected 43 wind reports, got %d", counts["wind"])
	}
}

func TestMockDataFields(t *testing.T) {
	reports := loadMockData(t)

	for _, r := range reports {
		if r.ID == "" {
			t.Error("expected non-empty ID")
		}
		if r.Geo.Lat == 0 && r.Geo.Lon == 0 {
			t.Errorf("report %s has zero coordinates", r.ID)
		}
		if r.Location.Name == "" {
			t.Errorf("report %s has empty location name", r.ID)
		}
		if r.Location.State == "" {
			t.Errorf("report %s has empty state", r.ID)
		}
		if r.BeginTime.IsZero() {
			t.Errorf("report %s has zero begin_time", r.ID)
		}
		if r.SourceOffice == "" {
			t.Errorf("report %s has empty source_office", r.ID)
		}
	}
}

func TestMockDataOptionalFields(t *testing.T) {
	reports := loadMockData(t)

	var withSeverity, withDistance int
	for _, r := range reports {
		if r.Measurement.Severity != nil {
			withSeverity++
		}
		if r.Location.Distance != nil {
			withDistance++
		}
	}

	if withSeverity == 0 {
		t.Error("expected at least some reports with severity")
	}
	if withSeverity == len(reports) {
		t.Error("expected some reports without severity")
	}
	if withDistance == 0 {
		t.Error("expected at least some reports with distance")
	}
}

func TestMockDataHailReport(t *testing.T) {
	reports := loadMockData(t)

	// Find the 1.25" San Saba hail report (8 ESE Chappel, TX).
	var first model.StormReport
	for _, r := range reports {
		if r.EventType == "hail" && r.Measurement.Magnitude == 1.25 && r.Location.County == "San Saba" {
			first = r
			break
		}
	}

	if first.ID == "" {
		t.Fatal("San Saba 1.25in hail report not found")
	}
	if first.EventType != "hail" {
		t.Errorf("expected type hail, got %s", first.EventType)
	}
	if first.Measurement.Magnitude != 1.25 {
		t.Errorf("expected magnitude 1.25, got %f", first.Measurement.Magnitude)
	}
	if first.Measurement.Unit != "in" {
		t.Errorf("expected unit in, got %s", first.Measurement.Unit)
	}
	if first.Geo.Lat != 31.02 {
		t.Errorf("expected lat 31.02, got %f", first.Geo.Lat)
	}
	if first.Location.State != "TX" {
		t.Errorf("expected state TX, got %s", first.Location.State)
	}
}
