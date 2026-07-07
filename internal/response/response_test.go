package response

import "testing"

func TestHelpersSetExpectedStatuses(t *testing.T) {
	cases := []struct {
		name string
		got  Result
		want Status
	}{
		{"success", Success("ok"), StatusSuccess},
		{"empty", Empty("none"), StatusEmpty},
		{"not_found", NotFound("missing"), StatusNotFound},
		{"error", Error("failed"), StatusError},
		{"source_required", SourceRequired("pick a source"), StatusSourceRequired},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got.Status != tc.want {
				t.Fatalf("Status = %q, want %q", tc.got.Status, tc.want)
			}
			if tc.got.Message == "" {
				t.Fatal("Message is empty, want supplied message preserved")
			}
		})
	}
}

func TestStatusForCount(t *testing.T) {
	if got := StatusForCount(0); got != StatusEmpty {
		t.Fatalf("StatusForCount(0) = %q, want %q", got, StatusEmpty)
	}
	if got := StatusForCount(1); got != StatusSuccess {
		t.Fatalf("StatusForCount(1) = %q, want %q", got, StatusSuccess)
	}
}
