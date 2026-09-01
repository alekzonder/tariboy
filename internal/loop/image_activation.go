package loop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/harness"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/plugincaps"
	"github.com/alekzonder/tariboy/internal/plugins"
)

// activatePendingImage runs only inside the engine's per-agent launch gate.
// It swaps rebuildable image bytes and promotes DB identity before prompt or
// message preparation begins.
type activatedImage struct {
	PromptTemplateSHA256 string
	Skills               harness.SkillLaunchConfig
}

func (m *Manager) activatePendingImage(ag *agent.Agent) (activatedImage, error) {
	l := agentdir.New(m.cfg.AgentsDir, ag.Name)
	if err := os.MkdirAll(l.Root, 0o700); err != nil {
		return activatedImage{}, err
	}
	recoveredSwap, err := recoverImageSwap(l, ag.ImageRef, ag.ImageDigest)
	if err != nil {
		return activatedImage{}, err
	}
	pending, err := m.cfg.Store.PendingImage(ag.Name)
	if err != nil {
		return activatedImage{}, err
	}
	if pending.Ref == "" {
		activeRef, err := image.ParseRef(ag.ImageRef)
		if err != nil {
			return activatedImage{}, fmt.Errorf("invalid active image ref %q: %w", ag.ImageRef, err)
		}
		if m.cfg.ImgStore.IsMutable(activeRef) {
			current, err := m.cfg.ImgStore.Inspect(activeRef)
			if err != nil {
				recorded, recordErr := m.cfg.Store.SetPendingImageErrorIfEmpty(ag.Name, err.Error())
				if recordErr != nil {
					return activatedImage{}, recordErr
				}
				if recorded {
					return activatedImage{}, err
				}
				pending, err = m.cfg.Store.PendingImage(ag.Name)
				if err != nil {
					return activatedImage{}, err
				}
			} else if current.Digest != ag.ImageDigest {
				won, err := m.cfg.Store.SetPendingImageIfEmpty(ag.Name, activeRef.String(), current.Digest)
				if err != nil {
					return activatedImage{}, err
				}
				if won {
					pending = agent.ImageAssignment{Ref: activeRef.String(), Digest: current.Digest}
				} else {
					pending, err = m.cfg.Store.PendingImage(ag.Name)
					if err != nil {
						return activatedImage{}, err
					}
				}
			} else if pending.Error != "" {
				cleared, err := m.cfg.Store.ClearPendingImageErrorIfEmpty(ag.Name)
				if err != nil {
					return activatedImage{}, err
				}
				if !cleared {
					pending, err = m.cfg.Store.PendingImage(ag.Name)
					if err != nil {
						return activatedImage{}, err
					}
				}
			}
		}
	}
	fail := func(cause error) (activatedImage, error) {
		_ = m.cfg.Store.SetPendingImageErrorIf(ag.Name, pending.Ref, pending.Digest, cause.Error())
		return activatedImage{}, cause
	}
	if pending.Ref == "" {
		activeRef, err := image.ParseRef(ag.ImageRef)
		if err != nil {
			return activatedImage{}, fmt.Errorf("invalid active image ref %q: %w", ag.ImageRef, err)
		}
		manifest, err := m.cfg.ImgStore.InspectPinned(activeRef, ag.ImageDigest)
		if err != nil {
			return activatedImage{}, err
		}
		skills, err := m.prepareImageSkillBridge(*ag, manifest, l.ImageDir())
		if err != nil {
			return activatedImage{}, err
		}
		if recoveredSwap {
			if err := agentdir.WriteShims(l, *ag, m.cfg.ToolsBin); err != nil {
				_, _ = m.cfg.Store.SetPendingImageErrorIfEmpty(ag.Name, err.Error())
				return activatedImage{}, err
			}
		}
		return activatedImage{PromptTemplateSHA256: manifest.PromptTemplateSHA256, Skills: skills}, nil
	}
	// A crash can occur after candidate shims are written but before the
	// candidate identity is promoted in the database. The database remains
	// authoritative, so recovery must restore both the active image bytes and
	// the daemon-owned shims derived from the active plugin set before retrying
	// any still-pending activation.
	if recoveredSwap {
		if err := agentdir.WriteShims(l, *ag, m.cfg.ToolsBin); err != nil {
			return fail(err)
		}
	}
	ref, err := image.ParseRef(pending.Ref)
	if err != nil {
		return fail(err)
	}
	manifest, err := m.cfg.ImgStore.InspectPinned(ref, pending.Digest)
	if err != nil {
		return fail(err)
	}
	stage, err := os.MkdirTemp(l.Root, ".image-stage-")
	if err != nil {
		return fail(err)
	}
	defer os.RemoveAll(stage)
	if err := m.cfg.ImgStore.UnpackPinned(ref, pending.Digest, stage); err != nil {
		return fail(err)
	}
	data, err := os.ReadFile(filepath.Join(stage, "manifest.json"))
	if err != nil {
		return fail(err)
	}
	var staged image.Manifest
	if err := json.Unmarshal(data, &staged); err != nil {
		return fail(err)
	}
	if staged.Name != ref.Name || staged.Tag != ref.Tag {
		return fail(fmt.Errorf("staged image ref mismatch"))
	}
	staged.Digest = pending.Digest
	if staged.SchemaVersion == 2 {
		if _, err := os.Stat(filepath.Join(stage, "prompt", "template.json")); err != nil {
			return fail(fmt.Errorf("staged prompt template: %w", err))
		}
	}
	skillLaunch, err := m.prepareImageSkillBridge(*ag, staged, stage)
	if err != nil {
		return fail(err)
	}
	requested := make([]string, 0, len(manifest.Plugins))
	for _, plugin := range manifest.Plugins {
		requested = append(requested, plugin.Name)
	}
	baseDir := filepath.Dir(m.cfg.ImgStore.Dir)
	resolver := plugins.ResolveInstalledMetadata(paths.Paths{Base: baseDir}.PluginsDir())
	if m.cfg.ExternalPlugins != nil {
		resolver = m.cfg.ExternalPlugins
	}
	var nextPlugins []string
	if manifest.SchemaVersion == 2 {
		nextPlugins, err = plugincaps.ValidateExplicit(requested, resolver)
	} else {
		nextPlugins, err = plugincaps.ResolveWithExternal(requested, resolver)
	}
	if err != nil {
		return fail(err)
	}
	backup := filepath.Join(l.Root, ".image-backup")
	_ = os.RemoveAll(backup)
	if err := os.Rename(l.ImageDir(), backup); err != nil && !os.IsNotExist(err) {
		return fail(err)
	}
	if err := os.Rename(stage, l.ImageDir()); err != nil {
		_ = os.Rename(backup, l.ImageDir())
		return fail(err)
	}
	nextAgent := *ag
	nextAgent.Plugins = nextPlugins
	if err := agentdir.WriteShims(l, nextAgent, m.cfg.ToolsBin); err != nil {
		_ = os.RemoveAll(l.ImageDir())
		_ = os.Rename(backup, l.ImageDir())
		_ = agentdir.WriteShims(l, *ag, m.cfg.ToolsBin)
		return fail(err)
	}
	if err := m.cfg.Store.PromotePendingImageWithPlugins(ag.Name, pending.Ref, pending.Digest, nextPlugins); err != nil {
		_ = os.RemoveAll(l.ImageDir())
		_ = os.Rename(backup, l.ImageDir())
		_ = agentdir.WriteShims(l, *ag, m.cfg.ToolsBin)
		return fail(err)
	}
	_ = os.RemoveAll(backup)
	if m.cfg.AuditFor != nil {
		if rec := m.cfg.AuditFor(ag.Name); rec != nil {
			rec.Record("agent_image_activated", "system", "", map[string]any{
				"old_ref": ag.ImageRef, "old_digest": ag.ImageDigest,
				"new_ref": pending.Ref, "new_digest": pending.Digest,
			})
		}
	}
	updated, err := m.cfg.Store.Get(ag.Name)
	if err != nil {
		return activatedImage{}, err
	}
	*ag = updated
	return activatedImage{PromptTemplateSHA256: manifest.PromptTemplateSHA256, Skills: skillLaunch}, nil
}

func (m *Manager) prepareImageSkillBridge(ag agent.Agent, manifest image.Manifest, imageDir string) (harness.SkillLaunchConfig, error) {
	if manifest.SchemaVersion != 2 || len(manifest.Skills) == 0 {
		return harness.SkillLaunchConfig{}, nil
	}
	adapter, err := harness.Get(ag.HarnessType)
	if err != nil {
		return harness.SkillLaunchConfig{}, err
	}
	if adapter.Type() == "stub" {
		return harness.SkillLaunchConfig{}, nil
	}
	l := agentdir.New(m.cfg.AgentsDir, ag.Name).WithRuntime(m.cfg.RuntimeDir)
	bridgeDir, err := l.ImageBridgeDir(manifest.Digest, harness.SkillAdapterContractVersion, adapter.Type())
	if err != nil {
		return harness.SkillLaunchConfig{}, err
	}
	descriptors := make([]harness.SkillDescriptor, 0, len(manifest.Skills))
	for _, skill := range manifest.Skills {
		descriptors = append(descriptors, harness.SkillDescriptor{Name: skill.Name, Description: skill.Description, TreeSHA256: skill.TreeSHA256})
	}
	bridge, err := adapter.SkillBridge(harness.SkillBridgeRequest{
		ImageName: manifest.Name, ImageDigest: manifest.Digest, BridgeDir: bridgeDir, Skills: descriptors,
	})
	if err != nil {
		return harness.SkillLaunchConfig{}, err
	}
	prepare := m.cfg.PrepareImageBridge
	if prepare == nil {
		prepare = agentdir.PrepareImageBridge
	}
	if err := prepare(filepath.Join(imageDir, "skills"), bridgeDir, manifest.Skills, bridge.Plan); err != nil {
		return harness.SkillLaunchConfig{}, fmt.Errorf("prepare %s image skill bridge: %w", adapter.Type(), err)
	}
	if len(bridge.Support.Args) > 0 {
		secrets, err := m.cfg.Store.SecretMap(ag.Name)
		if err != nil {
			return harness.SkillLaunchConfig{}, err
		}
		env := BuildEnv(os.Environ(), l.BinDir(), ag.Name, "", l.Sock(), false, "", "", ag.Env, secrets)
		executable, err := harness.FindExecutable(adapter.Executable(), env, agentCwd(ag, l))
		if err != nil {
			return harness.SkillLaunchConfig{}, fmt.Errorf("harness executable %q not found for image skills", adapter.Type())
		}
		probeEnv, err := mergeSkillLaunchEnv(env, bridge.Support.Env)
		if err != nil {
			return harness.SkillLaunchConfig{}, err
		}
		bridge.Support.Env = probeEnv
		if err := harness.ValidateSkillBridgeSupport(executable, adapter.Type(), bridge.Support); err != nil {
			return harness.SkillLaunchConfig{}, err
		}
	}
	return bridge.Launch, nil
}

// recoverImageSwap uses the backup directory as an explicit crash marker. The
// database remains authoritative: if image/ already matches its active ref the
// stale backup is removed; otherwise backup is restored and the uncommitted
// candidate is discarded. Abandoned staging directories are always rebuildable.
func recoverImageSwap(l agentdir.Layout, activeRef, activeDigest string) (bool, error) {
	entries, err := os.ReadDir(l.Root)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), ".image-stage-") {
			if err := os.RemoveAll(filepath.Join(l.Root, entry.Name())); err != nil {
				return false, err
			}
		}
	}
	backup := filepath.Join(l.Root, ".image-backup")
	if _, err := os.Stat(backup); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	imageIdentity := func(dir string) (string, string) {
		data, readErr := os.ReadFile(filepath.Join(dir, "manifest.json"))
		if readErr != nil {
			return "", ""
		}
		var manifest image.Manifest
		if json.Unmarshal(data, &manifest) != nil {
			return "", ""
		}
		digest, _ := os.ReadFile(filepath.Join(dir, ".image-digest"))
		return manifest.Name + ":" + manifest.Tag, strings.TrimSpace(string(digest))
	}
	currentRef, currentDigest := imageIdentity(l.ImageDir())
	backupRef, backupDigest := imageIdentity(backup)
	if currentRef == activeRef && currentDigest == activeDigest {
		return true, os.RemoveAll(backup)
	}
	if backupRef != activeRef || backupDigest != activeDigest {
		return false, fmt.Errorf("cannot recover image swap for active ref %s", activeRef)
	}
	if err := os.RemoveAll(l.ImageDir()); err != nil {
		return false, err
	}
	return true, os.Rename(backup, l.ImageDir())
}
