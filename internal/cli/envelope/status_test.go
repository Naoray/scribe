package envelope

import "testing"

func TestStatusIsValid(t *testing.T) {
	valid := []Status{StatusOK, StatusPartialSuccess, StatusAlreadyInstalled, StatusNoChange, StatusError}
	for _, status := range valid {
		if !status.IsValid() {
			t.Fatalf("%q should be valid", status)
		}
	}

	invalid := []Status{"OK", "Success", ""}
	for _, status := range invalid {
		if status.IsValid() {
			t.Fatalf("%q should be invalid", status)
		}
	}
}
