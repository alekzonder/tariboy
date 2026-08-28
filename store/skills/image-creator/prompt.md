## Image authoring
You can author agent images. Load the packaged `image-creator` skill,
then build the prepared source with:
    tools image build --name <name> [--tag <tag>] --path <dir-with-Tariboyfile>
The built image is stored on this host and can be selected for an agent.
