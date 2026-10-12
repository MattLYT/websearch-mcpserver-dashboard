package everything

import "testing"

func TestAppendExcludes(t *testing.T) {
	got := AppendExcludes(`d:\code\ai\ *.go`, []string{`\obj\`, `\node_modules\`, `old backup`, "", "  "})
	want := `d:\code\ai\ *.go !\obj\ !\node_modules\ !"old backup"`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	if AppendExcludes(`*.go`, nil) != `*.go` {
		t.Error("无排除项应原样返回")
	}
}

func TestScopeQueryWhitespaceFolder(t *testing.T) {
	for _, tc := range []struct {
		folder string
		want   string
	}{
		{`D:\Scope Fixture Alpha`, `"D:\Scope Fixture Alpha\" pzxm_scope_fixture_1010.txt`},
		{`D:\Scope Fixture Beta Alpha`, `"D:\Scope Fixture Beta Alpha\" pzxm_scope_fixture_1010.txt`},
		{"D:\\Scope\tFixture", "\"D:\\Scope\tFixture\\\" pzxm_scope_fixture_1010.txt"},
	} {
		got, err := ScopeQuery([]string{`D:\`}, tc.folder, "pzxm_scope_fixture_1010.txt")
		if err != nil || got != tc.want {
			t.Errorf("folder %q: got %q, want %q, err=%v", tc.folder, got, tc.want, err)
		}
	}
}

func TestScopeQueryWhitespaceRoot(t *testing.T) {
	got, err := ScopeQuery([]string{`D:\Scope Fixture Alpha`, `D:\CODE\ai`}, "", "alpha|beta")
	want := `"D:\Scope Fixture Alpha\"|D:\CODE\ai\ alpha|beta`
	if err != nil || got != want {
		t.Errorf("got %q, want %q, err=%v", got, want, err)
	}
}
