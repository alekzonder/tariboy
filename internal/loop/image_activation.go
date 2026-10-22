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
	ImageVersion         string
	Skills               harness.SkillLaunchConfig
}

func (m *Manager) activatePendingImage(ag *agent.Agent) (activatedImage, error) {
	var activated activatedImage
	// The store lock, not a name lock: activation reads the agent's current
	// and pending images, which can have different names, and the pending
	// assignment can move to another name while it runs.
	err := image.WithStoreLock(func() error {
		var err error
		activated, err = m.activatePendingImageLocked(ag)
		return err
	})
	return activated, err
}

func (m *Manager) activatePendingImageLocked(ag *agent.Agent) (activatedImage, error) {
	l := agentdir.New(m.cfg.AgentsDir, ag.Name)
	if err := os.MkdirAll(l.Root, 0o700); err != nil {
		return activatedImage{}, err
	}
	recoveredSwap, err := recoverImageSwap(l, ag.ImageRef, ag.ImageDigest)
	if err != nil {
		return activatedImage{}, err
	}
	pending, current, err := m.resolvePendingImage(ag)
	if err != nil {
		return activatedImage{}, err
	}
	if pending.Ref == "" {
		return m.activateCurrentImage(ag, l, current, recoveredSwap)
	}
	fail := func(cause error) (activatedImage, error) {
		_ = m.cfg.Store.SetPendingImageErrorIf(ag.Name, pending.Ref, pending.Digest, cause.Error())
		return activatedImage{}, cause
	}
	// A crash can occur after candidate shims are written but before the
	// candidate identity is promoted in the database. The database remains
	// authoritative, so recovery must restore both the active image bytes and
	// the daemon-owned shims derived from the active plugin set before retrying
	// any still-pending activation.
	if recoveredSwap {
		if err := agentdir.WriteShims(l, *ag); err != nil {
			return fail(err)
		}
	}
	staged, err := m.stagePendingImage(*ag, l, pending)
	if staged.dir != "" {
		defer os.RemoveAll(staged.dir)
	}
	if err != nil {
		return fail(err)
	}
	return m.swapPendingImage(ag, l, pending, staged, fail)
}

// resolvePendingImage returns the agent's pending assignment. With no explicit
// pending image it inspects the active ref: a moved mutable ref stages its new
// digest, an inspection failure is recorded as a retryable pending error, and a
// stale error on an empty assignment is cleared. current is the inspected
// active manifest when that inspection succeeded.
func (m *Manager) resolvePendingImage(ag *agent.Agent) (pending agent.ImageAssignment, current image.Manifest, err error) {
	pending, err = m.cfg.Store.PendingImage(ag.Name)
	if err != nil || pending.Ref != "" {
		return pending, current, err
	}
	activeRef, err := image.ParseRef(ag.ImageRef)
	if err != nil {
		return pending, current, fmt.Errorf("invalid active image ref %q: %w", ag.ImageRef, err)
	}
	current, err = m.cfg.ImgStore.Inspect(activeRef)
	if err != nil {
		recorded, recordErr := m.cfg.Store.SetPendingImageErrorIfEmpty(ag.Name, err.Error())
		if recordErr != nil {
			return pending, current, recordErr
		}
		if recorded {
			return pending, current, err
		}
		pending, err = m.cfg.Store.PendingImage(ag.Name)
		if err != nil {
			return pending, current, err
		}
	} else if current.Digest != ag.ImageDigest {
		won, err := m.cfg.Store.SetPendingImageIfEmpty(ag.Name, activeRef.String(), current.Digest)
		if err != nil {
			return pending, current, err
		}
		if won {
			pending = agent.ImageAssignment{Ref: activeRef.String(), Digest: current.Digest}
		} else if pending, err = m.cfg.Store.PendingImage(ag.Name); err != nil {
			return pending, current, err
		}
	}
	if pending.Ref == "" && pending.Error != "" {
		cleared, err := m.cfg.Store.ClearPendingImageErrorIfEmpty(ag.Name)
		if err != nil {
			return pending, current, err
		}
		if !cleared {
			if pending, err = m.cfg.Store.PendingImage(ag.Name); err != nil {
				return pending, current, err
			}
		}
	}
	return pending, current, nil
}

// activateCurrentImage launches on the already active image when nothing is
// pending, re-deriving shims if a crashed swap was just recovered.
func (m *Manager) activateCurrentImage(ag *agent.Agent, l agentdir.Layout, current image.Manifest, recoveredSwap bool) (activatedImage, error) {
	activeRef, err := image.ParseRef(ag.ImageRef)
	if err != nil {
		return activatedImage{}, fmt.Errorf("invalid active image ref %q: %w", ag.ImageRef, err)
	}
	manifest := current
	if manifest.Digest == "" {
		manifest, err = m.cfg.ImgStore.InspectPinned(activeRef, ag.ImageDigest)
		if err != nil {
			return activatedImage{}, err
		}
	}
	skills, err := m.prepareImageSkillBridge(*ag, manifest, l.ImageDir())
	if err != nil {
		return activatedImage{}, err
	}
	if recoveredSwap {
		if err := agentdir.WriteShims(l, *ag); err != nil {
			_, _ = m.cfg.Store.SetPendingImageErrorIfEmpty(ag.Name, err.Error())
			return activatedImage{}, err
		}
	}
	return activatedImage{PromptTemplateSHA256: manifest.PromptTemplateSHA256, ImageVersion: manifest.ImageVersion, Skills: skills}, nil
}

// stagedImage is a pending image unpacked and validated beside image/.
type stagedImage struct {
	dir      string
	manifest image.Manifest
	skills   harness.SkillLaunchConfig
	plugins  []string
}

// stagePendingImage unpacks the pending image into a fresh staging dir under
// l.Root and validates it. dir is set as soon as the staging dir exists, even
// on error, so the caller owns its removal.
func (m *Manager) stagePendingImage(ag agent.Agent, l agentdir.Layout, pending agent.ImageAssignment) (stagedImage, error) {
	var st stagedImage
	ref, err := image.ParseRef(pending.Ref)
	if err != nil {
		return st, err
	}
	st.manifest, err = m.cfg.ImgStore.InspectPinned(ref, pending.Digest)
	if err != nil {
		return st, err
	}
	st.dir, err = os.MkdirTemp(l.Root, ".image-stage-")
	if err != nil {
		return st, err
	}
	if err := m.cfg.ImgStore.UnpackPinned(ref, pending.Digest, st.dir); err != nil {
		return st, err
	}
	staged, err := readStagedManifest(st.dir, ref, pending.Digest)
	if err != nil {
		return st, err
	}
	requested := make([]string, 0, len(staged.Plugins))
	for _, plugin := range staged.Plugins {
		requested = append(requested, plugin.Name)
	}
	baseDir := filepath.Dir(m.cfg.ImgStore.Dir)
	resolver := plugins.ResolveInstalledMetadata(paths.Paths{Base: baseDir}.PluginsDir())
	if m.cfg.ExternalPlugins != nil {
		resolver = m.cfg.ExternalPlugins
	}
	if err := image.ValidateUnpackedContract(staged, st.dir, resolver); err != nil {
		return st, err
	}
	st.skills, err = m.prepareImageSkillBridge(ag, staged, st.dir)
	if err != nil {
		return st, err
	}
	if !image.IsLegacyV1(st.manifest) {
		st.plugins = requested
	} else if st.plugins, err = plugincaps.ResolveWithExternal(requested, resolver); err != nil {
		return st, err
	}
	return st, nil
}

// readStagedManifest loads the unpacked manifest and checks it belongs to ref.
func readStagedManifest(stage string, ref image.Ref, digest string) (image.Manifest, error) {
	var staged image.Manifest
	data, err := os.ReadFile(filepath.Join(stage, "manifest.json"))
	if err != nil {
		return staged, err
	}
	if err := json.Unmarshal(data, &staged); err != nil {
		return staged, err
	}
	if staged.Name != ref.Name {
		return staged, fmt.Errorf("staged image ref mismatch")
	}
	staged.Digest = digest
	if !image.IsLegacyV1(staged) {
		if _, err := os.Stat(filepath.Join(stage, "prompt", "template.json")); err != nil {
			return staged, fmt.Errorf("staged prompt template: %w", err)
		}
	}
	return staged, nil
}

// swapPendingImage moves the staged image into place, rewrites shims for the
// candidate plugin set and promotes the pending identity. Any failure restores
// the previous image and its shims before reporting through fail.
func (m *Manager) swapPendingImage(ag *agent.Agent, l agentdir.Layout, pending agent.ImageAssignment, st stagedImage, fail func(error) (activatedImage, error)) (activatedImage, error) {
	commit, rollback, err := agentdir.SwapImage(l, st.dir)
	if err != nil {
		return fail(err)
	}
	nextAgent := *ag
	nextAgent.Plugins = st.plugins
	if err := agentdir.WriteShims(l, nextAgent); err != nil {
		rollback()
		_ = agentdir.WriteShims(l, *ag)
		return fail(err)
	}
	if err := m.cfg.Store.PromotePendingImageWithPlugins(ag.Name, pending.Ref, pending.Digest, st.plugins); err != nil {
		rollback()
		_ = agentdir.WriteShims(l, *ag)
		return fail(err)
	}
	commit()
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
	return activatedImage{PromptTemplateSHA256: st.manifest.PromptTemplateSHA256, ImageVersion: st.manifest.ImageVersion, Skills: st.skills}, nil
}

// reprovisionCandidate validates imageRef (empty keeps the current image) and
// returns ag re-pointed at it, enabled, with the image's declared plugins.
func (m *Manager) reprovisionCandidate(ag agent.Agent, imageRef string) (agent.Agent, image.Ref, image.Manifest, error) {
	if imageRef == "" {
		imageRef = ag.ImageRef
	}
	ref, err := image.ParseRef(imageRef)
	if err != nil {
		return ag, ref, image.Manifest{}, err
	}
	archive, err := m.cfg.ImgStore.ArchiveBytes(ref)
	if err != nil {
		return ag, ref, image.Manifest{}, fmt.Errorf("image %s: %w", ref.String(), err)
	}
	man, err := image.ValidatePortableArchiveContract(archive, ref, m.imagePluginResolver())
	if err != nil {
		return ag, ref, man, fmt.Errorf("image %s: %w", ref.String(), err)
	}
	candidate := ag
	candidate.Enabled = true
	candidate.ImageRef = ref.String()
	candidate.ImageDigest = man.Digest
	if !image.IsLegacyV1(man) {
		candidate.Plugins = make([]string, 0, len(man.Plugins))
		for _, plugin := range man.Plugins {
			candidate.Plugins = append(candidate.Plugins, plugin.Name)
		}
	}
	return candidate, ref, man, nil
}

// installReprovisionedImage provisions candidate into a staging tree, prepares
// its skill bridge and then swaps it in for ag's image/ and bin/.
func (m *Manager) installReprovisionedImage(ag, candidate agent.Agent, ref image.Ref, man image.Manifest) error {
	l := agentdir.New(m.cfg.AgentsDir, ag.Name)
	if err := os.MkdirAll(l.Root, 0o700); err != nil {
		return err
	}
	stageRoot, err := os.MkdirTemp(l.Root, ".reprovision-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stageRoot)
	stage := agentdir.Layout{Root: stageRoot, Name: ag.Name}
	if err := agentdir.Provision(stage, candidate, m.cfg.ImgStore, ref); err != nil {
		return err
	}
	if _, err := m.prepareImageSkillBridge(candidate, man, stage.ImageDir()); err != nil {
		return err
	}
	return m.swapReprovisionedTree(ag, candidate, l, stage.ImageDir())
}

// swapReprovisionedTree moves stageImage into image/ and bin/ aside, rebuilds
// shims and durable dirs, and records candidate's identity. Any failure
// restores the previous image/, bin/ and identity.
func (m *Manager) swapReprovisionedTree(ag, candidate agent.Agent, l agentdir.Layout, stageImage string) error {
	// image/ goes first: its .image-backup is the crash marker that makes
	// recovery restore the active image and rewrite the shims.
	commitImage, rollbackImage, err := agentdir.SwapImage(l, stageImage)
	if err != nil {
		return err
	}
	commitBin, rollbackBin, err := agentdir.SwapDir(l.BinDir(), filepath.Join(l.Root, ".bin-backup"), "")
	if err != nil {
		rollbackImage()
		return err
	}
	rollback := func() {
		rollbackImage()
		rollbackBin()
	}
	if err := os.MkdirAll(l.BinDir(), 0o700); err != nil {
		rollback()
		return err
	}
	if err := agentdir.WriteShims(l, candidate); err != nil {
		rollback()
		return err
	}
	for _, dir := range []string{l.Workdir(), l.IterationsDir()} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			rollback()
			return err
		}
	}
	if err := m.cfg.Store.SetImageIdentity(ag.Name, candidate.ImageRef, candidate.ImageDigest); err != nil {
		rollback()
		return err
	}
	if err := m.cfg.Store.Update(candidate); err != nil {
		_ = m.cfg.Store.SetImageIdentity(ag.Name, ag.ImageRef, ag.ImageDigest)
		rollback()
		return err
	}
	commitImage()
	commitBin()
	return nil
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
		env := BuildEnv(baseEnv(m.cfg.BaseEnv), l.BinDir(), ag.Name, "", l.Sock(), false, "", "", ag.Env, secrets)
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
	backup := filepath.Join(l.Root, agentdir.ImageBackupName)
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
		refText, err := os.ReadFile(filepath.Join(dir, ".image-ref"))
		if err != nil {
			// An image unpacked before tags became pointers still names its tag.
			return manifest.Name + ":" + manifest.Tag, strings.TrimSpace(string(digest))
		}
		return strings.TrimSpace(string(refText)), strings.TrimSpace(string(digest))
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
