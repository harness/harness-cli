package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsWildCardExpression(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		wantOK  bool
		wantErr bool
	}{
		{"plain name", "express", false, false},
		{"star", "express*", true, false},
		{"question mark", "?express", true, false},
		{"both star and question", "ex*pr?ess", true, false},
		{"empty", "", false, false},
		{"bracket open unsupported", "ex[press", false, true},
		{"bracket close unsupported", "ex]press", false, true},
		{"brace open unsupported", "ex{press", false, true},
		{"brace close unsupported", "ex}press", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := IsWildCardExpression(tt.pattern)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMatchesWildCardPattern(t *testing.T) {
	tests := []struct {
		name    string
		pkg     string
		pattern string
		want    bool
	}{
		{"exact match", "express", "express", true},
		{"prefix star matches", "express-core", "express*", true},
		{"no match", "lodash", "express*", false},
		{"question mark matches single char", "abc", "a?c", true},
		{"question mark fails extra char", "abbc", "a?c", false},
		{"invalid pattern returns false", "anything", "[unclosed", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesWildCardPattern(tt.pkg, tt.pattern)
			assert.Equal(t, tt.want, got)
		})
	}
}
