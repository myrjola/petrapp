package ptr_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/ptr"
)

func TestRef(t *testing.T) {
	// Test with string
	t.Run("string", func(t *testing.T) {
		s := "test"
		p := ptr.Ref(s)

		if p == nil {
			t.Fatal("Expected pointer to be non-nil")
		}

		if *p != s {
			t.Errorf("Expected %q, got %q", s, *p)
		}

		// Verify that modifying the original value doesn't affect the pointer
		s = "modified"
		if *p == s {
			t.Errorf("Pointer value should not change when original value is modified")
		}
	})

	// Test with int
	t.Run("int", func(t *testing.T) {
		i := 42
		p := ptr.Ref(i)

		if p == nil {
			t.Fatal("Expected pointer to be non-nil")
		}

		if *p != i {
			t.Errorf("Expected %d, got %d", i, *p)
		}
	})

	// Test with struct
	t.Run("struct", func(t *testing.T) {
		type testStruct struct {
			Name string
			Age  int
		}

		ts := testStruct{Name: "Test", Age: 30}
		p := ptr.Ref(ts)

		if p == nil {
			t.Fatal("Expected pointer to be non-nil")
		}

		if p.Name != ts.Name || p.Age != ts.Age {
			t.Errorf("Expected %+v, got %+v", ts, *p)
		}
	})
}
