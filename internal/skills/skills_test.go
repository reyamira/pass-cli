package skills

import (
	"strings"
	"testing"
)

func TestListReturnsCore(t *testing.T) {
	list, err := List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("List() returned no skills")
	}

	var core *Skill
	for i := range list {
		if list[i].Name == "core" {
			core = &list[i]
		}
	}
	if core == nil {
		t.Fatal("List() did not include the 'core' skill")
	}
	if strings.TrimSpace(core.Description) == "" {
		t.Error("core skill has an empty description (frontmatter not parsed?)")
	}
}

// Every listed skill must be retrievable both plain and --full, with non-empty
// content — guards against a dangling list entry or a broken embed.
func TestEveryListedSkillIsRetrievable(t *testing.T) {
	list, err := List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	for _, s := range list {
		if strings.TrimSpace(s.Name) == "" {
			t.Error("skill with empty name in List()")
			continue
		}
		plain, err := Get(s.Name, false)
		if err != nil {
			t.Errorf("Get(%q, false) error: %v", s.Name, err)
			continue
		}
		if strings.TrimSpace(plain) == "" {
			t.Errorf("Get(%q, false) returned empty content", s.Name)
		}
		full, err := Get(s.Name, true)
		if err != nil {
			t.Errorf("Get(%q, true) error: %v", s.Name, err)
			continue
		}
		// --full must be a superset of the plain body.
		if !strings.HasPrefix(full, strings.TrimRight(plain, "\n")) {
			t.Errorf("Get(%q, true) does not start with the plain body", s.Name)
		}
	}
}

func TestCoreFullIsLongerAndContainsBase(t *testing.T) {
	plain, err := Get("core", false)
	if err != nil {
		t.Fatalf("Get(core,false) error: %v", err)
	}
	full, err := Get("core", true)
	if err != nil {
		t.Fatalf("Get(core,true) error: %v", err)
	}
	if len(full) <= len(plain) {
		t.Errorf("core --full (%d bytes) is not longer than plain (%d bytes)", len(full), len(plain))
	}
	if !strings.Contains(full, "Full command reference") {
		t.Error("core --full is missing the command reference section")
	}
}

func TestGetStripsFrontmatter(t *testing.T) {
	body, err := Get("core", false)
	if err != nil {
		t.Fatalf("Get(core,false) error: %v", err)
	}
	if strings.HasPrefix(body, "---") {
		t.Error("Get() did not strip the leading frontmatter block")
	}
	if strings.Contains(body, "description:") {
		t.Error("Get() leaked a frontmatter field into the body")
	}
}

func TestGetUnknownSkillErrors(t *testing.T) {
	_, err := Get("does-not-exist", false)
	if err == nil {
		t.Fatal("Get() on an unknown skill did not error")
	}
	if !strings.Contains(err.Error(), "core") {
		t.Errorf("unknown-skill error should list valid names; got: %v", err)
	}
}

func TestStubContent(t *testing.T) {
	stub, err := StubContent()
	if err != nil {
		t.Fatalf("StubContent() error: %v", err)
	}
	if strings.TrimSpace(stub) == "" {
		t.Fatal("StubContent() is empty")
	}
	for _, want := range []string{
		"pass-cli skills get core",
		"allowed-tools: Bash(pass-cli:*)",
	} {
		if !strings.Contains(stub, want) {
			t.Errorf("StubContent() missing %q", want)
		}
	}
}

func TestSplitFrontmatter(t *testing.T) {
	fields, body := splitFrontmatter("---\nname: x\ndescription: \"hi there\"\n---\n# Body\ntext\n")
	if fields["name"] != "x" {
		t.Errorf("name = %q, want x", fields["name"])
	}
	if fields["description"] != "hi there" {
		t.Errorf("description = %q, want 'hi there'", fields["description"])
	}
	if !strings.HasPrefix(body, "# Body") {
		t.Errorf("body = %q, want it to start with '# Body'", body)
	}

	// No frontmatter → whole input is the body.
	fields2, body2 := splitFrontmatter("# Just a heading\n")
	if len(fields2) != 0 {
		t.Errorf("expected no fields, got %v", fields2)
	}
	if body2 != "# Just a heading\n" {
		t.Errorf("body2 = %q", body2)
	}
}
