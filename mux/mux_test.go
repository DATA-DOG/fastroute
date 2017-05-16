package mux

import "testing"

func TestPathClean(t *testing.T) {

	var cleanTests = []struct {
		path, result string
	}{
		// Already clean
		{"/", "/"},
		{"/abc", "/abc"},
		{"/a/b/c", "/a/b/c"},
		{"/abc/", "/abc/"},
		{"/a/b/c/", "/a/b/c/"},

		// missing root
		{"", "/"},
		{"abc", "/abc"},
		{"abc/def", "/abc/def"},
		{"a/b/c", "/a/b/c"},

		// Remove doubled slash
		{"//", "/"},
		{"/abc//", "/abc/"},
		{"/abc/def//", "/abc/def/"},
		{"/a/b/c//", "/a/b/c/"},
		{"/abc//def//ghi", "/abc/def/ghi"},
		{"//abc", "/abc"},
		{"///abc", "/abc"},
		{"//abc//", "/abc/"},

		// Remove . elements
		{".", "/"},
		{"./", "/"},
		{"/abc/./def", "/abc/def"},
		{"/./abc/def", "/abc/def"},
		{"/abc/.", "/abc/"},

		// Remove .. elements
		{"..", "/"},
		{"../", "/"},
		{"../../", "/"},
		{"../..", "/"},
		{"../../abc", "/abc"},
		{"/abc/def/ghi/../jkl", "/abc/def/jkl"},
		{"/abc/def/../ghi/../jkl", "/abc/jkl"},
		{"/abc/def/..", "/abc"},
		{"/abc/def/../..", "/"},
		{"/abc/def/../../..", "/"},
		{"/abc/def/../../..", "/"},
		{"/abc/def/../../../ghi/jkl/../../../mno", "/mno"},

		// Combinations
		{"abc/./../def", "/def"},
		{"abc//./../def", "/def"},
		{"abc/../../././../def", "/def"},
	}

	for _, test := range cleanTests {
		if s := cleanPath(test.path); s != test.result {
			t.Errorf("CleanPath(%q) = %q, want %q", test.path, s, test.result)
		}
		if s := cleanPath(test.result); s != test.result {
			t.Errorf("CleanPath(%q) = %q, want %q", test.result, s, test.result)
		}
	}
}
