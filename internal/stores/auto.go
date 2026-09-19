package stores

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// AutoBuild is a Store's periodic refresh-and-build policy. A zero interval
// disables it; Images selects which Store images an automatic cycle rebuilds.
type AutoBuild struct {
	IntervalMinutes int      `json:"interval_minutes"`
	Images          []string `json:"images"`
}

// Builder builds one Store image selector (store/image) with the default tags,
// which publish both image_version and latest.
type Builder func(ctx context.Context, selector string) error

// AutoInterval is the cycle granularity of RunAuto; policies are configured in
// whole minutes.
const AutoInterval = time.Minute

func (c *Catalog) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// SetAuto replaces a Store's automatic build policy. Enabling or changing it
// restarts the interval from now, so a policy change never fires immediately.
func (c *Catalog) SetAuto(ctx context.Context, name string, intervalMinutes int, images []string) (Detail, error) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	if intervalMinutes < 0 {
		return Detail{}, fmt.Errorf("%w interval %d (want whole minutes, 0 disables)", ErrInvalid, intervalMinutes)
	}
	detail, err := c.detail(ctx, name)
	if err != nil {
		return Detail{}, err
	}
	known := make(map[string]bool, len(detail.Images))
	for _, image := range detail.Images {
		known[image.Name] = true
	}
	selected := make([]string, 0, len(images))
	seen := make(map[string]bool, len(images))
	for _, image := range images {
		if !known[image] {
			return Detail{}, fmt.Errorf("%w image %q for Store %s", ErrInvalid, image, name)
		}
		if seen[image] {
			continue
		}
		seen[image] = true
		selected = append(selected, image)
	}
	encoded, err := json.Marshal(selected)
	if err != nil {
		return Detail{}, err
	}
	if _, err := c.DB.ExecContext(ctx,
		`UPDATE image_stores SET auto_interval_minutes=?, auto_images=?, auto_last_run_at=? WHERE name=?`,
		intervalMinutes, string(encoded), c.now().Unix(), name); err != nil {
		return Detail{}, err
	}
	detail.Auto = AutoBuild{IntervalMinutes: intervalMinutes, Images: selected}
	return detail, nil
}

// RunAutoCycle refreshes every Store whose interval has elapsed and rebuilds
// its selected images that report update_needed. A failing Store or image never
// stops the remaining work; the next attempt is one interval later.
func (c *Catalog) RunAutoCycle(ctx context.Context, now time.Time, build Builder, log *slog.Logger) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	due, err := c.autoDue(ctx, now)
	if err != nil {
		log.Warn("Store automatic build scan", "err", err)
		return
	}
	for _, policy := range due {
		if err := c.markAutoRun(ctx, policy.name, now); err != nil {
			log.Warn("Store automatic build schedule", "store", policy.name, "err", err)
			continue
		}
		detail, err := c.Refresh(ctx, policy.name)
		if err != nil {
			log.Warn("Store automatic refresh", "store", policy.name, "err", err)
			continue
		}
		selected := make(map[string]bool, len(policy.images))
		for _, image := range policy.images {
			selected[image] = true
		}
		for _, image := range detail.Images {
			if !selected[image.Name] || !image.UpdateNeeded {
				continue
			}
			selector := policy.name + "/" + image.Name
			if err := build(ctx, selector); err != nil {
				log.Warn("Store automatic build", "selector", selector, "err", err)
				continue
			}
			log.Info("Store automatic build", "selector", selector, "image_version", image.Version)
		}
	}
}

// RunAuto drives RunAutoCycle until the context is cancelled.
func (c *Catalog) RunAuto(ctx context.Context, build Builder, log *slog.Logger) {
	ticker := time.NewTicker(AutoInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			c.RunAutoCycle(ctx, now, build, log)
		}
	}
}

type autoPolicy struct {
	name   string
	images []string
}

func (c *Catalog) autoDue(ctx context.Context, now time.Time) ([]autoPolicy, error) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	if c.DB == nil {
		return nil, errDatabaseUnavailable
	}
	rows, err := c.DB.QueryContext(ctx,
		`SELECT name,auto_images FROM image_stores
		 WHERE auto_interval_minutes > 0 AND auto_last_run_at + auto_interval_minutes * 60 <= ?
		 ORDER BY name`, now.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var due []autoPolicy
	for rows.Next() {
		var policy autoPolicy
		var encoded string
		if err := rows.Scan(&policy.name, &encoded); err != nil {
			return nil, err
		}
		policy.images = decodeAutoImages(encoded)
		due = append(due, policy)
	}
	return due, rows.Err()
}

func (c *Catalog) markAutoRun(ctx context.Context, name string, now time.Time) error {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	if c.DB == nil {
		return errDatabaseUnavailable
	}
	_, err := c.DB.ExecContext(ctx, `UPDATE image_stores SET auto_last_run_at=? WHERE name=?`, now.Unix(), name)
	return err
}

func (c *Catalog) readAuto(name string) (AutoBuild, error) {
	if c.DB == nil {
		return AutoBuild{}, errDatabaseUnavailable
	}
	var auto AutoBuild
	var encoded string
	if err := c.DB.QueryRow(`SELECT auto_interval_minutes,auto_images FROM image_stores WHERE name=?`, name).Scan(&auto.IntervalMinutes, &encoded); err != nil {
		return AutoBuild{}, err
	}
	auto.Images = decodeAutoImages(encoded)
	return auto, nil
}

func decodeAutoImages(encoded string) []string {
	images := []string{}
	if encoded == "" {
		return images
	}
	if err := json.Unmarshal([]byte(encoded), &images); err != nil {
		return []string{}
	}
	return images
}
