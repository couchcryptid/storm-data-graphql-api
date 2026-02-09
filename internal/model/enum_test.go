package model_test

import (
	"testing"

	"github.com/couchcryptid/storm-data-graphql-api/internal/model"
)

func TestSortFieldIsValid(t *testing.T) {
	valid := []model.SortField{
		model.SortFieldBeginTime,
		model.SortFieldMagnitude,
		model.SortFieldLocationState,
		model.SortFieldEventType,
	}
	for _, sf := range valid {
		if !sf.IsValid() {
			t.Errorf("expected %q to be valid", sf)
		}
	}

	invalid := []model.SortField{"INVALID", "", "begin_time", "asc"}
	for _, sf := range invalid {
		if sf.IsValid() {
			t.Errorf("expected %q to be invalid", sf)
		}
	}
}

func TestSortFieldString(t *testing.T) {
	tests := []struct {
		field model.SortField
		want  string
	}{
		{model.SortFieldBeginTime, "BEGIN_TIME"},
		{model.SortFieldMagnitude, "MAGNITUDE"},
		{model.SortFieldLocationState, "LOCATION_STATE"},
		{model.SortFieldEventType, "EVENT_TYPE"},
	}
	for _, tt := range tests {
		if got := tt.field.String(); got != tt.want {
			t.Errorf("SortField(%q).String() = %q, want %q", tt.field, got, tt.want)
		}
	}
}

func TestSortOrderIsValid(t *testing.T) {
	if !model.SortOrderAsc.IsValid() {
		t.Error("expected ASC to be valid")
	}
	if !model.SortOrderDesc.IsValid() {
		t.Error("expected DESC to be valid")
	}

	invalid := []model.SortOrder{"INVALID", "", "asc", "desc"}
	for _, so := range invalid {
		if so.IsValid() {
			t.Errorf("expected %q to be invalid", so)
		}
	}
}

func TestSortOrderString(t *testing.T) {
	if got := model.SortOrderAsc.String(); got != "ASC" {
		t.Errorf("SortOrderAsc.String() = %q, want ASC", got)
	}
	if got := model.SortOrderDesc.String(); got != "DESC" {
		t.Errorf("SortOrderDesc.String() = %q, want DESC", got)
	}
}
