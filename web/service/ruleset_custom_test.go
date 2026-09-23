package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCustomRuleListsCRUD(t *testing.T) {
	oldPath := customRuleListsPath
	defer func() { customRuleListsPath = oldPath }()
	customRuleListsPath = filepath.Join(t.TempDir(), "custom_rule_lists.json")

	svc := &RuleSetService{}

	first, err := svc.SaveCustomList(CustomRuleList{
		Name: "AI List",
		URLs: []string{
			"https://example.com/openai.list",
			"https://example.com/claude.list",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || len(first.URLs) != 2 {
		t.Fatalf("unexpected first custom tag: %#v", first)
	}

	second, err := svc.SaveCustomList(CustomRuleList{
		Name: "Video List",
		URLs: []string{"https://example.org/video.list"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == "" || second.ID == first.ID {
		t.Fatalf("invalid second ID: first=%q second=%q", first.ID, second.ID)
	}

	items, err := svc.CustomLists()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d custom tags, want 2", len(items))
	}

	updated, err := svc.SaveCustomList(CustomRuleList{
		ID:   first.ID,
		Name: "AI Rules",
		URLs: []string{
			"https://example.com/openai-v2.list",
			"https://example.com/gemini.list",
			"https://example.com/claude-v2.list",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != first.ID || updated.Name != "AI Rules" || len(updated.URLs) != 3 {
		t.Fatalf("unexpected updated custom tag: %#v", updated)
	}

	items, err = svc.CustomLists()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("update must not create a new item: got %d", len(items))
	}

	if _, err := svc.SaveCustomList(CustomRuleList{
		Name: "Duplicate",
		URLs: []string{second.URLs[0]},
	}); err == nil {
		t.Fatal("expected duplicate URL rejection")
	}

	if err := svc.DeleteCustomList(first.ID); err != nil {
		t.Fatal(err)
	}
	items, err = svc.CustomLists()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != second.ID {
		t.Fatalf("unexpected items after delete: %#v", items)
	}
}

func TestCustomRuleListsRejectPrivateURL(t *testing.T) {
	oldPath := customRuleListsPath
	defer func() { customRuleListsPath = oldPath }()
	customRuleListsPath = filepath.Join(t.TempDir(), "custom_rule_lists.json")

	svc := &RuleSetService{}
	if _, err := svc.SaveCustomList(CustomRuleList{
		Name: "Private",
		URLs: []string{"http://127.0.0.1/private.list"},
	}); err == nil {
		t.Fatal("expected private URL rejection")
	}
	if _, err := svc.SaveCustomList(CustomRuleList{
		Name: "",
		URLs: []string{"https://example.com/rules.list"},
	}); err == nil {
		t.Fatal("expected empty name rejection")
	}
}

func TestCustomRuleListsMigrateLegacySingleURL(t *testing.T) {
	oldPath := customRuleListsPath
	defer func() { customRuleListsPath = oldPath }()
	customRuleListsPath = filepath.Join(t.TempDir(), "custom_rule_lists.json")

	legacy := `[{"id":"old-1","name":"旧收藏","url":"https://example.com/old.list","createdAt":1,"updatedAt":1}]`
	if err := os.WriteFile(customRuleListsPath, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}

	svc := &RuleSetService{}
	items, err := svc.CustomLists()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || len(items[0].URLs) != 1 || items[0].URLs[0] != "https://example.com/old.list" {
		t.Fatalf("legacy URL was not migrated in memory: %#v", items)
	}

	items[0].Name = "旧收藏已编辑"
	if _, err := svc.SaveCustomList(items[0]); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(customRuleListsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"url":`) || !strings.Contains(string(data), `"urls":`) {
		t.Fatalf("legacy format was not rewritten: %s", data)
	}
}

func TestCustomRuleURLLimits(t *testing.T) {
	urls := make([]string, 21)
	for i := range urls {
		urls[i] = fmt.Sprintf("https://example.com/rule-%d.list", i)
	}
	if _, err := normalizeCustomRuleURLsLimit(urls, "", 20); err == nil {
		t.Fatal("expected single custom tag 20 URL limit")
	}
	normalized, err := normalizeCustomRuleURLsLimit(urls, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized) != 21 {
		t.Fatalf("batch normalize got %d URLs, want 21", len(normalized))
	}
}
