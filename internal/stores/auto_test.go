package stores

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/store"
)

func outdatedStore(t *testing.T) (*Catalog, string) {
	t.Helper()
	base, source := t.TempDir(), t.TempDir()
	writeImage(t, source, "alpha", "2.0.0")
	writeImage(t, source, "beta", "3.0.0")
	built := &image.Store{Dir: filepath.Join(base, "images")}
	buildStoreImage(t, built, source, "alpha", "alpha", "latest", "1.0.0", "2026-01-01T00:00:00Z")
	buildStoreImage(t, built, source, "beta", "beta", "latest", "1.0.0", "2026-01-01T00:00:00Z")
	catalog, db := openCatalog(t, base)
	t.Cleanup(func() { db.Close() })
	if _, err := catalog.Add(context.Background(), "team", source, ""); err != nil {
		t.Fatal(err)
	}
	return catalog, source
}

func TestSetAutoPersistsIntervalAndSelectedImages(t *testing.T) {
	catalog, _ := outdatedStore(t)
	detail, err := catalog.SetAuto(context.Background(), "team", 30, []string{"alpha"})
	if err != nil {
		t.Fatal(err)
	}
	want := AutoBuild{IntervalMinutes: 30, Images: []string{"alpha"}, Results: []AutoResult{}}
	if !reflect.DeepEqual(detail.Auto, want) {
		t.Fatalf("SetAuto() auto = %#v, want %#v", detail.Auto, want)
	}
	reread, err := catalog.Detail(context.Background(), "team")
	if err != nil || !reflect.DeepEqual(reread.Auto, want) {
		t.Fatalf("Detail() auto = %#v, %v", reread.Auto, err)
	}
}

func TestSetAutoRejectsBadIntervalAndUnknownImage(t *testing.T) {
	catalog, _ := outdatedStore(t)
	if _, err := catalog.SetAuto(context.Background(), "team", -1, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("negative interval error = %v", err)
	}
	if _, err := catalog.SetAuto(context.Background(), "team", 10, []string{"ghost"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown image error = %v", err)
	}
	if _, err := catalog.SetAuto(context.Background(), "missing", 10, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown Store error = %v", err)
	}
}

func TestRunAutoCycleBuildsOnlySelectedOutdatedImages(t *testing.T) {
	catalog, _ := outdatedStore(t)
	now := time.Now()
	catalog.Now = func() time.Time { return now }
	if _, err := catalog.SetAuto(context.Background(), "team", 30, []string{"alpha"}); err != nil {
		t.Fatal(err)
	}
	var built []string
	build := func(_ context.Context, selector string) error {
		built = append(built, selector)
		return nil
	}
	catalog.RunAutoCycle(context.Background(), now.Add(29*time.Minute), build, nil)
	if len(built) != 0 {
		t.Fatalf("built before the interval elapsed: %v", built)
	}
	catalog.RunAutoCycle(context.Background(), now.Add(30*time.Minute), build, nil)
	if !reflect.DeepEqual(built, []string{"team/alpha"}) {
		t.Fatalf("built = %v, want [team/alpha]", built)
	}
	catalog.RunAutoCycle(context.Background(), now.Add(31*time.Minute), build, nil)
	if len(built) != 1 {
		t.Fatalf("second cycle ran before the next interval: %v", built)
	}
}

func TestRunAutoCycleIsolatesBuildFailures(t *testing.T) {
	catalog, _ := outdatedStore(t)
	now := time.Now()
	catalog.Now = func() time.Time { return now }
	if _, err := catalog.SetAuto(context.Background(), "team", 1, []string{"alpha", "beta"}); err != nil {
		t.Fatal(err)
	}
	var built []string
	build := func(_ context.Context, selector string) error {
		built = append(built, selector)
		if selector == "team/alpha" {
			return errors.New("build exploded")
		}
		return nil
	}
	catalog.RunAutoCycle(context.Background(), now.Add(time.Minute), build, nil)
	if !reflect.DeepEqual(built, []string{"team/alpha", "team/beta"}) {
		t.Fatalf("built = %v, want both selections attempted", built)
	}
}

func TestRunAutoCycleSkipsDisabledStores(t *testing.T) {
	catalog, _ := outdatedStore(t)
	now := time.Now()
	catalog.Now = func() time.Time { return now }
	if _, err := catalog.SetAuto(context.Background(), "team", 0, []string{"alpha"}); err != nil {
		t.Fatal(err)
	}
	var built []string
	catalog.RunAutoCycle(context.Background(), now.Add(24*time.Hour), func(_ context.Context, selector string) error {
		built = append(built, selector)
		return nil
	}, nil)
	if len(built) != 0 {
		t.Fatalf("disabled Store built %v", built)
	}
}

func TestRunAutoCycleRecordsLastResultPerImage(t *testing.T) {
	catalog, _ := outdatedStore(t)
	now := time.Now()
	catalog.Now = func() time.Time { return now }
	if _, err := catalog.SetAuto(context.Background(), "team", 1, []string{"alpha", "beta"}); err != nil {
		t.Fatal(err)
	}
	failAlpha := true
	build := func(_ context.Context, selector string) error {
		if failAlpha && selector == "team/alpha" {
			return errors.New("Store team image alpha: skills-lock.json mismatch")
		}
		return nil
	}
	results := func() []AutoResult {
		t.Helper()
		detail, err := catalog.Detail(context.Background(), "team")
		if err != nil {
			t.Fatal(err)
		}
		return detail.Auto.Results
	}

	first := now.Add(time.Minute)
	catalog.RunAutoCycle(context.Background(), first, build, nil)
	want := []AutoResult{
		{Image: "alpha", Selector: "team/alpha", At: store.FormatTime(first), ImageVersion: "2.0.0", Error: "Store team image alpha: skills-lock.json mismatch"},
		{Image: "beta", Selector: "team/beta", At: store.FormatTime(first), ImageVersion: "3.0.0"},
	}
	if got := results(); !reflect.DeepEqual(got, want) {
		t.Fatalf("results after a failure = %#v, want %#v", got, want)
	}

	failAlpha = false
	second := now.Add(2 * time.Minute)
	catalog.RunAutoCycle(context.Background(), second, build, nil)
	want = []AutoResult{
		{Image: "alpha", Selector: "team/alpha", At: store.FormatTime(second), ImageVersion: "2.0.0"},
		{Image: "beta", Selector: "team/beta", At: store.FormatTime(second), ImageVersion: "3.0.0"},
	}
	if got := results(); !reflect.DeepEqual(got, want) {
		t.Fatalf("results after a success = %#v, want %#v", got, want)
	}

	if _, err := catalog.SetAuto(context.Background(), "team", 1, []string{"beta"}); err != nil {
		t.Fatal(err)
	}
	if got := results(); !reflect.DeepEqual(got, want[1:]) {
		t.Fatalf("results after deselecting alpha = %#v, want %#v", got, want[1:])
	}
}
