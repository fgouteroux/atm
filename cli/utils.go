// Copyright 2018 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"context"

	"github.com/alecthomas/kingpin/v2"

	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/pkg/labels"
)

// TypeMatchers only valid for when you are going to add a silence.
func TypeMatchers(matchers []labels.Matcher) models.Matchers {
	typeMatchers := make(models.Matchers, len(matchers))
	for i, matcher := range matchers {
		typeMatchers[i] = TypeMatcher(matcher)
	}
	return typeMatchers
}

// TypeMatcher only valid for when you are going to add a silence.
func TypeMatcher(matcher labels.Matcher) *models.Matcher {
	name := matcher.Name
	value := matcher.Value
	typeMatcher := models.Matcher{
		Name:  &name,
		Value: &value,
	}

	isEqual := (matcher.Type == labels.MatchEqual) || (matcher.Type == labels.MatchRegexp)
	isRegex := (matcher.Type == labels.MatchRegexp) || (matcher.Type == labels.MatchNotRegexp)
	typeMatcher.IsEqual = &isEqual
	typeMatcher.IsRegex = &isRegex
	return &typeMatcher
}

// Helper function for adding the ctx with timeout into an action.
func execWithTimeout(fn func(context.Context, *kingpin.ParseContext) error) func(*kingpin.ParseContext) error {
	return func(x *kingpin.ParseContext) error {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return fn(ctx, x)
	}
}
