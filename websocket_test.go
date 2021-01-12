package hub

import (
	"net/http"
	"testing"
)

func TestWebsocket_CheckOrigin(t *testing.T) {
	t.Run("Origin header is empty", func(t *testing.T) {
		r := &http.Request{}
		if ok := checkOrigin(r, make([]string, 0)); !ok {
			t.Fatalf("Expected %t, got %t", true, ok)
		}
	})

	t.Run("Origin header is allowed", func(t *testing.T) {
		allowedOrigins := []string{"localhost:0000"}
		r := &http.Request{Header: map[string][]string{
			"Origin": allowedOrigins,
		}}
		want := true
		if ok := checkOrigin(r, allowedOrigins); ok != want {
			t.Fatalf("Expected %t, got %t", want, ok)
		}
	})

	t.Run("All origins allowed", func(t *testing.T) {
		allowedOrigins := []string{"*"}
		r := &http.Request{Header: map[string][]string{
			"Origin": []string{"localhost:0000"},
		}}
		want := true
		if ok := checkOrigin(r, allowedOrigins); ok != want {
			t.Fatalf("Expected %t, got %t", want, ok)
		}
	})

	t.Run("Origin header is not allowed", func(t *testing.T) {
		allowedOrigins := []string{"localhost:0000"}
		r := &http.Request{Header: map[string][]string{
			"Origin": []string{"localhost:0001"},
		}}
		want := false
		if ok := checkOrigin(r, allowedOrigins); ok != want {
			t.Fatalf("Expected %t, got %t", want, ok)
		}
	})

}
