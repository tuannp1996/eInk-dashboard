package config

import "testing"

func TestBasePathValidation(t *testing.T) {
	for _, basePath := range []string{"", "/elink-dashboard", "/nested/dashboard"} {
		cfg := Default()
		cfg.AuthDisabled, cfg.BasePath = true, basePath
		if err := cfg.Validate(); err != nil {
			t.Errorf("valid base path %q: %v", basePath, err)
		}
	}
	for _, basePath := range []string{"/", "dashboard", "//host", "/dashboard/", "/a/../b", "/a?b", "/a#b", `/a\b`, "/a%2fb"} {
		cfg := Default()
		cfg.AuthDisabled, cfg.BasePath = true, basePath
		if err := cfg.Validate(); err == nil {
			t.Errorf("invalid base path accepted: %q", basePath)
		}
	}
}
