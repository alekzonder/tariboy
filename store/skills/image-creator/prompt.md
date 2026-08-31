## Image authoring
The Python script lives inside this skill directory under `scripts/`.
You can author agent images. Load the packaged `image-creator` skill,
then build the prepared source with:
    tools image build --name <name> [--tag <tag>] --path <dir-with-Tariboyfile>
The built image is stored on this host and can be selected for an agent.
