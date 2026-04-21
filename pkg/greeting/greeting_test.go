package greeting

import "testing"

func TestMessage(t *testing.T) {
	t.Parallel()

	got := Message("Go", "Cobra")
	want := "Hello, Go from Cobra!"
	if got != want {
		t.Fatalf("Message() = %q, want %q", got, want)
	}
}

func TestMessageDefaults(t *testing.T) {
	t.Parallel()

	got := Message(" ", " ")
	want := "Hello, World from bu1ld!"
	if got != want {
		t.Fatalf("Message() = %q, want %q", got, want)
	}
}
