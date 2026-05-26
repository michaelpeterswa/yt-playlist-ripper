package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestSanitizeRestricted(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"3Blue1Brown", "3Blue1Brown"},
		{"Tom Scott", "Tom_Scott"},
		{"Bret Devereaux: A Collection of Unmitigated Pedantry", "Bret_Devereaux_A_Collection_of_Unmitigated_Pedantry"},
		{"Café Crème", "Caf_Cr_me"},
		{"--leading and trailing--", "leading_and_trailing"},
		{"slashes/and\\backslashes", "slashes_and_backslashes"},
		{"!!!??? &&&", ""},
		{"with    multiple   spaces", "with_multiple_spaces"},
		{"a.b.c-d_e", "a.b.c-d_e"},
	}
	for _, tc := range cases {
		got := sanitizeRestricted(tc.in)
		if got != tc.want {
			t.Errorf("sanitizeRestricted(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

func TestSanitizeDefault(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Tom Scott", "Tom Scott"},
		{"Bret Devereaux: A Collection of Unmitigated Pedantry", "Bret Devereaux - A Collection of Unmitigated Pedantry"},
		{"slashes/and\\backslashes", "slashes_and_backslashes"},
		{"why? because.", "why because."},
		{"quoted \"thing\"", "quoted 'thing'"},
	}
	for _, tc := range cases {
		got := sanitizeDefault(tc.in)
		if got != tc.want {
			t.Errorf("sanitizeDefault(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

// TestPlanMigrationsLegacyLayout drops two videos in the historical
// "{playlist} - ({uploader})/{date} - {title}/" layout and confirms the
// planner picks them up with the right target dirs and prefix.
func TestPlanMigrationsLegacyLayout(t *testing.T) {
	root := t.TempDir()

	// Video A (uploader: "3Blue1Brown", 2021)
	dirA := filepath.Join(root, "Math - (3Blue1Brown)", "20210101 - Some Math Video")
	if err := os.MkdirAll(dirA, 0o755); err != nil {
		t.Fatal(err)
	}
	baseA := "20210101 - Some Math Video [abc12345678]"
	writeFile(t, filepath.Join(dirA, baseA+".mkv"), "video")
	writeFile(t, filepath.Join(dirA, baseA+".info.json"), `{"id":"abc12345678","uploader":"3Blue1Brown","upload_date":"20210101"}`)
	writeFile(t, filepath.Join(dirA, baseA+".jpg"), "thumb")
	writeFile(t, filepath.Join(dirA, baseA+".en.srt"), "subs")

	// Video B (uploader: "Tom Scott", 2022) — same parent playlist dir
	// in real life would be different, but the planner doesn't care.
	dirB := filepath.Join(root, "Misc - (Tom Scott)", "20220505 - Some Other Video")
	if err := os.MkdirAll(dirB, 0o755); err != nil {
		t.Fatal(err)
	}
	baseB := "20220505 - Some Other Video [def09876543]"
	writeFile(t, filepath.Join(dirB, baseB+".mkv"), "video")
	writeFile(t, filepath.Join(dirB, baseB+".info.json"), `{"id":"def09876543","uploader":"Tom Scott","upload_date":"20220505"}`)

	// Already-migrated video — should be skipped.
	migrated := filepath.Join(root, "Tom_Scott", "Season 2023")
	if err := os.MkdirAll(migrated, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(migrated, "Tom_Scott - 20230101 - Already Migrated [xyz11111111].info.json"), `{"id":"xyz11111111","uploader":"Tom Scott","upload_date":"20230101"}`)

	result, err := planMigrations(root, true)
	if err != nil {
		t.Fatalf("planMigrations failed: %v", err)
	}
	if got, want := len(result.plans), 2; got != want {
		t.Fatalf("got %d plans, want %d (plans=%+v)", got, want, result.plans)
	}
	if got, want := len(result.duplicates), 0; got != want {
		t.Errorf("got %d duplicate groups, want %d (%+v)", got, want, result.duplicates)
	}

	plans := result.plans
	sort.Slice(plans, func(i, j int) bool { return plans[i].src < plans[j].src })

	// Video A plan
	if got, want := plans[0].uploader, "3Blue1Brown"; got != want {
		t.Errorf("plan[0].uploader = %q; want %q", got, want)
	}
	if got, want := plans[0].dstDir, filepath.Join(root, "3Blue1Brown", "Season 2021"); got != want {
		t.Errorf("plan[0].dstDir = %q; want %q", got, want)
	}
	if got, want := len(plans[0].siblings), 4; got != want {
		t.Errorf("plan[0] siblings = %d; want %d (%v)", got, want, plans[0].siblings)
	}

	// Video B plan
	if got, want := plans[1].uploader, "Tom_Scott"; got != want {
		t.Errorf("plan[1].uploader = %q; want %q", got, want)
	}
	if got, want := plans[1].dstDir, filepath.Join(root, "Tom_Scott", "Season 2022"); got != want {
		t.Errorf("plan[1].dstDir = %q; want %q", got, want)
	}
	if got, want := len(plans[1].siblings), 2; got != want {
		t.Errorf("plan[1] siblings = %d; want %d (%v)", got, want, plans[1].siblings)
	}
}

// TestPlanMigrationsSkipsChannelInfoAndStateDirs makes sure the planner
// doesn't try to migrate the channel-level info.json files we emit
// ourselves, and doesn't descend into our state directories.
func TestPlanMigrationsSkipsChannelInfoAndStateDirs(t *testing.T) {
	root := t.TempDir()

	// Channel-level info.json at a show root — should be ignored.
	showRoot := filepath.Join(root, "3Blue1Brown")
	if err := os.MkdirAll(showRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(showRoot, "3Blue1Brown - NA - Videos [UCYO_jab_esuFRV4b17AJtAw].info.json"), `{"id":"NA","uploader":"3Blue1Brown","upload_date":""}`)

	// State dirs — should be skipped entirely.
	stateA := filepath.Join(root, ".bootstrap")
	stateB := filepath.Join(root, ".archives")
	if err := os.MkdirAll(stateA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateB, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(stateA, "UCYO_jab_esuFRV4b17AJtAw.done"), "")
	writeFile(t, filepath.Join(stateB, "UCYO_jab_esuFRV4b17AJtAw.txt"), "youtube abc12345678")

	result, err := planMigrations(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.plans) != 0 {
		t.Errorf("expected 0 plans, got %d: %+v", len(result.plans), result.plans)
	}
	if len(result.duplicates) != 0 {
		t.Errorf("expected 0 duplicate groups, got %d: %+v", len(result.duplicates), result.duplicates)
	}
}

// TestPlanMigrationsDedupsByTarget covers the case where the same video
// (same uploader, same id, same date) lives under two legacy playlist
// trees. Both legacy paths resolve to the same Jellyfin target — the
// planner should keep one and surface the other as a duplicate.
func TestPlanMigrationsDedupsByTarget(t *testing.T) {
	root := t.TempDir()

	infoBody := `{"id":"abc12345678","uploader":"3Blue1Brown","upload_date":"20210101"}`
	base := "20210101 - Same Video [abc12345678]"

	// Copy A under "Math - (3Blue1Brown)/..."
	dirA := filepath.Join(root, "Math - (3Blue1Brown)", "20210101 - Same Video")
	if err := os.MkdirAll(dirA, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dirA, base+".mkv"), "video-a")
	writeFile(t, filepath.Join(dirA, base+".info.json"), infoBody)

	// Copy B under "Best Of - (3Blue1Brown)/..."
	dirB := filepath.Join(root, "Best Of - (3Blue1Brown)", "20210101 - Same Video")
	if err := os.MkdirAll(dirB, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dirB, base+".mkv"), "video-b")
	writeFile(t, filepath.Join(dirB, base+".info.json"), infoBody)

	result, err := planMigrations(root, true)
	if err != nil {
		t.Fatalf("planMigrations failed: %v", err)
	}
	if got, want := len(result.plans), 1; got != want {
		t.Fatalf("expected 1 plan (after dedup), got %d (%+v)", got, result.plans)
	}
	if got, want := len(result.duplicates), 1; got != want {
		t.Fatalf("expected 1 duplicate group, got %d (%+v)", got, result.duplicates)
	}
	group := result.duplicates[0]
	if got, want := len(group), 2; got != want {
		t.Fatalf("expected duplicate group of size 2, got %d (%v)", got, group)
	}
	// The kept source (group[0], also in plans) should be the lexicographically
	// first path — "Best Of - ..." sorts before "Math - ...".
	if !strings.HasPrefix(group[0], filepath.Join(root, "Best Of")) {
		t.Errorf("expected kept source under 'Best Of', got %q", group[0])
	}
	if result.plans[0].src != group[0] {
		t.Errorf("plan[0].src (%q) should match group[0] (%q)", result.plans[0].src, group[0])
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
