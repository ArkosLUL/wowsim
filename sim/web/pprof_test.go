package main

import (
	"io"
	"net/http"
	"runtime"
	"strings"
	"testing"
)

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(body)
}

// The server main_test.go starts is the app's port. Its mux must not fall back to
// http.DefaultServeMux, where importing net/http/pprof put the profiling endpoints.
func TestPprofOnlyOnItsOwnListener(t *testing.T) {
	if status, _ := get(t, "http://localhost:3339/debug/pprof/"); status != http.StatusNotFound {
		t.Errorf("the app's port answers /debug/pprof/ with %d, want 404", status)
	}

	listener := servePprof("127.0.0.1:0")
	t.Cleanup(func() {
		listener.Close()
		runtime.SetMutexProfileFraction(0)
		runtime.SetBlockProfileRate(0)
	})
	base := "http://" + listener.Addr().String() + "/debug/pprof/"
	if status, body := get(t, base); status != http.StatusOK || !strings.Contains(body, "goroutine") {
		t.Errorf("the pprof listener answers /debug/pprof/ with %d, want 200 and the profile index", status)
	}
	if status, _ := get(t, base+"goroutine?debug=1"); status != http.StatusOK {
		t.Errorf("the pprof listener answers the goroutine profile with %d, want 200", status)
	}
}
