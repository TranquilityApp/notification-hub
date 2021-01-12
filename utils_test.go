package hub

import "testing"

func TestUtils_ContainsString(t *testing.T) {
	t.Run("Test slice that does contain string", func(t *testing.T) {
		target := "target"
		source := []string{"target", "target2"}
		if ok := containsString(target, source); !ok {
			t.Fatalf("Unable to find string %s in slice", target)
		}
	})

	t.Run("Test slice that does not contain string", func(t *testing.T) {
		target := "doesntExist"
		source := []string{"target", "target2"}
		if ok := containsString(target, source); ok {
			t.Fatalf("Unable to find string %s in slice", target)
		}
	})
}
