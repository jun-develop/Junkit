package database_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jun-develop/junkit/database"
)

// errDuplicate stands in for a driver's unique-violation error.
var errDuplicate = errors.New("fake driver: duplicate key")

func init() {
	database.RegisterClassifier(func(err error) (database.ErrorType, string, bool) {
		if errors.Is(err, errDuplicate) {
			return database.ErrorUniqueViolation, "duplicate", true
		}

		return database.ErrorNone, "", false
	})
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantType   database.ErrorType
		wantDetail string
	}{
		{name: "nil error", err: nil, wantType: database.ErrorNone, wantDetail: ""},
		{name: "a registered classifier recognizes its error", err: errDuplicate, wantType: database.ErrorUniqueViolation, wantDetail: "duplicate"},
		{name: "wrapped errors reach the classifier", err: fmt.Errorf("insert: %w", errDuplicate), wantType: database.ErrorUniqueViolation, wantDetail: "duplicate"},
		{name: "context deadline exceeded", err: context.DeadlineExceeded, wantType: database.ErrorConnection, wantDetail: "query timeout or canceled"},
		{name: "context canceled", err: context.Canceled, wantType: database.ErrorConnection, wantDetail: "query timeout or canceled"},
		{name: "an error no classifier recognizes is unknown", err: errors.New("something else broke"), wantType: database.ErrorUnknown, wantDetail: "something else broke"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotDetail := database.Classify(tt.err)

			if gotType != tt.wantType || gotDetail != tt.wantDetail {
				t.Fatalf("Classify() = %q, %q, want %q, %q", gotType, gotDetail, tt.wantType, tt.wantDetail)
			}
		})
	}
}

func TestMatch(t *testing.T) {
	errEmailExists := errors.New("email already exists")
	mapping := map[database.ErrorType]error{database.ErrorUniqueViolation: errEmailExists}

	if got := database.Match(errDuplicate, mapping); !errors.Is(got, errEmailExists) {
		t.Fatalf("Match() = %v, want %v", got, errEmailExists)
	}

	if !database.IsUniqueViolation(errDuplicate) {
		t.Fatal("IsUniqueViolation() = false for an error the registered classifier maps to a unique violation")
	}
}
