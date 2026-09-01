package image

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/plugincaps"
)

func sha256hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// buildOpts holds optional build inputs. Zero value = builtin behaviour only.
type buildOpts struct {
	externalPlugins  plugincaps.ExternalResolver
	builtinStoreRoot string
	mutableRef       bool
}

// WithBuiltinStoreRoot makes legacy schema-v1 prompt fragments load from the
// installed Store for the current product version.
func WithBuiltinStoreRoot(root string) BuildOption {
	return func(o *buildOpts) { o.builtinStoreRoot = root }
}

// BuildOption configures an optional Build input.
type BuildOption func(*buildOpts)

// WithExternalPlugins wires installed-manifest and optional prompt resolution.
func WithExternalPlugins(f plugincaps.ExternalResolver) BuildOption {
	return func(o *buildOpts) { o.externalPlugins = f }
}

// WithMutableRef publishes an ordinary authoring ref that may later advance.
func WithMutableRef() BuildOption {
	return func(o *buildOpts) { o.mutableRef = true }
}

func Build(imgFile *imagefile.Imagefile, ref Ref, store *Store, clock func() time.Time, options ...BuildOption) (Manifest, error) {
	var opts buildOpts
	for _, o := range options {
		o(&opts)
	}
	// versions declared by THIS imagefile (override parent versions)
	versions := map[string]string{}
	var requested []string
	for _, pl := range imgFile.Plugins {
		requested = append(requested, pl.Name)
		if pl.Version != "" {
			versions[pl.Name] = pl.Version
		}
	}

	// parent (from:) fields, when set
	var (
		parentPlugins []ManifestPlugin
		parentEnv     map[string]string
		parentPolicy  ManifestPolicy
		parentHarness ManifestHarness
		parentSecrets []string
		parents       []string
		parentBody    string
	)
	if imgFile.From != "" {
		parentRef, err := ParseRef(imgFile.From)
		if err != nil {
			return Manifest{}, fmt.Errorf("invalid from %q: %w", imgFile.From, err)
		}
		if !store.Exists(parentRef) {
			return Manifest{}, fmt.Errorf("base image %s not built", parentRef.String())
		}
		pm, err := store.Inspect(parentRef)
		if err != nil {
			return Manifest{}, fmt.Errorf("read base image %s: %w", parentRef.String(), err)
		}
		pb, err := store.ReadBody(parentRef)
		if err != nil {
			return Manifest{}, fmt.Errorf("read base image body %s: %w", parentRef.String(), err)
		}
		parentPlugins, parentEnv, parentPolicy = pm.Plugins, pm.Env, pm.Policy
		parentHarness, parentSecrets, parentBody = pm.Harness, pm.RequiresSecrets, pb
		parents = append(append([]string{}, pm.Parents...), parentRef.String())
	}

	// plugin name union: parent order first, then this file's requested order
	var pluginOrder []string
	seen := map[string]bool{}
	for _, pl := range parentPlugins {
		if !seen[pl.Name] {
			pluginOrder = append(pluginOrder, pl.Name)
			seen[pl.Name] = true
		}
		if pl.Version != "" {
			if _, ok := versions[pl.Name]; !ok {
				versions[pl.Name] = pl.Version
			}
		}
	}
	for _, n := range requested {
		if !seen[n] {
			pluginOrder = append(pluginOrder, n)
			seen[n] = true
		}
	}
	external := make(map[string]plugincaps.ResolvedPlugin, len(pluginOrder))
	if opts.externalPlugins != nil {
		for _, name := range pluginOrder {
			plugin, err := opts.externalPlugins(name)
			if err != nil {
				return Manifest{}, err
			}
			external[name] = plugin
		}
	}
	resolved, err := plugincaps.ResolveWithExternal(pluginOrder, func(name string) (plugincaps.ResolvedPlugin, error) {
		return external[name], nil
	})
	if err != nil {
		return Manifest{}, err
	}

	// env: parent then child override
	env := map[string]string{}
	for k, v := range parentEnv {
		env[k] = v
	}
	for k, v := range imgFile.Env {
		env[k] = v
	}

	// policy: child wins per non-empty list
	policy := parentPolicy
	if len(imgFile.Policy.ToolsAllow) > 0 {
		policy.ToolsAllow = imgFile.Policy.ToolsAllow
	}
	if len(imgFile.Policy.ToolsDeny) > 0 {
		policy.ToolsDeny = imgFile.Policy.ToolsDeny
	}

	// harness: inherit, then child-wins per non-empty string field; interactive = child value
	harness := parentHarness
	if imgFile.Harness.Type != "" {
		harness.Type = imgFile.Harness.Type
	}
	if imgFile.Harness.Model != "" {
		harness.Model = imgFile.Harness.Model
	}
	if imgFile.Harness.Effort != "" {
		harness.Effort = imgFile.Harness.Effort
	}
	if imgFile.Harness.InteractiveSet {
		harness.Interactive = imgFile.Harness.Interactive
	}
	if harness.Type == "" {
		harness.Type = "claude"
	}

	// requires_secrets: union
	reqSecrets := []string{}
	secretSeen := map[string]bool{}
	for _, s := range append(append([]string{}, parentSecrets...), imgFile.RequiresSecrets...) {
		if !secretSeen[s] {
			secretSeen[s] = true
			reqSecrets = append(reqSecrets, s)
		}
	}

	// split this file's prompts into overrides and body prompts
	inResolved := map[string]bool{}
	for _, n := range resolved {
		inResolved[n] = true
	}
	overrides := map[string]string{}
	var bodyPrompts []imagefile.Prompt
	for _, pr := range imgFile.Prompts {
		if strings.HasPrefix(pr.Name, "system:") {
			plug := strings.TrimPrefix(pr.Name, "system:")
			if !inResolved[plug] {
				return Manifest{}, fmt.Errorf("prompt override system:%s references a plugin not in the image", plug)
			}
			b, err := os.ReadFile(pr.Filepath)
			if err != nil {
				return Manifest{}, fmt.Errorf("read override %s: %w", pr.Filepath, err)
			}
			overrides[plug] = string(b)
		} else {
			bodyPrompts = append(bodyPrompts, pr)
		}
	}

	// SYSTEM: recomputed for the full set. Precedence per plugin, low→high:
	// builtin capability fragment < plugin-provided prompt < image system: override.
	// Builtins render first (their fixed order); plugin-owned prompts follow, in
	// resolved-plugin order, for any resolved plugin without a builtin fragment.
	var layers []Layer
	emitted := map[string]bool{}
	var sysParts []string
	bodyFragments, err := plugincaps.BodyFragmentsFromStore(resolved, opts.builtinStoreRoot)
	if err != nil {
		return Manifest{}, err
	}
	for _, f := range bodyFragments {
		emitted[f.Plugin] = true
		body := f.Body
		if ov, ok := overrides[f.Plugin]; ok {
			body = ov
		}
		sysParts = append(sysParts, body)
	}
	for _, plug := range resolved {
		if emitted[plug] {
			continue
		}
		body := ""
		if plugin := external[plug]; plugin.HasPrompt {
			body = plugin.Prompt
		}
		if ov, ok := overrides[plug]; ok {
			body = ov
		}
		if body != "" {
			sysParts = append(sysParts, body)
			emitted[plug] = true
		}
	}
	system := strings.Join(sysParts, "\n\n")
	layers = append(layers, Layer{Name: "system", SHA256: sha256hex([]byte(system))})

	// BODY: this image's bespoke prompts, appended to the inherited body
	var bespokeParts []string
	for _, pr := range bodyPrompts {
		b, err := os.ReadFile(pr.Filepath)
		if err != nil {
			return Manifest{}, fmt.Errorf("read prompt %s: %w", pr.Filepath, err)
		}
		bespokeParts = append(bespokeParts, string(b))
		layers = append(layers, Layer{Name: filepath.Base(pr.Filepath), SHA256: sha256hex(b)})
	}
	bespoke := strings.Join(bespokeParts, "\n\n")
	body := bespoke
	switch {
	case parentBody != "" && bespoke != "":
		body = parentBody + "\n\n" + bespoke
	case parentBody != "":
		body = parentBody
	}

	// TAIL: recomputed
	var tailParts []string
	tailFragments, err := plugincaps.TailFragmentsFromStore(resolved, opts.builtinStoreRoot)
	if err != nil {
		return Manifest{}, err
	}
	for _, f := range tailFragments {
		tailParts = append(tailParts, f.Body)
	}
	tail := strings.Join(tailParts, "\n\n")
	layers = append(layers, Layer{Name: "tail", SHA256: sha256hex([]byte(tail))})

	prompt := system
	if body != "" {
		prompt = system + "\n\n" + body
	}

	// manifest (Digest empty until the archive is written)
	manPlugins := make([]ManifestPlugin, 0, len(resolved))
	for _, n := range resolved {
		mp := ManifestPlugin{Name: n}
		if v, ok := versions[n]; ok {
			mp.Version = v
		}
		manPlugins = append(manPlugins, mp)
	}
	evals := make([]ManifestEval, 0, len(imgFile.Evals))
	for _, e := range imgFile.Evals {
		prompt := e.Prompt
		if prompt != "" {
			// imagefile.validate resolved Prompt to an existing absolute path; inline
			// its CONTENT so the criteria/script survives to runtime (the image never
			// packs the file, and the build-host path is meaningless on another host).
			b, err := os.ReadFile(prompt)
			if err != nil {
				return Manifest{}, fmt.Errorf("read eval %q prompt %q: %w", e.Name, e.Prompt, err)
			}
			prompt = string(b)
		}
		evals = append(evals, ManifestEval{Name: e.Name, Type: e.Type, Prompt: prompt})
	}
	if parents == nil {
		parents = []string{}
	}
	man := Manifest{
		SchemaVersion:   1,
		Name:            ref.Name,
		Tag:             ref.Tag,
		BuiltAt:         clock().UTC().Format(time.RFC3339),
		Parents:         parents,
		Plugins:         manPlugins,
		RequiresSecrets: reqSecrets,
		Harness:         harness,
		Env:             env,
		Policy:          policy,
		Evals:           evals,
		Layers:          layers,
	}
	digest, err := store.writeArchive(ref, man, prompt, tail, body, imgFile.Skills, opts.mutableRef)
	if err != nil {
		return Manifest{}, err
	}
	man.Digest = digest
	return man, nil
}
