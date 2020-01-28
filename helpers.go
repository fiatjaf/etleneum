package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/itchyny/gojq"
	"github.com/yudai/gojsondiff"
)

var wordMatcher *regexp.Regexp = regexp.MustCompile(`\b\w+\b`)

type Result struct {
	Ok    bool        `json:"ok"`
	Value interface{} `json:"value"`
	Error string      `json:"error,omitempty"`
}

func jsonError(w http.ResponseWriter, message string, code int) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(Result{
		Ok:    false,
		Error: message,
	})
}

func diffDeltaOneliner(prefix string, idelta gojsondiff.Delta) (lines []string) {
	key := prefix
	if key != "" {
		key += "."
	}

	switch pdelta := idelta.(type) {
	case gojsondiff.PreDelta:
		switch delta := pdelta.(type) {
		case *gojsondiff.Moved:
			key = key + delta.PrePosition().String()
			lines = append(lines, fmt.Sprintf("- %s", key))
		case *gojsondiff.Deleted:
			key = key + delta.PrePosition().String()
			lines = append(lines, fmt.Sprintf("- %s", key[:len(key)-1]))
		}
	}

	switch pdelta := idelta.(type) {
	case gojsondiff.PostDelta:
		switch delta := pdelta.(type) {
		case *gojsondiff.TextDiff:
			key = key + delta.PostPosition().String()
			lines = append(lines, fmt.Sprintf("= %s %v", key, delta.NewValue))
		case *gojsondiff.Modified:
			key = key + delta.PostPosition().String()
			value, _ := json.Marshal(delta.NewValue)
			lines = append(lines, fmt.Sprintf("= %s %s", key, value))
		case *gojsondiff.Added:
			key = key + delta.PostPosition().String()
			value, _ := json.Marshal(delta.Value)
			lines = append(lines, fmt.Sprintf("+ %s %s", key, value))
		case *gojsondiff.Object:
			key = key + delta.PostPosition().String()
			for _, nextdelta := range delta.Deltas {
				lines = append(lines, diffDeltaOneliner(key, nextdelta)...)
			}
		case *gojsondiff.Array:
			key = key + delta.PostPosition().String()
			for _, nextdelta := range delta.Deltas {
				lines = append(lines, diffDeltaOneliner(key, nextdelta)...)
			}
		case *gojsondiff.Moved:
			key = key + delta.PostPosition().String()
			value, _ := json.Marshal(delta.Value)
			lines = append(lines, fmt.Sprintf("+ %s %s", key, value))
			if delta.Delta != nil {
				if d, ok := delta.Delta.(gojsondiff.Delta); ok {
					lines = append(lines, diffDeltaOneliner(key, d)...)
				}
			}
		}
	}

	return
}

func runJQ(
	ctx context.Context,
	input []byte,
	filter string,
) (result interface{}, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*2)
	defer cancel()

	query, err := gojq.Parse(filter)
	if err != nil {
		return
	}

	var object map[string]interface{}
	err = json.Unmarshal(input, &object)
	if err != nil {
		return nil, err
	}

	iter := query.RunWithContext(ctx, object)
	v, ok := iter.Next()
	if !ok {
		return nil, nil
	}
	if err, ok := v.(error); ok {
		return nil, err
	}
	return v, nil
}
