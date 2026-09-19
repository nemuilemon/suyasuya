package service

import (
	"suyasuya/internal/domain"
	"testing"
)

func TestDeleteJobsRequiresExplicitSelection(t *testing.T) {
	id := "bb3b90e1-cd83-4a83-a960-bfe8786dcc87"
	cases := []struct {
		input domain.DeleteJobsInput
		valid bool
	}{
		{domain.DeleteJobsInput{}, false},
		{domain.DeleteJobsInput{IDs: []string{"invalid"}}, false},
		{domain.DeleteJobsInput{IDs: []string{id}, Completed: true}, false},
		{domain.DeleteJobsInput{IDs: []string{id}}, true},
		{domain.DeleteJobsInput{Completed: true}, true},
	}
	for _, c := range cases {
		if err := validateDeleteJobs(c.input); (err == nil) != c.valid {
			t.Fatalf("%+v: %v", c.input, err)
		}
	}
}
